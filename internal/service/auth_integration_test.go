//go:build integration

package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/auth"
	"github.com/futurebuildai/buildos/internal/mailer"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// capturingMailer records every Send call so reset-email tests can assert
// the message was composed and addressed correctly. Not safe for concurrent
// use; the auth service sends sequentially per request.
type capturingMailer struct {
	mu      sync.Mutex
	sent    []capturedMail
	sendErr error // when set, Send records the message but returns this error
}

type capturedMail struct {
	OrgID string
	Msg   mailer.Message
}

func (m *capturingMailer) Send(_ context.Context, orgID string, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, capturedMail{OrgID: orgID, Msg: msg})
	return m.sendErr
}

// setErr arms the mailer to return err on the next (and every) Send, while
// still recording the attempt — used to exercise the best-effort send leg.
func (m *capturingMailer) setErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
}

func (m *capturingMailer) last() (capturedMail, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return capturedMail{}, false
	}
	return m.sent[len(m.sent)-1], true
}

// newAuthService builds an AuthService against a fresh pool with a freshly
// generated RSA key, a capturing mailer, and an injected clock. Returns the
// service, the seeded org id, and the mailer so tests can inspect sent mail.
func newAuthService(t *testing.T, clock func() time.Time) (*AuthService, uuid.UUID, *capturingMailer) {
	t.Helper()
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	issuer, err := auth.NewTokenIssuer(priv, "test-kid", "buildos", "buildos")
	if err != nil {
		t.Fatalf("new token issuer: %v", err)
	}

	mail := &capturingMailer{}
	svc, err := NewAuthService(AuthServiceConfig{
		Pool:       pool,
		Users:      store.NewUserStore(),
		Setup:      store.NewSetupStore(),
		Issuer:     issuer,
		Mailer:     mail,
		Clock:      clock,
		AppBaseURL: "https://app.example.test",
	})
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	return svc, orgID, mail
}

// issueBootstrap mints a bootstrap token for orgID via the setup service so
// ClaimFirstOwner has a real active token to redeem.
func issueBootstrap(t *testing.T, svc *AuthService, orgID uuid.UUID, clock func() time.Time) string {
	t.Helper()
	setupSvc := NewSetupService(svc.pool, store.NewSetupStore(), NewNoopAuditRecorder(), clock)
	issued, err := setupSvc.IssueBootstrapToken(context.Background(), orgID, "operator", 0)
	if err != nil {
		t.Fatalf("issue bootstrap token: %v", err)
	}
	return issued.Cleartext
}

// TestAuthService_NewAuthService_Guards covers the constructor's two legs the
// newAuthService helper never exercises: the required-dependency guard (a nil
// Pool/Users/Setup/Issuer → error) and the nil-Mailer default (the helper
// always injects a capturingMailer, so the noop-mailer fallback is unreached).
func TestAuthService_NewAuthService_Guards(t *testing.T) {
	pool := testdb.NewPool(t)
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	issuer, err := auth.NewTokenIssuer(priv, "test-kid", "buildos", "buildos")
	if err != nil {
		t.Fatalf("new token issuer: %v", err)
	}

	// Missing a required dependency (Pool) is rejected.
	if _, err := NewAuthService(AuthServiceConfig{
		Users: store.NewUserStore(), Setup: store.NewSetupStore(), Issuer: issuer,
	}); err == nil {
		t.Error("NewAuthService(nil pool) = nil error, want a required-deps error")
	}

	// A nil Mailer is allowed — it defaults to the noop mailer and the
	// service constructs successfully.
	svc, err := NewAuthService(AuthServiceConfig{
		Pool: pool, Users: store.NewUserStore(), Setup: store.NewSetupStore(), Issuer: issuer,
		// Mailer intentionally nil → exercises the noop-default leg.
	})
	if err != nil {
		t.Fatalf("NewAuthService(nil mailer) = %v, want success", err)
	}
	if svc.mailer == nil {
		t.Error("mailer was not defaulted to the noop mailer")
	}
}

func TestAuthService_ClaimFirstOwner_HappyPath(t *testing.T) {
	svc, orgID, _ := newAuthService(t, nil)
	ctx := context.Background()
	token := issueBootstrap(t, svc, orgID, nil)

	pair, err := svc.ClaimFirstOwner(ctx, token, "owner@kelbrook.test", "correct horse battery staple", "Owner One")
	if err != nil {
		t.Fatalf("ClaimFirstOwner: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected non-empty access and refresh tokens")
	}
	if pair.User.Role != "owner" {
		t.Errorf("Role = %q, want owner", pair.User.Role)
	}
	if pair.User.OrgID != orgID {
		t.Errorf("OrgID = %s, want %s", pair.User.OrgID, orgID)
	}
	if pair.ExpiresIn <= 0 {
		t.Errorf("ExpiresIn = %d, want > 0", pair.ExpiresIn)
	}

	// The new owner can now log in with the same credentials.
	loginPair, err := svc.Login(ctx, "owner@kelbrook.test", "correct horse battery staple")
	if err != nil {
		t.Fatalf("Login after claim: %v", err)
	}
	if loginPair.User.ID != pair.User.ID {
		t.Errorf("login user id = %s, want %s", loginPair.User.ID, pair.User.ID)
	}
}

func TestAuthService_ClaimFirstOwner_SecondClaimRejected(t *testing.T) {
	svc, orgID, _ := newAuthService(t, nil)
	ctx := context.Background()

	token1 := issueBootstrap(t, svc, orgID, nil)
	if _, err := svc.ClaimFirstOwner(ctx, token1, "owner@kelbrook.test", "correct horse battery staple", ""); err != nil {
		t.Fatalf("first claim: %v", err)
	}

	// A second token for the same org cannot create a second owner.
	token2 := issueBootstrap(t, svc, orgID, nil)
	_, err := svc.ClaimFirstOwner(ctx, token2, "intruder@kelbrook.test", "another long password here", "")
	if !errors.Is(err, ErrFirstOwnerExists) {
		t.Fatalf("second claim err = %v, want ErrFirstOwnerExists", err)
	}
}

func TestAuthService_ClaimFirstOwner_InvalidToken(t *testing.T) {
	svc, _, _ := newAuthService(t, nil)
	_, err := svc.ClaimFirstOwner(context.Background(), "not-a-real-token", "owner@kelbrook.test", "correct horse battery staple", "")
	if !errors.Is(err, ErrInvalidBootstrapToken) {
		t.Fatalf("err = %v, want ErrInvalidBootstrapToken", err)
	}
}

func TestAuthService_ClaimFirstOwner_RejectsShortPassword(t *testing.T) {
	svc, orgID, _ := newAuthService(t, nil)
	token := issueBootstrap(t, svc, orgID, nil)
	_, err := svc.ClaimFirstOwner(context.Background(), token, "owner@kelbrook.test", "short", "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// TestAuthService_ClaimFirstOwner_RejectsEmptyTokenAndBadEmail covers the
// two pre-redemption guards the InvalidToken/RejectsShortPassword tests miss:
// an empty cleartext token short-circuits to ErrInvalidBootstrapToken before
// any DB touch (InvalidToken passes a non-empty bogus token, which instead
// reaches the redemption-failure leg), and a malformed email trips
// validateEmail → ErrInvalidInput before the redemption tx (so a non-empty
// cleartext is enough to get past the empty-token guard to the email check).
func TestAuthService_ClaimFirstOwner_RejectsEmptyTokenAndBadEmail(t *testing.T) {
	svc, _, _ := newAuthService(t, nil)
	ctx := context.Background()

	if _, err := svc.ClaimFirstOwner(ctx, "", "owner@kelbrook.test", "correct horse battery staple", ""); !errors.Is(err, ErrInvalidBootstrapToken) {
		t.Errorf("empty cleartext: err = %v, want ErrInvalidBootstrapToken", err)
	}
	if _, err := svc.ClaimFirstOwner(ctx, "any-non-empty-token", "not-an-email", "correct horse battery staple", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bad email: err = %v, want ErrInvalidInput", err)
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	svc, orgID, _ := newAuthService(t, nil)
	ctx := context.Background()
	token := issueBootstrap(t, svc, orgID, nil)
	if _, err := svc.ClaimFirstOwner(ctx, token, "owner@kelbrook.test", "correct horse battery staple", ""); err != nil {
		t.Fatalf("claim: %v", err)
	}
	_, err := svc.Login(ctx, "owner@kelbrook.test", "wrong password entirely")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthService_Login_UnknownEmail(t *testing.T) {
	svc, _, _ := newAuthService(t, nil)
	_, err := svc.Login(context.Background(), "nobody@kelbrook.test", "any password at all here")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthService_Refresh_RotatesAndInvalidatesOld(t *testing.T) {
	svc, orgID, _ := newAuthService(t, nil)
	ctx := context.Background()
	token := issueBootstrap(t, svc, orgID, nil)
	pair, err := svc.ClaimFirstOwner(ctx, token, "owner@kelbrook.test", "correct horse battery staple", "")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	rotated, err := svc.Refresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if rotated.RefreshToken == pair.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	if rotated.AccessToken == "" {
		t.Fatal("rotated access token is empty")
	}

	// The original refresh token is now revoked.
	if _, err := svc.Refresh(ctx, pair.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("reuse old token err = %v, want ErrInvalidRefreshToken", err)
	}
	// The rotated token still works.
	if _, err := svc.Refresh(ctx, rotated.RefreshToken); err != nil {
		t.Fatalf("refresh with rotated token: %v", err)
	}
}

func TestAuthService_Logout_RevokesAndIsIdempotent(t *testing.T) {
	svc, orgID, _ := newAuthService(t, nil)
	ctx := context.Background()
	token := issueBootstrap(t, svc, orgID, nil)
	pair, err := svc.ClaimFirstOwner(ctx, token, "owner@kelbrook.test", "correct horse battery staple", "")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	if err := svc.Logout(ctx, pair.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	// Token no longer refreshes.
	if _, err := svc.Refresh(ctx, pair.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("refresh after logout err = %v, want ErrInvalidRefreshToken", err)
	}
	// Logging out again is a no-op success.
	if err := svc.Logout(ctx, pair.RefreshToken); err != nil {
		t.Fatalf("second Logout: %v", err)
	}
	// Unknown token is also a no-op.
	if err := svc.Logout(ctx, "never-existed"); err != nil {
		t.Fatalf("Logout unknown: %v", err)
	}
}

func TestAuthService_PasswordReset_RoundTrip(t *testing.T) {
	svc, orgID, mail := newAuthService(t, nil)
	ctx := context.Background()
	token := issueBootstrap(t, svc, orgID, nil)
	pair, err := svc.ClaimFirstOwner(ctx, token, "owner@kelbrook.test", "correct horse battery staple", "")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	if err := svc.RequestPasswordReset(ctx, "owner@kelbrook.test"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	sent, ok := mail.last()
	if !ok {
		t.Fatal("no reset email was sent")
	}
	if sent.Msg.To != "owner@kelbrook.test" {
		t.Errorf("reset email To = %q", sent.Msg.To)
	}

	// Extract the cleartext token from the link in the email.
	resetToken := extractResetToken(t, sent.Msg.TextBody)
	if err := svc.ResetPassword(ctx, resetToken, "a brand new password here"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	// Old password no longer works; new one does.
	if _, err := svc.Login(ctx, "owner@kelbrook.test", "correct horse battery staple"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login old password err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.Login(ctx, "owner@kelbrook.test", "a brand new password here"); err != nil {
		t.Fatalf("login new password: %v", err)
	}

	// All pre-reset refresh tokens were revoked.
	if _, err := svc.Refresh(ctx, pair.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("pre-reset refresh err = %v, want ErrInvalidRefreshToken", err)
	}

	// The reset token is single-use.
	if err := svc.ResetPassword(ctx, resetToken, "yet another password value"); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("reused reset token err = %v, want ErrInvalidResetToken", err)
	}
}

func TestAuthService_RequestPasswordReset_UnknownEmailIsSilentSuccess(t *testing.T) {
	svc, _, mail := newAuthService(t, nil)
	if err := svc.RequestPasswordReset(context.Background(), "nobody@kelbrook.test"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if _, ok := mail.last(); ok {
		t.Fatal("a reset email was sent for an unknown address — enumeration leak")
	}
}

func TestAuthService_ResetPassword_InvalidToken(t *testing.T) {
	svc, _, _ := newAuthService(t, nil)
	err := svc.ResetPassword(context.Background(), "bogus-token", "a perfectly fine password")
	if !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("err = %v, want ErrInvalidResetToken", err)
	}
}

// TestAuthService_InputGuards covers the early input-validation legs that
// short-circuit before any DB work on the three token-consuming flows —
// the branches the happy-path round-trips above never reach. One fixture
// (one container) is shared across all guards since none touch the pool.
func TestAuthService_InputGuards(t *testing.T) {
	svc, _, _ := newAuthService(t, nil)
	ctx := context.Background()

	// Refresh with an empty token is rejected before the lookup tx.
	if _, err := svc.Refresh(ctx, ""); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("Refresh(\"\"): err = %v, want ErrInvalidRefreshToken", err)
	}

	// Logout with an empty token is a no-op success (idempotent goal state).
	if err := svc.Logout(ctx, ""); err != nil {
		t.Errorf("Logout(\"\"): err = %v, want nil", err)
	}

	// ResetPassword with an empty token is rejected before validatePassword.
	if err := svc.ResetPassword(ctx, "", "a perfectly fine password"); !errors.Is(err, ErrInvalidResetToken) {
		t.Errorf("ResetPassword(\"\", ...): err = %v, want ErrInvalidResetToken", err)
	}

	// ResetPassword with a too-short new password hits the validatePassword
	// guard (after the non-empty-token check, before the consume tx).
	if err := svc.ResetPassword(ctx, "some-non-empty-token", "short"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("ResetPassword(tok, short): err = %v, want ErrInvalidInput", err)
	}
}

// TestAuthService_Login_EmptyInputsRejected covers the pre-tx guard the
// WrongPassword/UnknownEmail tests miss: an empty email OR empty password
// short-circuits to ErrInvalidCredentials before any user lookup (the same
// uniform sentinel, so a blank field can't be distinguished from a bad one).
func TestAuthService_Login_EmptyInputsRejected(t *testing.T) {
	svc, _, _ := newAuthService(t, nil)
	ctx := context.Background()
	if _, err := svc.Login(ctx, "   ", "some password here"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("empty email: err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.Login(ctx, "owner@kelbrook.test", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("empty password: err = %v, want ErrInvalidCredentials", err)
	}
}

// TestAuthService_Login_NoPasswordHashRejected covers the legacy/OIDC-row
// leg: a user seeded with a NULL password_hash (no native credential ever
// set) cannot log in via the native path — the found.PasswordHash == ""
// branch returns ErrInvalidCredentials rather than running VerifyPassword
// against an empty hash.
func TestAuthService_Login_NoPasswordHashRejected(t *testing.T) {
	svc, orgID, _ := newAuthService(t, nil)
	userID := uuid.New()
	testdb.SeedUser(t, svc.pool, userID, orgID) // inserts no password_hash
	_, err := svc.Login(context.Background(), userID.String()+"@test.local", "any password at all here")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

// TestAuthService_RequestPasswordReset_EmptyEmailIsNoop covers the empty-email
// short-circuit (returns nil before opening the tx, sends nothing) — distinct
// from the unknown-email no-op which DOES open a tx and find nothing.
func TestAuthService_RequestPasswordReset_EmptyEmailIsNoop(t *testing.T) {
	svc, _, mail := newAuthService(t, nil)
	if err := svc.RequestPasswordReset(context.Background(), "   "); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if _, ok := mail.last(); ok {
		t.Fatal("a reset email was sent for an empty address")
	}
}

// TestAuthService_RequestPasswordReset_MailerFailureStillSucceeds covers the
// best-effort send leg: a mailer outage is swallowed at WARN and the caller
// still sees success (the token was minted; no enumeration via an error). The
// RoundTrip test only ever exercises the happy send.
func TestAuthService_RequestPasswordReset_MailerFailureStillSucceeds(t *testing.T) {
	svc, orgID, mail := newAuthService(t, nil)
	ctx := context.Background()
	token := issueBootstrap(t, svc, orgID, nil)
	if _, err := svc.ClaimFirstOwner(ctx, token, "owner@kelbrook.test", "correct horse battery staple", ""); err != nil {
		t.Fatalf("claim: %v", err)
	}

	mail.setErr(errors.New("smtp upstream down"))
	if err := svc.RequestPasswordReset(ctx, "owner@kelbrook.test"); err != nil {
		t.Fatalf("RequestPasswordReset with failing mailer: err = %v, want nil", err)
	}
	// The send was attempted (and recorded) even though it errored — proof
	// the WARN soft-fail leg ran rather than propagating the error.
	if _, ok := mail.last(); !ok {
		t.Fatal("send was not attempted")
	}
}

// extractResetToken pulls the token query-param value out of the reset link
// embedded in the email text body.
func extractResetToken(t *testing.T, body string) string {
	t.Helper()
	const marker = "token="
	i := indexOf(body, marker)
	if i < 0 {
		t.Fatalf("reset link not found in email body: %q", body)
	}
	tok := body[i+len(marker):]
	// Trim at the first whitespace/newline.
	for j := 0; j < len(tok); j++ {
		if tok[j] == '\n' || tok[j] == ' ' || tok[j] == '\r' {
			return tok[:j]
		}
	}
	return tok
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
