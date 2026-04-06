package futureshade

import (
	"errors"
	"fmt"
)

// Service defines the interface for the FutureShade service.
type Service interface {
	// Health checks the status of the service.
	Health() error

	// IsEnabled returns whether the service is active.
	IsEnabled() bool
}

// service implements the Service interface.
type service struct {
	cfg *Config
}

// NewService creates a new FutureShade service.
// It implements a "Fail Open" strategy: if configuration is missing or invalid,
// it returns a service in disabled state, ensuring the main application can still start.
func NewService(cfg *Config) Service {
	if cfg == nil {
		// Return a safe default with empty config (effectively disabled)
		return &service{cfg: &Config{Enabled: false}}
	}
	return &service{cfg: cfg}
}

// Health checks if the service is healthy and configured.
// Returns nil if healthy (Active).
// Returns error if disabled or missing keys (Service Unavailable context).
func (s *service) Health() error {
	if !s.cfg.Enabled {
		return errors.New("futureshade service is disabled")
	}
	if s.cfg.APIKey == "" {
		return fmt.Errorf("futureshade service configuration missing API key")
	}
	return nil
}

// IsEnabled returns whether FutureShade is active.
func (s *service) IsEnabled() bool {
	return s.cfg.Enabled
}
