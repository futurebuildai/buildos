package futureshade

import (
	"testing"
	"time"
)

// =============================================================================
// Config Tests
// =============================================================================

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{}
	if cfg.Enabled {
		t.Error("default Config.Enabled should be false")
	}
	if cfg.APIKey != "" {
		t.Error("default Config.APIKey should be empty")
	}
	if cfg.ModelID != "" {
		t.Error("default Config.ModelID should be empty")
	}
}

func TestConfig_WithValues(t *testing.T) {
	cfg := Config{
		APIKey:  "test-api-key",
		ModelID: "claude-opus-4-6",
		Enabled: true,
	}
	if !cfg.Enabled {
		t.Error("Config.Enabled should be true")
	}
	if cfg.APIKey != "test-api-key" {
		t.Errorf("Config.APIKey = %q, want 'test-api-key'", cfg.APIKey)
	}
	if cfg.ModelID != "claude-opus-4-6" {
		t.Errorf("Config.ModelID = %q, want 'claude-opus-4-6'", cfg.ModelID)
	}
}

// =============================================================================
// ShadowDoc Type Tests
// =============================================================================

func TestShadowDoc_Construction(t *testing.T) {
	now := time.Now().UTC()
	doc := ShadowDoc{
		ID:          "doc-123",
		SourceType:  "PRD",
		SourceID:    "/path/to/file.md",
		ContentHash: "sha256:abc123",
		Analysis: map[string]interface{}{
			"score":    0.85,
			"category": "requirements",
		},
		CreatedAt: now,
	}

	if doc.ID != "doc-123" {
		t.Errorf("ID = %q, want 'doc-123'", doc.ID)
	}
	if doc.SourceType != "PRD" {
		t.Errorf("SourceType = %q, want 'PRD'", doc.SourceType)
	}
	if doc.SourceID != "/path/to/file.md" {
		t.Errorf("SourceID = %q, want '/path/to/file.md'", doc.SourceID)
	}
	if doc.ContentHash != "sha256:abc123" {
		t.Errorf("ContentHash = %q, want 'sha256:abc123'", doc.ContentHash)
	}
	if len(doc.Analysis) != 2 {
		t.Errorf("Analysis has %d entries, want 2", len(doc.Analysis))
	}
	if doc.CreatedAt != now {
		t.Errorf("CreatedAt mismatch")
	}
}

func TestShadowDoc_EmptyAnalysis(t *testing.T) {
	doc := ShadowDoc{
		ID:         "empty-doc",
		SourceType: "Code",
	}
	if doc.Analysis != nil {
		t.Error("expected nil Analysis for uninitialized doc")
	}
}

func TestShadowDoc_SupportedSourceTypes(t *testing.T) {
	sourceTypes := []string{"PRD", "Spec", "Code"}
	for _, st := range sourceTypes {
		doc := ShadowDoc{SourceType: st}
		if doc.SourceType != st {
			t.Errorf("SourceType = %q, want %q", doc.SourceType, st)
		}
	}
}

// =============================================================================
// Service Tests
// =============================================================================

func TestNewService_NilConfig_ReturnsDisabledService(t *testing.T) {
	svc := NewService(nil)
	if svc == nil {
		t.Fatal("NewService(nil) returned nil")
	}
	if svc.IsEnabled() {
		t.Error("service should be disabled when config is nil")
	}
}

func TestNewService_DisabledConfig(t *testing.T) {
	cfg := &Config{Enabled: false}
	svc := NewService(cfg)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.IsEnabled() {
		t.Error("service should be disabled")
	}
}

func TestNewService_EnabledConfig(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		APIKey:  "test-key",
		ModelID: "claude-opus-4-6",
	}
	svc := NewService(cfg)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if !svc.IsEnabled() {
		t.Error("service should be enabled")
	}
}

// =============================================================================
// Health Check Tests
// =============================================================================

func TestService_Health_Disabled(t *testing.T) {
	svc := NewService(&Config{Enabled: false})
	err := svc.Health()
	if err == nil {
		t.Fatal("expected error from disabled service Health()")
	}
	if err.Error() != "futureshade service is disabled" {
		t.Errorf("error = %q, want 'futureshade service is disabled'", err.Error())
	}
}

func TestService_Health_MissingAPIKey(t *testing.T) {
	svc := NewService(&Config{Enabled: true, APIKey: ""})
	err := svc.Health()
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if err.Error() != "futureshade service configuration missing API key" {
		t.Errorf("error = %q, want specific API key error", err.Error())
	}
}

func TestService_Health_FullyConfigured(t *testing.T) {
	svc := NewService(&Config{
		Enabled: true,
		APIKey:  "sk-valid-key",
		ModelID: "claude-opus-4-6",
	})
	err := svc.Health()
	if err != nil {
		t.Errorf("unexpected error from healthy service: %v", err)
	}
}

// =============================================================================
// Fail-Open Strategy Tests
// =============================================================================

func TestService_FailOpen_NilConfig(t *testing.T) {
	// Fail-open: nil config should not panic, should return a working (disabled) service
	svc := NewService(nil)
	if svc.IsEnabled() {
		t.Error("nil config should produce disabled service (fail-open)")
	}
	err := svc.Health()
	if err == nil {
		t.Error("disabled service should return health error")
	}
}

func TestService_FailOpen_EmptyConfig(t *testing.T) {
	// Fail-open: empty config should not panic, should be disabled
	svc := NewService(&Config{})
	if svc.IsEnabled() {
		t.Error("empty config should produce disabled service (fail-open)")
	}
}

func TestService_FailOpen_EnabledButNoKey(t *testing.T) {
	// Even if enabled, missing API key means health check fails
	// but service creation doesn't panic
	svc := NewService(&Config{Enabled: true})
	if !svc.IsEnabled() {
		t.Error("IsEnabled should return true even without API key")
	}
	err := svc.Health()
	if err == nil {
		t.Error("expected health error for missing API key")
	}
}

// =============================================================================
// Interface Compliance
// =============================================================================

func TestService_ImplementsServiceInterface(t *testing.T) {
	var _ Service = NewService(nil)
}
