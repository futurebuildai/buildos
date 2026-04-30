// Command dev-idp is a tiny mock OIDC issuer used in place of The Brain
// for staging environments and sales demos. It generates a fresh RSA
// keypair on startup, exposes a JWKS endpoint, and mints RS256 JWTs
// that BuildOS validates exactly as it would tokens from The Brain.
//
// Endpoints:
//
//	GET  /healthz       liveness check
//	GET  /jwks          JSON Web Key Set with the public signing key
//	POST /token         issue a JWT for arbitrary claims (dev/CI)
//	POST /demo/login    issue a JWT for a pre-seeded persona
//	GET  /personas      list available personas
//
// NOT for production. Do not deploy this binary to customer-facing
// environments. The persona set is hardcoded and the keypair is
// regenerated on every restart, which means tokens issued before a
// restart stop validating after BuildOS's JWKS cache TTL elapses.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
)

// Persona is a pre-seeded identity the /demo/login endpoint will mint
// a token for. The set is hardcoded; edit defaultPersonas to add more.
type Persona struct {
	Username string `json:"username"`
	Display  string `json:"display"`
	Sub      string `json:"sub"`
	OrgID    string `json:"org_id"`
	Role     string `json:"role"`
	PlanTier string `json:"plan_tier"`
}

// Demo org UUID — every default persona belongs to this org. Seed fixture
// data should reference this same UUID so demo flows work end-to-end.
const demoOrgID = "11111111-1111-1111-1111-111111111111"

var defaultPersonas = []Persona{
	{Username: "alice", Display: "Alice (Owner)", Sub: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", OrgID: demoOrgID, Role: "owner", PlanTier: "enterprise"},
	{Username: "bob", Display: "Bob (Admin)", Sub: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", OrgID: demoOrgID, Role: "admin", PlanTier: "enterprise"},
	{Username: "carol", Display: "Carol (Superintendent)", Sub: "cccccccc-cccc-cccc-cccc-cccccccccccc", OrgID: demoOrgID, Role: "superintendent", PlanTier: "enterprise"},
	{Username: "dave", Display: "Dave (Field Worker)", Sub: "dddddddd-dddd-dddd-dddd-dddddddddddd", OrgID: demoOrgID, Role: "field_worker", PlanTier: "enterprise"},
}

type config struct {
	port     string
	issuer   string
	audience string
	tokenTTL time.Duration
}

type server struct {
	cfg      config
	signer   jose.Signer
	jwks     jose.JSONWebKeySet
	personas map[string]Persona
	logger   *slog.Logger
}

// signedClaims is the on-the-wire shape: BuildOS-specific fields plus
// the standard registered jwt.Claims (iss, aud, exp, …). The custom
// `Sub` field shadows the embedded Subject so the wire output has a
// single "sub" key.
type signedClaims struct {
	Sub      string `json:"sub"`
	OrgID    string `json:"org_id"`
	Role     string `json:"role"`
	PlanTier string `json:"plan_tier"`
	jwt.Claims
}

func main() {
	port := flag.String("port", getEnv("DEV_IDP_PORT", "8083"), "HTTP port")
	issuer := flag.String("issuer", getEnv("DEV_IDP_ISSUER", ""), "JWT issuer URL (default: http://localhost:<port>)")
	audience := flag.String("audience", getEnv("DEV_IDP_AUDIENCE", "fb-os"), "JWT audience claim — must match BuildOS expected aud")
	ttlSec := flag.Int("ttl", parseInt(getEnv("DEV_IDP_TTL_SECONDS", "3600"), 3600), "Token TTL in seconds")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if *issuer == "" {
		*issuer = "http://localhost:" + *port
	}

	cfg := config{
		port:     *port,
		issuer:   *issuer,
		audience: *audience,
		tokenTTL: time.Duration(*ttlSec) * time.Second,
	}

	if err := run(cfg, logger); err != nil {
		logger.Error("dev-idp exited with error", "error", err)
		os.Exit(1)
	}
}

func run(cfg config, logger *slog.Logger) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generating RSA keypair: %w", err)
	}
	keyID := "dev-idp-" + uuid.NewString()

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: priv},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", keyID),
	)
	if err != nil {
		return fmt.Errorf("creating signer: %w", err)
	}

	jwks := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key:       priv.Public(),
		KeyID:     keyID,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}}}

	personaIdx := make(map[string]Persona, len(defaultPersonas))
	for _, p := range defaultPersonas {
		personaIdx[p.Username] = p
	}

	srv := &server{cfg: cfg, signer: signer, jwks: jwks, personas: personaIdx, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.handleHealth)
	mux.HandleFunc("/jwks", srv.handleJWKS)
	mux.HandleFunc("/token", srv.handleToken)
	mux.HandleFunc("/demo/login", srv.handleDemoLogin)
	mux.HandleFunc("/personas", srv.handlePersonas)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("dev-idp started",
			"port", cfg.port,
			"issuer", cfg.issuer,
			"audience", cfg.audience,
			"key_id", keyID,
			"personas", len(personaIdx))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.jwks)
}

type tokenRequest struct {
	Sub        string `json:"sub"`
	OrgID      string `json:"org_id"`
	Role       string `json:"role"`
	PlanTier   string `json:"plan_tier,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func (s *server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req tokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Sub == "" || req.OrgID == "" || req.Role == "" {
		writeError(w, http.StatusBadRequest, "sub, org_id, role are required")
		return
	}
	if req.PlanTier == "" {
		req.PlanTier = "enterprise"
	}
	ttl := s.cfg.tokenTTL
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	tok, expiresIn, err := s.issueToken(req.Sub, req.OrgID, req.Role, req.PlanTier, ttl)
	if err != nil {
		s.logger.Error("issue token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: tok,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
	})
}

type demoLoginRequest struct {
	Username string `json:"username"`
}

type demoLoginResponse struct {
	AccessToken string  `json:"access_token"`
	TokenType   string  `json:"token_type"`
	ExpiresIn   int     `json:"expires_in"`
	Persona     Persona `json:"persona"`
}

func (s *server) handleDemoLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req demoLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	p, ok := s.personas[req.Username]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown persona; GET /personas to list available")
		return
	}
	tok, expiresIn, err := s.issueToken(p.Sub, p.OrgID, p.Role, p.PlanTier, s.cfg.tokenTTL)
	if err != nil {
		s.logger.Error("demo login: issue token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, demoLoginResponse{
		AccessToken: tok,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
		Persona:     p,
	})
}

func (s *server) handlePersonas(w http.ResponseWriter, _ *http.Request) {
	out := make([]Persona, 0, len(s.personas))
	for _, p := range s.personas {
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"personas": out})
}

func (s *server) issueToken(sub, orgID, role, planTier string, ttl time.Duration) (string, int, error) {
	now := time.Now()
	c := signedClaims{
		Sub:      sub,
		OrgID:    orgID,
		Role:     role,
		PlanTier: planTier,
		Claims: jwt.Claims{
			Issuer:   s.cfg.issuer,
			Audience: jwt.Audience{s.cfg.audience},
			IssuedAt: jwt.NewNumericDate(now),
			Expiry:   jwt.NewNumericDate(now.Add(ttl)),
			ID:       uuid.NewString(),
		},
	}
	tok, err := jwt.Signed(s.signer).Claims(c).Serialize()
	if err != nil {
		return "", 0, err
	}
	return tok, int(ttl.Seconds()), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func parseInt(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}
