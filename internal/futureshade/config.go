package futureshade

// Config holds configuration for the FutureShade service.
type Config struct {
	// APIKey is the API key for the Anthropic AI provider.
	APIKey string
	// ModelID is the default model ID to use for analysis.
	ModelID string
	// Enabled determines if the service is active.
	// When false, all operations pass through (fail-open).
	Enabled bool
}
