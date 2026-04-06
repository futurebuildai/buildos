package ai

import (
	"context"
	"fmt"
	"sync"
)

// MockClient is a thread-safe mock for the AI Client interface.
// Uses mutex to prevent data races on spy slices.
type MockClient struct {
	mu sync.Mutex

	// Mock responses
	GenerateResponse *GenerateResponse
	GenerateError    error

	// Spies
	GenerateCalls  []GenerateRequest
	EmbeddingCalls []string

	EmbeddingResponse []float32
	EmbeddingErr      error
}

// GenerateEmbedding records the call and returns the pre-configured response.
// Thread-safe via mutex.
func (m *MockClient) GenerateEmbedding(_ context.Context, text string) ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EmbeddingCalls = append(m.EmbeddingCalls, text)
	return m.EmbeddingResponse, m.EmbeddingErr
}

// GenerateContent records the call and returns the pre-configured response.
// Thread-safe via mutex.
func (m *MockClient) GenerateContent(_ context.Context, req GenerateRequest) (GenerateResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GenerateCalls = append(m.GenerateCalls, req)
	if m.GenerateError != nil {
		return GenerateResponse{}, m.GenerateError
	}
	if m.GenerateResponse == nil {
		return GenerateResponse{}, fmt.Errorf("mock response not configured")
	}
	return *m.GenerateResponse, nil
}

// Close is a no-op for the mock.
func (m *MockClient) Close() error {
	return nil
}

// SetResponse is a convenience method to set the mock response.
// Thread-safe via mutex.
func (m *MockClient) SetResponse(resp GenerateResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GenerateResponse = &resp
}

// SetError is a convenience method to set the mock error.
// Thread-safe via mutex.
func (m *MockClient) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GenerateError = err
}
