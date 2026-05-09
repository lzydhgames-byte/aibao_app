package llm

import (
	"context"
	"errors"
	"time"
)

// MockClient is the test/dev LLM client. It returns a configurable response
// or error and counts calls.
type MockClient struct {
	Response *GenerateResponse
	Err      error
	Calls    int
}

// NewMock returns a MockClient that always returns a placeholder story.
func NewMock() *MockClient {
	return &MockClient{
		Response: &GenerateResponse{
			Text:         "（Mock）小宇推开了门，看到爱宝在竹林里挥手。小宇决定走过去，开启一场冒险。",
			InputTokens:  100,
			OutputTokens: 50,
			Provider:     "mock",
			Model:        "mock",
			Latency:      10 * time.Millisecond,
		},
	}
}

// Generate returns the configured Response (or Err).
func (m *MockClient) Generate(_ context.Context, _ GenerateRequest) (*GenerateResponse, error) {
	m.Calls++
	if m.Err != nil {
		return nil, m.Err
	}
	if m.Response == nil {
		return nil, errors.New("mock not configured")
	}
	return m.Response, nil
}

// HealthCheck always succeeds for Mock.
func (m *MockClient) HealthCheck(_ context.Context) error { return nil }
