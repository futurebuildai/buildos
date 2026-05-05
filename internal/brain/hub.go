package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
)

// HubClient wraps Brain's per-tenant 3rd-party credential vault per
// ADR-001 D6. Brain holds Gable/LocalBlue/Twilio/etc credentials in
// an AES-256-GCM vault keyed by org. BuildOS forks reach upstreams
// in one of two modes:
//
//   - Proxy mode (default): BuildOS asks Brain to make the upstream
//     call. GetCredential returns a credential handle (id +
//     provider + expires_at), no decrypted secret crosses the
//     wire. Subsequent upstream calls go through Brain's proxy
//     endpoint, which resolves the credential server-side.
//
//   - Direct mode (BRAIN_HUB_DIRECT=true on a fork): BuildOS
//     receives the decrypted credential and makes upstream calls
//     itself. Used by forks running in regulated environments
//     where the proxy hop is not acceptable.
//
// The mode is fork-static (set at NewClient time via Config) — not
// per-request — so an audit reading "Brain returned a decrypted
// secret" can be traced back to the fork's deployment posture, not
// to a runtime path.
type HubClient struct {
	c          *Client
	directMode bool
}

// Credential is one stored 3rd-party credential's metadata. The
// Secret field is populated in direct mode ONLY; in proxy mode
// (default), only the metadata + ProxyHandle are returned and
// Secret is empty.
type Credential struct {
	ID        uuid.UUID `json:"id"`
	Provider  string    `json:"provider"` // "gable" | "localblue" | "twilio" | ...
	Scope     string    `json:"scope"`    // "default" | "<custom>"
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// Secret is the decrypted credential value. Populated in
	// direct mode only. Treat as Restricted PII per
	// internal/pii — never log, never serialize into audit
	// rows, never include in error messages.
	Secret string `json:"secret,omitempty"`

	// ProxyHandle is the opaque token Brain returns in proxy
	// mode. Pass it back to Brain on upstream-proxy calls
	// (`POST /api/hub/proxy/<provider>/...`) so Brain can
	// resolve the right credential server-side.
	ProxyHandle string `json:"proxy_handle,omitempty"`
}

// GetCredentialRequest is the input for GetCredential. Scope
// defaults to "default" when empty — most providers only need one
// credential per org, but a fork can register multiple scopes
// (e.g., "production" / "sandbox") per provider.
type GetCredentialRequest struct {
	Provider string
	Scope    string
}

// GetCredential fetches the credential metadata (and, in direct
// mode, the decrypted secret) for the given provider/scope. Returns
// ErrNotFound if no matching credential exists.
//
// In proxy mode (default), the returned Credential.Secret is empty
// and Credential.ProxyHandle should be passed to subsequent Brain
// proxy calls. In direct mode, Credential.Secret carries the
// decrypted value.
func (h *HubClient) GetCredential(ctx context.Context, req GetCredentialRequest) (*Credential, error) {
	if req.Provider == "" {
		return nil, fmt.Errorf("brain.Hub.GetCredential: provider is required")
	}
	scope := req.Scope
	if scope == "" {
		scope = "default"
	}

	path := "/api/hub/credentials/" + url.PathEscape(req.Provider) + "/" + url.PathEscape(scope)
	if h.directMode {
		// Brain interprets ?mode=direct as "the fork is
		// authorized to receive the decrypted secret". Brain's
		// own policy gate decides whether to honor it; if not,
		// the response includes a proxy handle and Secret is
		// empty regardless of mode.
		path += "?mode=direct"
	}

	raw, err := h.c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var cred Credential
	if err := json.Unmarshal(raw, &cred); err != nil {
		return nil, fmt.Errorf("brain.Hub.GetCredential: decode response: %w", err)
	}
	return &cred, nil
}

// RefreshIfExpired asks Brain to refresh the credential's OAuth
// tokens (or rotate API keys, depending on provider). Brain checks
// the expiry server-side and no-ops if the credential is still
// valid — the "if expired" semantics are server-side, this method
// just kicks the operation.
//
// Returns nil on success (whether or not Brain actually refreshed),
// ErrNotFound if the credential ID doesn't exist, or wrapped
// ErrTransient for upstream-OAuth failures (Brain returns 502).
func (h *HubClient) RefreshIfExpired(ctx context.Context, credentialID uuid.UUID) error {
	if credentialID == uuid.Nil {
		return fmt.Errorf("brain.Hub.RefreshIfExpired: credential_id is required")
	}
	_, err := h.c.doRequest(ctx, "POST", "/api/hub/credentials/"+credentialID.String()+"/refresh", nil)
	return err
}
