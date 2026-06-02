//go:build integration

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/cryptobox"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// newVaultService constructs a VaultService bound to a fresh pool, a
// test cipher (32-byte all-zero key, version 1), a no-op audit
// recorder, and a seeded org. Helper for every test in this file.
func newVaultService(t *testing.T) (*VaultService, uuid.UUID) {
	t.Helper()
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	masterKey := make([]byte, 32) // 32 bytes = AES-256; deterministic for tests
	cipher, err := cryptobox.NewCipher(masterKey, 1)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	svc := NewVaultService(pool, store.NewIntegrationCredentialStore(), cipher, NewNoopAuditRecorder(), nil, nil)
	return svc, orgID
}

func TestVaultService_SetCredential_RoundTrips(t *testing.T) {
	svc, orgID := newVaultService(t)
	ctx := context.Background()

	const key = "sk-ant-test-abcd1234"
	cred, err := svc.SetCredential(ctx, SetCredentialInput{
		OrgID:    orgID,
		Provider: ProviderAnthropic,
		Label:    "Primary Anthropic",
		Key:      key,
		UserSub:  "owner-1",
	})
	if err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	if !cred.IsActive {
		t.Error("credential should be active")
	}
	if cred.Last4 != "1234" {
		t.Errorf("Last4 = %q, want 1234", cred.Last4)
	}
	// The returned model must not leak the cleartext anywhere; the
	// stored bytes are the ciphertext, not the key.
	if string(cred.Ciphertext) == key {
		t.Error("ciphertext equals cleartext — not encrypted")
	}

	// Resolver returns the original cleartext.
	got, err := svc.AnthropicKey(ctx, orgID.String())
	if err != nil {
		t.Fatalf("AnthropicKey: %v", err)
	}
	if got != key {
		t.Errorf("AnthropicKey = %q, want %q", got, key)
	}
}

func TestVaultService_SetCredential_Rotates(t *testing.T) {
	svc, orgID := newVaultService(t)
	ctx := context.Background()

	if _, err := svc.SetCredential(ctx, SetCredentialInput{
		OrgID: orgID, Provider: ProviderAnthropic, Key: "sk-ant-old-0000",
	}); err != nil {
		t.Fatalf("first SetCredential: %v", err)
	}
	if _, err := svc.SetCredential(ctx, SetCredentialInput{
		OrgID: orgID, Provider: ProviderAnthropic, Key: "sk-ant-new-9999",
	}); err != nil {
		t.Fatalf("second SetCredential: %v", err)
	}

	// Active key is the newest.
	got, err := svc.AnthropicKey(ctx, orgID.String())
	if err != nil {
		t.Fatalf("AnthropicKey: %v", err)
	}
	if got != "sk-ant-new-9999" {
		t.Errorf("AnthropicKey = %q, want sk-ant-new-9999", got)
	}

	// Exactly one row is active; the old row is retained but inactive.
	creds, err := svc.ListCredentials(ctx, orgID)
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(creds) != 2 {
		t.Fatalf("len(creds) = %d, want 2 (old inactive + new active)", len(creds))
	}
	active := 0
	for _, c := range creds {
		if c.IsActive {
			active++
		}
	}
	if active != 1 {
		t.Errorf("active count = %d, want 1", active)
	}
}

func TestVaultService_DeleteCredential_SoftFailsResolver(t *testing.T) {
	svc, orgID := newVaultService(t)
	ctx := context.Background()

	if _, err := svc.SetCredential(ctx, SetCredentialInput{
		OrgID: orgID, Provider: ProviderAnthropic, Key: "sk-ant-test-1234",
	}); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	if err := svc.DeleteCredential(ctx, orgID, ProviderAnthropic, "owner-1"); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}

	// After delete, the resolver soft-fails to "".
	got, err := svc.AnthropicKey(ctx, orgID.String())
	if err != nil {
		t.Fatalf("AnthropicKey after delete: %v", err)
	}
	if got != "" {
		t.Errorf("AnthropicKey = %q, want \"\" after delete", got)
	}

	// Deleting again (nothing active) is ErrNotFound.
	err = svc.DeleteCredential(ctx, orgID, ProviderAnthropic, "owner-1")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("second DeleteCredential err = %v, want ErrNotFound", err)
	}
}

func TestVaultService_AnthropicKey_NoCredential_SoftFails(t *testing.T) {
	svc, orgID := newVaultService(t)
	got, err := svc.AnthropicKey(context.Background(), orgID.String())
	if err != nil {
		t.Fatalf("AnthropicKey: %v", err)
	}
	if got != "" {
		t.Errorf("AnthropicKey = %q, want \"\" for org with no credential", got)
	}
}

func TestVaultService_ResendKey_RoundTrips(t *testing.T) {
	svc, orgID := newVaultService(t)
	ctx := context.Background()

	const key = "re_test_key_wxyz"
	if _, err := svc.SetCredential(ctx, SetCredentialInput{
		OrgID: orgID, Provider: ProviderResend, Key: key,
	}); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	got, err := svc.ResendKey(ctx, orgID.String())
	if err != nil {
		t.Fatalf("ResendKey: %v", err)
	}
	if got != key {
		t.Errorf("ResendKey = %q, want %q", got, key)
	}
}

func TestVaultService_Capabilities_FlipsWithCredentials(t *testing.T) {
	svc, orgID := newVaultService(t)
	ctx := context.Background()

	// Fresh org: nothing configured → both flags off, both known providers
	// present and unconfigured.
	caps, err := svc.Capabilities(ctx, orgID)
	if err != nil {
		t.Fatalf("Capabilities (empty): %v", err)
	}
	if caps.AIConfigured || caps.EmailConfigured {
		t.Errorf("empty org should have both off, got %+v", caps)
	}
	if len(caps.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(caps.Providers))
	}

	// Set an Anthropic key → ai flips on, email stays off.
	if _, err := svc.SetCredential(ctx, SetCredentialInput{
		OrgID: orgID, Provider: ProviderAnthropic, Key: "sk-ant-test-1234", UserSub: "owner-1",
	}); err != nil {
		t.Fatalf("SetCredential anthropic: %v", err)
	}
	caps, err = svc.Capabilities(ctx, orgID)
	if err != nil {
		t.Fatalf("Capabilities (anthropic set): %v", err)
	}
	if !caps.AIConfigured {
		t.Errorf("ai should flip on after anthropic set, got %+v", caps)
	}
	if caps.EmailConfigured {
		t.Errorf("email should stay off, got %+v", caps)
	}
	// The configured provider surfaces non-secret metadata (fingerprint=last4).
	var anthropic *ProviderCapability
	for i := range caps.Providers {
		if caps.Providers[i].Provider == ProviderAnthropic {
			anthropic = &caps.Providers[i]
		}
	}
	if anthropic == nil || !anthropic.Configured {
		t.Fatalf("anthropic should be configured: %+v", caps.Providers)
	}
	if anthropic.Fingerprint != "1234" {
		t.Errorf("fingerprint = %q, want 1234 (last4)", anthropic.Fingerprint)
	}
	if anthropic.CreatedBy != "owner-1" || anthropic.CreatedAt.IsZero() {
		t.Errorf("configured provider missing created_*: %+v", anthropic)
	}

	// Delete the Anthropic key → ai flips back off.
	if err := svc.DeleteCredential(ctx, orgID, ProviderAnthropic, "owner-1"); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}
	caps, err = svc.Capabilities(ctx, orgID)
	if err != nil {
		t.Fatalf("Capabilities (anthropic deleted): %v", err)
	}
	if caps.AIConfigured {
		t.Errorf("ai should flip off after delete, got %+v", caps)
	}
}

func TestVaultService_Capabilities_OrgIsolation(t *testing.T) {
	svc, orgID := newVaultService(t)
	ctx := context.Background()

	// A second org in the same pool with its own Anthropic key. The pool is
	// shared (same VaultService), so this proves Capabilities filters by org.
	otherOrg := uuid.New()
	testdb.SeedOrg(t, svc.pool, otherOrg, "Other Co")
	if _, err := svc.SetCredential(ctx, SetCredentialInput{
		OrgID: otherOrg, Provider: ProviderAnthropic, Key: "sk-ant-other-5678", UserSub: "owner-2",
	}); err != nil {
		t.Fatalf("SetCredential other org: %v", err)
	}

	// The first org sees nothing — the other org's key must not bleed through.
	caps, err := svc.Capabilities(ctx, orgID)
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if caps.AIConfigured {
		t.Errorf("org isolation breached: org sees another org's key: %+v", caps)
	}

	// The other org sees its own key.
	otherCaps, err := svc.Capabilities(ctx, otherOrg)
	if err != nil {
		t.Fatalf("Capabilities other org: %v", err)
	}
	if !otherCaps.AIConfigured {
		t.Errorf("other org should see its own key: %+v", otherCaps)
	}
}

func TestVaultService_ListCredentials_MetadataOnly(t *testing.T) {
	svc, orgID := newVaultService(t)
	ctx := context.Background()

	const key = "sk-ant-secret-9911"
	if _, err := svc.SetCredential(ctx, SetCredentialInput{
		OrgID: orgID, Provider: ProviderAnthropic, Label: "Main", Key: key,
	}); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	creds, err := svc.ListCredentials(ctx, orgID)
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("len(creds) = %d, want 1", len(creds))
	}
	c := creds[0]
	if c.Last4 != "9911" {
		t.Errorf("Last4 = %q, want 9911", c.Last4)
	}
	// The cleartext must never appear in the listed metadata.
	if string(c.Ciphertext) == key {
		t.Error("ciphertext equals cleartext — not encrypted")
	}
	if c.Label != "Main" {
		t.Errorf("Label = %q, want Main", c.Label)
	}
}
