package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/auth"
	"github.com/futurebuildai/buildos/internal/mailer"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Native-auth sentinel errors. Handlers map these to HTTP codes:
//
//	ErrInvalidCredentials    → 401 (login)
//	ErrInvalidRefreshToken   → 401 (refresh)
//	ErrInvalidResetToken     → 400 (password reset confirm)
//	ErrFirstOwnerExists      → 409 (bootstrap claim after an owner exists)
//
// ErrInvalidCredentials is intentionally uniform across "unknown email",
// "no native password set", and "wrong password" to avoid an account-
// enumeration oracle.
var (
	ErrInvalidCredentials  = errors.New("auth: invalid credentials")
	ErrInvalidRefreshToken = errors.New("auth: invalid refresh token")
	ErrInvalidResetToken   = errors.New("auth: invalid or expired reset token")
	ErrFirstOwnerExists    = errors.New("auth: an owner already exists for this org")
)

// Native-auth audit actions.
const (
	AuditResourceUser = "user"

	AuditActionAuthOwnerClaimed   = "auth.owner.claimed"
	AuditActionAuthLogin          = "auth.login"
	AuditActionAuthLogout         = "auth.logout"
	AuditActionAuthRefresh        = "auth.token.refreshed"
	AuditActionAuthResetRequested = "auth.password_reset.requested"
	AuditActionAuthResetCompleted = "auth.password_reset.completed"
)

// Default token lifetimes. Access-token TTL lives on the TokenIssuer; these
// govern the opaque refresh and reset tokens.
const (
	DefaultRefreshTTL       = 30 * 24 * time.Hour
	DefaultPasswordResetTTL = 1 * time.Hour
)

// AuthService owns native identity: first-owner claim, login, token refresh,
// logout, and password reset. It mints RS256 access tokens via an
// auth.TokenIssuer and persists opaque (hashed) refresh + reset tokens.
//
// One tx per mutation, composed with the audit write — same pattern as the
// other services.
type AuthService struct {
	pool       *pgxpool.Pool
	users      *store.UserStore
	setup      *store.SetupStore // bootstrap-token validate/redeem for first-owner claim
	issuer     *auth.TokenIssuer
	mailer     mailer.Mailer
	audit      AuditRecorder
	logger     *slog.Logger
	now        func() time.Time
	refreshTTL time.Duration
	resetTTL   time.Duration
	appBaseURL string // used to build password-reset links in email
	dummyHash  string // fixed argon2id hash to equalize login timing (anti-enumeration)
}

// AuthServiceConfig configures NewAuthService. Pool, Users, Setup, and Issuer
// are required; the rest default sensibly.
type AuthServiceConfig struct {
	Pool       *pgxpool.Pool
	Users      *store.UserStore
	Setup      *store.SetupStore
	Issuer     *auth.TokenIssuer
	Mailer     mailer.Mailer // nil → no-op
	Audit      AuditRecorder // nil → no-op
	Logger     *slog.Logger  // nil → slog.Default()
	Clock      func() time.Time
	RefreshTTL time.Duration
	ResetTTL   time.Duration
	AppBaseURL string
}

// NewAuthService builds an AuthService from its dependencies.
func NewAuthService(cfg AuthServiceConfig) (*AuthService, error) {
	if cfg.Pool == nil || cfg.Users == nil || cfg.Setup == nil || cfg.Issuer == nil {
		return nil, errors.New("auth: pool, users, setup, and issuer are required")
	}
	if cfg.Mailer == nil {
		cfg.Mailer = mailer.NewNoopMailer(cfg.Logger)
	}
	if cfg.Audit == nil {
		cfg.Audit = NewNoopAuditRecorder()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = DefaultRefreshTTL
	}
	if cfg.ResetTTL <= 0 {
		cfg.ResetTTL = DefaultPasswordResetTTL
	}
	// Precompute a fixed argon2id hash once at startup. Login verifies against
	// it on the unknown-email / no-password paths so every attempt spends the
	// same KDF cost — defeating account enumeration by response timing.
	dummyHash, derr := auth.HashPassword("buildos-login-timing-equalizer")
	if derr != nil {
		cfg.Logger.Warn("auth: could not precompute timing-equalizer hash", "error", derr)
	}
	return &AuthService{
		pool:       cfg.Pool,
		users:      cfg.Users,
		setup:      cfg.Setup,
		issuer:     cfg.Issuer,
		mailer:     cfg.Mailer,
		audit:      cfg.Audit,
		logger:     cfg.Logger,
		now:        cfg.Clock,
		refreshTTL: cfg.RefreshTTL,
		resetTTL:   cfg.ResetTTL,
		appBaseURL: cfg.AppBaseURL,
		dummyHash:  dummyHash,
	}, nil
}

// TokenPair is what a successful login / refresh / first-owner claim returns.
// RefreshToken is the cleartext opaque token shown to the client once; only
// its hash is stored. User carries the authenticated principal (password hash
// elided by its json:"-" tag).
type TokenPair struct {
	AccessToken  string
	ExpiresAt    time.Time
	ExpiresIn    int // seconds until access-token expiry
	RefreshToken string
	User         models.User
}

// ClaimFirstOwner redeems a bootstrap token to create the fork's first owner
// with a native email/password credential, then issues a token pair. This is
// the unauthenticated entry point that replaces the old JWT-gated bootstrap
// redemption (Brain JIT-provisioning is gone).
//
// Invariants enforced in one transaction:
//   - the bootstrap token must be active (validated + redeemed here)
//   - the token's org must have zero existing users (first owner only)
//   - the new user is created with role=owner and the supplied password hash
//
// Any bootstrap-token failure returns the uniform ErrInvalidBootstrapToken;
// a pre-existing user returns ErrFirstOwnerExists.
func (s *AuthService) ClaimFirstOwner(ctx context.Context, cleartext, email, password, displayName string) (TokenPair, error) {
	email = strings.TrimSpace(email)
	if cleartext == "" {
		return TokenPair{}, ErrInvalidBootstrapToken
	}
	if err := validateEmail(email); err != nil {
		return TokenPair{}, err
	}
	if err := validatePassword(password); err != nil {
		return TokenPair{}, err
	}
	if displayName == "" {
		displayName = email
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return TokenPair{}, fmt.Errorf("auth: hash password: %w", err)
	}
	hash := auth.HashOpaqueToken(cleartext)
	now := s.now()

	var owner models.User
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		tok, gerr := s.setup.GetActiveBootstrapTokenByHash(ctx, tx, hash, now)
		if gerr != nil {
			if errors.Is(gerr, store.ErrNotFound) {
				return ErrInvalidBootstrapToken
			}
			return gerr
		}

		// First-owner-only: refuse if the org already has any user.
		count, cerr := s.users.CountUsersInOrg(ctx, tx, tok.OrgID)
		if cerr != nil {
			return cerr
		}
		if count > 0 {
			return ErrFirstOwnerExists
		}

		u, cerr := s.users.CreateUser(ctx, tx, store.CreateUserParams{
			OrgID:        tok.OrgID,
			Email:        email,
			DisplayName:  displayName,
			Role:         "owner",
			PasswordHash: passwordHash,
		})
		if cerr != nil {
			return cerr
		}
		owner = u

		// Redeem the bootstrap token, pointing redeemed_by at the new owner.
		if rerr := s.setup.RedeemBootstrapToken(ctx, tx, tok.ID, u.ID, now); rerr != nil {
			if errors.Is(rerr, store.ErrNotFound) {
				return ErrInvalidBootstrapToken // lost a redeem race
			}
			return rerr
		}

		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        tok.OrgID,
			UserSub:      u.ID.String(),
			Action:       AuditActionAuthOwnerClaimed,
			ResourceType: AuditResourceUser,
			ResourceID:   u.ID,
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidBootstrapToken) || errors.Is(err, ErrFirstOwnerExists) || errors.Is(err, ErrInvalidInput) {
			return TokenPair{}, err
		}
		return TokenPair{}, mapAuthStoreError(err)
	}

	return s.issueTokenPair(ctx, owner)
}

// Login verifies an email/password credential and returns a token pair. All
// failure modes (unknown email, no native password, wrong password) collapse
// to ErrInvalidCredentials.
func (s *AuthService) Login(ctx context.Context, email, password string) (TokenPair, error) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return TokenPair{}, ErrInvalidCredentials
	}

	var u models.User
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		found, gerr := s.users.GetUserByEmailGlobal(ctx, tx, email)
		if gerr != nil {
			if errors.Is(gerr, store.ErrNotFound) {
				// Spend the same argon2id cost as a real verify so response
				// time doesn't reveal whether the account exists.
				_ = auth.VerifyPassword(password, s.dummyHash)
				return ErrInvalidCredentials
			}
			return gerr
		}
		if found.PasswordHash == "" {
			// Legacy/OIDC user with no native password set — equalize timing too.
			_ = auth.VerifyPassword(password, s.dummyHash)
			return ErrInvalidCredentials
		}
		if verr := auth.VerifyPassword(password, found.PasswordHash); verr != nil {
			return ErrInvalidCredentials
		}
		u = found
		if terr := s.users.TouchLastLogin(ctx, tx, found.ID, s.now()); terr != nil {
			return terr
		}
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        found.OrgID,
			UserSub:      found.ID.String(),
			Action:       AuditActionAuthLogin,
			ResourceType: AuditResourceUser,
			ResourceID:   found.ID,
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return TokenPair{}, err
		}
		return TokenPair{}, mapAuthStoreError(err)
	}

	return s.issueTokenPair(ctx, u)
}

// Refresh rotates a refresh token: it validates the presented cleartext,
// revokes the old token, issues a new refresh token, and mints a fresh access
// token. A reused/revoked/expired token returns ErrInvalidRefreshToken.
func (s *AuthService) Refresh(ctx context.Context, cleartext string) (TokenPair, error) {
	if cleartext == "" {
		return TokenPair{}, ErrInvalidRefreshToken
	}
	hash := auth.HashOpaqueToken(cleartext)
	now := s.now()

	var (
		user       models.User
		newRefresh string
	)
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		tok, gerr := s.users.GetActiveRefreshToken(ctx, tx, hash, now)
		if gerr != nil {
			if errors.Is(gerr, store.ErrNotFound) {
				return ErrInvalidRefreshToken
			}
			return gerr
		}
		u, uerr := s.users.GetUserByID(ctx, tx, tok.UserID)
		if uerr != nil {
			if errors.Is(uerr, store.ErrNotFound) {
				return ErrInvalidRefreshToken
			}
			return uerr
		}
		user = u

		// Rotate: revoke the presented token, mint a fresh one.
		if rerr := s.users.RevokeRefreshToken(ctx, tx, tok.ID, now); rerr != nil {
			if errors.Is(rerr, store.ErrNotFound) {
				return ErrInvalidRefreshToken // raced another rotation
			}
			return rerr
		}
		ct, _, ierr := s.createRefreshToken(ctx, tx, u, now)
		if ierr != nil {
			return ierr
		}
		newRefresh = ct

		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        u.OrgID,
			UserSub:      u.ID.String(),
			Action:       AuditActionAuthRefresh,
			ResourceType: AuditResourceUser,
			ResourceID:   u.ID,
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) {
			return TokenPair{}, err
		}
		return TokenPair{}, mapAuthStoreError(err)
	}

	access, exp, merr := s.issuer.Mint(user.ID.String(), user.OrgID.String(), user.Role, "")
	if merr != nil {
		return TokenPair{}, fmt.Errorf("auth: mint access token: %w", merr)
	}
	return TokenPair{
		AccessToken:  access,
		ExpiresAt:    exp,
		ExpiresIn:    int(s.issuer.AccessTTL().Seconds()),
		RefreshToken: newRefresh,
		User:         user,
	}, nil
}

// Logout revokes the presented refresh token. It is idempotent: an unknown or
// already-revoked token is treated as success (the goal state — token no
// longer valid — is reached either way).
func (s *AuthService) Logout(ctx context.Context, cleartext string) error {
	if cleartext == "" {
		return nil
	}
	hash := auth.HashOpaqueToken(cleartext)
	now := s.now()
	return pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		tok, gerr := s.users.GetActiveRefreshToken(ctx, tx, hash, now)
		if gerr != nil {
			if errors.Is(gerr, store.ErrNotFound) {
				return nil // already gone — idempotent
			}
			return mapAuthStoreError(gerr)
		}
		if rerr := s.users.RevokeRefreshToken(ctx, tx, tok.ID, now); rerr != nil {
			if errors.Is(rerr, store.ErrNotFound) {
				return nil
			}
			return mapAuthStoreError(rerr)
		}
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        tok.OrgID,
			UserSub:      tok.UserID.String(),
			Action:       AuditActionAuthLogout,
			ResourceType: AuditResourceUser,
			ResourceID:   tok.UserID,
		})
		return nil
	})
}

// RequestPasswordReset issues a single-use reset token and emails it to the
// user. To avoid account enumeration it ALWAYS reports success to the caller,
// whether or not the email matches a user — the handler returns 202 either
// way. A real user gets a token + email; an unknown email is a silent no-op.
func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	now := s.now()

	var (
		user      models.User
		cleartext string
		found     bool
	)
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		u, gerr := s.users.GetUserByEmailGlobal(ctx, tx, email)
		if gerr != nil {
			if errors.Is(gerr, store.ErrNotFound) {
				return nil // silent no-op — no enumeration
			}
			return gerr
		}
		ct, hash, terr := auth.GenerateOpaqueToken()
		if terr != nil {
			return terr
		}
		if _, cerr := s.users.CreatePasswordResetToken(ctx, tx, store.CreatePasswordResetTokenParams{
			UserID:    u.ID,
			TokenHash: hash,
			ExpiresAt: now.Add(s.resetTTL),
		}); cerr != nil {
			return cerr
		}
		user, cleartext, found = u, ct, true
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        u.OrgID,
			UserSub:      u.ID.String(),
			Action:       AuditActionAuthResetRequested,
			ResourceType: AuditResourceUser,
			ResourceID:   u.ID,
		})
		return nil
	})
	if err != nil {
		return mapAuthStoreError(err)
	}

	if found {
		// Best-effort email; a mailer failure must not leak (or fail the
		// request). Log at WARN and still report success to the caller.
		if merr := s.sendResetEmail(ctx, user, cleartext); merr != nil {
			s.logger.WarnContext(ctx, "password reset email send failed",
				"org_id", user.OrgID.String(), "error", merr)
		}
	}
	return nil
}

// ResetPassword consumes a reset token, sets the new password, and revokes all
// of the user's refresh tokens (forcing re-login everywhere). A bad/expired/
// reused token returns ErrInvalidResetToken.
func (s *AuthService) ResetPassword(ctx context.Context, cleartext, newPassword string) error {
	if cleartext == "" {
		return ErrInvalidResetToken
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	newHash, herr := auth.HashPassword(newPassword)
	if herr != nil {
		return fmt.Errorf("auth: hash password: %w", herr)
	}
	hash := auth.HashOpaqueToken(cleartext)
	now := s.now()

	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		tok, gerr := s.users.GetActivePasswordResetToken(ctx, tx, hash, now)
		if gerr != nil {
			if errors.Is(gerr, store.ErrNotFound) {
				return ErrInvalidResetToken
			}
			return gerr
		}
		u, uerr := s.users.GetUserByID(ctx, tx, tok.UserID)
		if uerr != nil {
			if errors.Is(uerr, store.ErrNotFound) {
				return ErrInvalidResetToken
			}
			return uerr
		}
		if perr := s.users.UpdatePasswordHash(ctx, tx, u.ID, newHash); perr != nil {
			return perr
		}
		if rerr := s.users.RedeemPasswordResetToken(ctx, tx, tok.ID, now); rerr != nil {
			if errors.Is(rerr, store.ErrNotFound) {
				return ErrInvalidResetToken // replayed
			}
			return rerr
		}
		// Invalidate every active session — a reset implies the old
		// credential may be compromised.
		if _, rerr := s.users.RevokeAllRefreshTokensForUser(ctx, tx, u.ID, now); rerr != nil {
			return rerr
		}
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        u.OrgID,
			UserSub:      u.ID.String(),
			Action:       AuditActionAuthResetCompleted,
			ResourceType: AuditResourceUser,
			ResourceID:   u.ID,
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidResetToken) || errors.Is(err, ErrInvalidInput) {
			return err
		}
		return mapAuthStoreError(err)
	}
	return nil
}

// ---------- helpers ----------

// issueTokenPair mints an access token and a fresh refresh token for a user in
// its own short transaction (refresh-token insert + nothing else).
func (s *AuthService) issueTokenPair(ctx context.Context, u models.User) (TokenPair, error) {
	access, exp, err := s.issuer.Mint(u.ID.String(), u.OrgID.String(), u.Role, "")
	if err != nil {
		return TokenPair{}, fmt.Errorf("auth: mint access token: %w", err)
	}
	now := s.now()
	var cleartext string
	terr := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		ct, _, ierr := s.createRefreshToken(ctx, tx, u, now)
		if ierr != nil {
			return ierr
		}
		cleartext = ct
		return nil
	})
	if terr != nil {
		return TokenPair{}, mapAuthStoreError(terr)
	}
	return TokenPair{
		AccessToken:  access,
		ExpiresAt:    exp,
		ExpiresIn:    int(s.issuer.AccessTTL().Seconds()),
		RefreshToken: cleartext,
		User:         u,
	}, nil
}

// createRefreshToken generates an opaque refresh token, stores its hash, and
// returns the cleartext + row id.
func (s *AuthService) createRefreshToken(ctx context.Context, tx pgx.Tx, u models.User, now time.Time) (cleartext string, id uuid.UUID, err error) {
	ct, hash, gerr := auth.GenerateOpaqueToken()
	if gerr != nil {
		return "", uuid.Nil, gerr
	}
	id, ierr := s.users.CreateRefreshToken(ctx, tx, store.CreateRefreshTokenParams{
		UserID:    u.ID,
		OrgID:     u.OrgID,
		TokenHash: hash,
		ExpiresAt: now.Add(s.refreshTTL),
	})
	if ierr != nil {
		return "", uuid.Nil, ierr
	}
	return ct, id, nil
}

// sendResetEmail composes and sends the password-reset email. The reset link
// embeds the cleartext token as a query param against the configured app base
// URL; when no base URL is configured the token is shown inline (dev rigs).
func (s *AuthService) sendResetEmail(ctx context.Context, u models.User, cleartext string) error {
	link := cleartext
	if s.appBaseURL != "" {
		link = strings.TrimRight(s.appBaseURL, "/") + "/reset-password?token=" + url.QueryEscape(cleartext)
	}
	msg := mailer.Message{
		To:      u.Email,
		Subject: "Reset your BuildOS password",
		TextBody: fmt.Sprintf(
			"A password reset was requested for your BuildOS account.\n\n"+
				"Use this link to choose a new password:\n%s\n\n"+
				"If you did not request this, you can ignore this email.",
			link,
		),
		HTMLBody: fmt.Sprintf(
			"<p>A password reset was requested for your BuildOS account.</p>"+
				"<p><a href=\"%s\">Choose a new password</a></p>"+
				"<p>If you did not request this, you can ignore this email.</p>",
			link,
		),
	}
	return s.mailer.Send(ctx, u.OrgID.String(), msg)
}

// validateEmail applies a minimal structural check — an "@" with non-empty
// local and domain parts. The real validation is the confirmation email; this
// just rejects obvious garbage at the boundary.
func validateEmail(email string) error {
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 || strings.ContainsAny(email, " \t\r\n") {
		return fmt.Errorf("%w: email is not valid", ErrInvalidInput)
	}
	return nil
}

// validatePassword enforces a minimum length. Kept simple and explicit; the
// argon2id hash defends against weak-but-long passphrases far better than
// composition rules do.
func validatePassword(password string) error {
	if len(password) < 12 {
		return fmt.Errorf("%w: password must be at least 12 characters", ErrInvalidInput)
	}
	return nil
}

// mapAuthStoreError translates a pgx unique-violation (SQLSTATE 23505) into
// ErrInvalidInput (e.g. duplicate email) so the handler returns 409/422 rather
// than 500; other errors fall through to the package-shared mapStoreError.
func mapAuthStoreError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %s", ErrInvalidInput, pgErr.ConstraintName)
	}
	return mapStoreError(err)
}
