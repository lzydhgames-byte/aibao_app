package llm

import (
	"context"
	"errors"
	"time"
)

// ErrTimeout is returned when an LLM call exceeds its deadline.
var ErrTimeout = errors.New("llm call timeout")

// ErrUpstream is returned when the LLM provider returned an error.
var ErrUpstream = errors.New("llm upstream error")

// Message is one turn of the chat (system / user / assistant).
type Message struct {
	Role    string // "system" / "user" / "assistant"
	Content string
}

// GenerateRequest is the structured input to a chat completion call.
type GenerateRequest struct {
	Model       string
	Messages    []Message
	MaxTokens   int     // 0 means "let provider default"
	Temperature float64 // 0..2
}

// GenerateResponse is the structured output.
type GenerateResponse struct {
	Text         string
	InputTokens  int           // Plan 11B Usage data: business pulls these into pkgcost.Usage and calls service/cost/Recorder.Record themselves
	OutputTokens int
	Provider     string
	Model        string
	Latency      time.Duration
}

// Client is the LLM provider abstraction. Story service depends on this
// interface, not on any concrete provider.
type Client interface {
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	HealthCheck(ctx context.Context) error
}
