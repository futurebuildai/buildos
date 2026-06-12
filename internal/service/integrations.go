package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/cryptobox"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Integration-credential audit-log resource type + action constants.
// Lets `/audit?action_prefix=integration.` reconstruct who set or
// removed which provider key.
const (
	AuditResourceIntegrationCredential = "integration_credential"

	AuditActionIntegrationCredentialSet     = "integration.credential.set"
	AuditActionIntegrationCredentialDeleted = "integration.credential.deleted"
)

// Provider name constants for the keys VaultService resolves on behalf
// of the AI client and the mailer. Arbitrary providers are accepted by
// SetCredential (trimmed + lowercased), but these two are wired into
// the KeyResolver implementations below.
const (
	ProviderAnthropic = "anthropic"
	ProviderResend    = "resend"
	// ProviderObjectStore holds the per-fork S3-compatible (R2) object-store
	// credentials as a JSON-encoded {access_key_id, secret_access_key} pair,
	// sealed AES-256-GCM exactly like the anthropic/resend keys. The endpoint +
	// bucket + region are non-secret and come from config (env), NOT here.
	// Resolved at storage-adapter construction via ObjectStoreCreds.
	ProviderObjectStore = "object_store"
)

// VaultService is the encrypted BYOK credential vault (WS3). It seals
// per-org 3rd-party API keys with cryptobox (AES-256-GCM) and persists
// only the ciphertext; the cleartext key exists transiently inside this
// service during set (Seal) and resolve (Open) and is NEVER logged.
//
// It implements both ai.KeyResolver (AnthropicKey) and
// mailer.KeyResolver (ResendKey) so the vault directly feeds the AI
// client and the mailer. Both resolvers SOFT-FAIL: a missing credential
// returns ("", nil), which ai/mailer treat as "unconfigured".
//
// Follows the canonical one-tx-per-mutation + audit pattern used by the
// other service-layer types (see setup.go, budget.go).
type VaultService struct {
	pool   *pgxpool.Pool
	store  *store.IntegrationCredentialStore
	cipher *cryptobox.Cipher
	audit  AuditRecorder
	logger *slog.Logger
	now    func() time.Time
}

// NewVaultService creates a vault service bound to a pool + store +
// cipher. audit may be nil; nil falls back to a no-op recorder. logger
// may be nil; nil uses slog.Default. clock may be nil; nil uses
// time.Now (tests inject a deterministic clock).
func NewVaultService(pool *pgxpool.Pool, s *store.IntegrationCredentialStore, cipher *cryptobox.Cipher, audit AuditRecorder, logger *slog.Logger, clock func() time.Time) *VaultService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = time.Now
	}
	return &VaultService{pool: pool, store: s, cipher: cipher, audit: audit, logger: logger, now: clock}
}

// SetCredentialInput is the SetCredential payload. Key is the cleartext
// 3rd-party API key — secret material, never logged. UserSub is the
// caller's OIDC subject (recorded in created_by + the audit trail).
type SetCredentialInput struct {
	OrgID    uuid.UUID
	Provider string
	Label    string
	Key      string
	UserSub  string
}

// SetCredential seals a cleartext API key and stores it as the active
// credential for (org_id, provider), atomically rotating out any prior
// active row. The returned model carries only metadata (no secret).
func (s *VaultService) SetCredential(ctx context.Context, in SetCredentialInput) (models.IntegrationCredential, error) {
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if in.OrgID == uuid.Nil {
		return models.IntegrationCredential{}, fmt.Errorf("%w: org_id required", ErrInvalidInput)
	}
	if provider == "" {
		return models.IntegrationCredential{}, fmt.Errorf("%w: provider required", ErrInvalidInput)
	}
	// Don't trim the key: leading/trailing chars may be significant in
	// some providers' key formats. Only reject an entirely-empty key.
	if in.Key == "" {
		return models.IntegrationCredential{}, fmt.Errorf("%w: key required", ErrInvalidInput)
	}

	ciphertext, nonce, version, err := s.cipher.Seal([]byte(in.Key))
	if err != nil {
		// cryptobox errors are intentionally detail-free; wrap without
		// echoing any key material.
		return models.IntegrationCredential{}, fmt.Errorf("seal credential: %w", err)
	}
	last4 := lastNRunes(in.Key, 4)

	var out models.IntegrationCredential
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		c, qErr := s.store.UpsertActive(ctx, tx, store.UpsertActiveCredentialParams{
			OrgID:      in.OrgID,
			Provider:   provider,
			Label:      strings.TrimSpace(in.Label),
			Ciphertext: ciphertext,
			Nonce:      nonce,
			KeyVersion: version,
			Last4:      last4,
			CreatedBy:  in.UserSub,
		})
		if qErr != nil {
			return qErr
		}
		out = c

		// Audit metadata: provider + last4 only. NEVER the key.
		after, _ := json.Marshal(map[string]any{
			"provider": provider,
			"label":    out.Label,
			"last4":    out.Last4,
		})
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      in.UserSub,
			Action:       AuditActionIntegrationCredentialSet,
			ResourceType: AuditResourceIntegrationCredential,
			ResourceID:   out.ID,
			After:        after,
		})
		return nil
	})
	if err != nil {
		return models.IntegrationCredential{}, mapVaultStoreError(err)
	}
	return out, nil
}

// DeleteCredential deactivates the active credential for
// (org_id, provider). Returns ErrNotFound when no active credential
// exists. UserSub is recorded in the audit trail.
func (s *VaultService) DeleteCredential(ctx context.Context, orgID uuid.UUID, provider, userSub string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if orgID == uuid.Nil {
		return fmt.Errorf("%w: org_id required", ErrInvalidInput)
	}
	if provider == "" {
		return fmt.Errorf("%w: provider required", ErrInvalidInput)
	}

	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		affected, qErr := s.store.DeactivateByProvider(ctx, tx, orgID, provider)
		if qErr != nil {
			return qErr
		}
		if affected == 0 {
			return ErrNotFound
		}
		meta, _ := json.Marshal(map[string]any{"provider": provider})
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        orgID,
			UserSub:      userSub,
			Action:       AuditActionIntegrationCredentialDeleted,
			ResourceType: AuditResourceIntegrationCredential,
			ResourceID:   orgID, // no row id to point at after deactivation; org is the subject
			Metadata:     meta,
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidInput) {
			return err
		}
		return mapVaultStoreError(err)
	}
	return nil
}

// ListCredentials returns metadata for every credential (active +
// inactive) in an org, newest first. The secret bytes are never
// emitted (json:"-" on the model).
func (s *VaultService) ListCredentials(ctx context.Context, orgID uuid.UUID) ([]models.IntegrationCredential, error) {
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("%w: org_id required", ErrInvalidInput)
	}
	var out []models.IntegrationCredential
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var qErr error
		out, qErr = s.store.ListByOrg(ctx, tx, orgID)
		return qErr
	})
	if err != nil {
		return nil, mapVaultStoreError(err)
	}
	return out, nil
}

// ---------- Capabilities ----------

// ProviderCapability is the per-provider configured state surfaced by
// Capabilities. Fingerprint is the non-secret last4 of the active key
// (empty when unconfigured); CreatedAt/CreatedBy come from the active
// row. No secret material is ever read or decrypted to build this.
type ProviderCapability struct {
	Provider    string
	Configured  bool
	Fingerprint string
	CreatedAt   time.Time
	CreatedBy   string
}

// CapabilitiesResult reports which vault-backed features are live for an
// org. AIConfigured / EmailConfigured are presence checks (an active
// anthropic / resend credential exists) — they do NOT validate the key
// upstream, matching the soft-fail resolver contract. Drives the
// frontend's proactive AI/email gating (GET /api/v1/capabilities).
type CapabilitiesResult struct {
	AIConfigured      bool
	EmailConfigured   bool
	StorageConfigured bool
	Providers         []ProviderCapability
}

// Capabilities reports per-org feature availability derived purely from
// active credential PRESENCE (no decrypt, no upstream call). It lists
// the two first-class providers (anthropic → AI, resend → email) so the
// console can gate affordances before attempting a call that would 503.
func (s *VaultService) Capabilities(ctx context.Context, orgID uuid.UUID) (CapabilitiesResult, error) {
	creds, err := s.ListCredentials(ctx, orgID)
	if err != nil {
		return CapabilitiesResult{}, err
	}

	// Index the active credential per provider (newest-first ordering
	// from ListByOrg means the first active row is the current one).
	active := make(map[string]models.IntegrationCredential)
	for _, c := range creds {
		if c.IsActive {
			if _, seen := active[c.Provider]; !seen {
				active[c.Provider] = c
			}
		}
	}

	providerFor := func(provider string) ProviderCapability {
		c, ok := active[provider]
		if !ok {
			return ProviderCapability{Provider: provider, Configured: false}
		}
		return ProviderCapability{
			Provider:    provider,
			Configured:  true,
			Fingerprint: c.Last4,
			CreatedAt:   c.CreatedAt,
			CreatedBy:   c.CreatedBy,
		}
	}

	anthropic := providerFor(ProviderAnthropic)
	resend := providerFor(ProviderResend)
	objectStore := providerFor(ProviderObjectStore)
	return CapabilitiesResult{
		AIConfigured:      anthropic.Configured,
		EmailConfigured:   resend.Configured,
		StorageConfigured: objectStore.Configured,
		Providers:         []ProviderCapability{anthropic, resend, objectStore},
	}, nil
}

// ---------- KeyResolver implementations ----------

// AnthropicKey implements ai.KeyResolver. Returns the cleartext
// Anthropic API key for the org, or ("", nil) when none is configured
// (SOFT-FAIL — the AI client treats "" as unconfigured/503). orgID is
// the string form of the org UUID (as carried on the AI client's ctx).
func (s *VaultService) AnthropicKey(ctx context.Context, orgID string) (string, error) {
	return s.resolveActiveKey(ctx, orgID, ProviderAnthropic)
}

// ResendKey implements mailer.KeyResolver. Returns the cleartext Resend
// API key for the org, or ("", nil) when none is configured (SOFT-FAIL
// — the mailer treats "" as unconfigured). orgID is the string form of
// the org UUID.
func (s *VaultService) ResendKey(ctx context.Context, orgID string) (string, error) {
	return s.resolveActiveKey(ctx, orgID, ProviderResend)
}

// ObjectStoreCreds resolves the per-org object-store (R2) credentials: the
// access key id + secret access key sealed under the "object_store" provider.
// SOFT-FAILS to ("", "", nil) when none is configured or on any decrypt
// failure, so the storage adapter falls back to its unconfigured path (uploads
// 503) rather than 500ing — same posture as AnthropicKey/ResendKey. The secret
// is NEVER logged. The stored cleartext is a JSON object:
//
//	{"access_key_id":"...","secret_access_key":"..."}
func (s *VaultService) ObjectStoreCreds(ctx context.Context, orgID uuid.UUID) (accessKeyID, secretAccessKey string, err error) {
	raw, err := s.resolveActiveKey(ctx, orgID.String(), ProviderObjectStore)
	if err != nil {
		return "", "", err
	}
	if raw == "" {
		return "", "", nil // soft-fail: unconfigured
	}
	var creds struct {
		AccessKeyID     string `json:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key"`
	}
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		// A malformed stored blob is treated as unconfigured (soft-fail) — never
		// echo the cleartext in the error.
		s.logger.WarnContext(ctx, "vault: object_store credential is not valid JSON; treating as unconfigured",
			"org_id", orgID, "provider", ProviderObjectStore)
		return "", "", nil
	}
	return creds.AccessKeyID, creds.SecretAccessKey, nil
}

// connectorProviderPrefix namespaces a connector instance's credential in the
// vault as "connector:<name>", so it can never collide with the bare AI/email
// providers (anthropic/resend) — the resolvers above look up only the bare names.
const connectorProviderPrefix = "connector:"

// ResolveConnectorSecret implements connectors.SecretResolver (Phase 3b-ii). It
// returns the cleartext credential for an MCP connector instance, or "" when none
// is configured / on any decrypt failure (SOFT-FAIL — the MCP call then runs
// unauthenticated and soft-fails on a 401). The secret never reaches the agentic
// leaf or the model.
func (s *VaultService) ResolveConnectorSecret(ctx context.Context, orgID uuid.UUID, connectorName string) (string, error) {
	return s.resolveActiveKey(ctx, orgID.String(), connectorProviderPrefix+connectorName)
}

// resolveActiveKey is the shared backing for AnthropicKey + ResendKey.
// It parses orgID, looks up the active credential for the provider, and
// decrypts it. Every failure mode that means "no usable key" SOFT-FAILS
// to ("", nil): a malformed orgID, no active credential, or a decrypt
// failure all return ("", nil) so the AI client / mailer fall back to
// their unconfigured path rather than surfacing a 500. The cleartext
// key is NEVER logged.
func (s *VaultService) resolveActiveKey(ctx context.Context, orgID, provider string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(orgID))
	if err != nil {
		// Soft-fail: an unparseable org id can't have a credential.
		return "", nil
	}

	var cred models.IntegrationCredential
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		c, qErr := s.store.GetActiveByProvider(ctx, tx, parsed, provider)
		if qErr != nil {
			return qErr
		}
		cred = c
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Soft-fail: no active credential for this provider.
			return "", nil
		}
		return "", mapVaultStoreError(err)
	}

	plaintext, err := s.cipher.Open(cred.Ciphertext, cred.Nonce)
	if err != nil {
		// A decrypt failure (wrong/rotated master key, tampered row)
		// must not 500 the AI/mail path. Log without any key material
		// and soft-fail to unconfigured.
		s.logger.WarnContext(ctx, "vault: credential decrypt failed; treating as unconfigured",
			"org_id", parsed, "provider", provider)
		return "", nil
	}
	return string(plaintext), nil
}

// ---------- helpers ----------

// lastNRunes returns the last n runes of s (or all of s when it has
// fewer than n runes). Used to compute the non-secret last4 display
// hint for a stored key.
func lastNRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return string(r)
	}
	return string(r[len(r)-n:])
}

// mapVaultStoreError translates a pgx unique-violation (SQLSTATE 23505)
// into ErrInvalidInput so the HTTP handler maps to 400/409 rather than
// 500. Falls through to the package-shared mapStoreError otherwise
// (store.ErrNotFound → ErrNotFound). Mirrors mapSetupStoreError.
func mapVaultStoreError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %s", ErrInvalidInput, pgErr.ConstraintName)
	}
	return mapStoreError(err)
}
