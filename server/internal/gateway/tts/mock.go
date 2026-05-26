package tts

import (
	"bytes"
	"context"
	"errors"
	"time"
	"unicode/utf8"
)

// MockClient is a deterministic in-memory TTS for tests.
type MockClient struct {
	failNext bool
	calls    int
}

// NewMock constructs a MockClient.
func NewMock() *MockClient { return &MockClient{} }

// FailNext makes the next Synthesize call return ErrUpstream.
func (m *MockClient) FailNext() { m.failNext = true }

// Calls returns how many Synthesize calls were made.
func (m *MockClient) Calls() int { return m.calls }

// Synthesize returns fake bytes proportional to text length.
func (m *MockClient) Synthesize(_ context.Context, req SynthesizeRequest) (*SynthesizeResponse, error) {
	m.calls++
	if m.failNext {
		m.failNext = false
		return nil, errors.New("mock: forced failure: " + ErrUpstream.Error())
	}
	if req.Text == "" {
		return nil, errors.New("tts: empty text")
	}
	var buf bytes.Buffer
	buf.WriteString("MP3 ")
	buf.WriteString(req.Text)
	return &SynthesizeResponse{
		Audio:           buf.Bytes(),
		Format:          req.Format,
		DurationSeconds: len([]rune(req.Text)) / 4,
		CharCount:       utf8.RuneCountInString(req.Text),
		Provider:        "mock",
		Latency:         5 * time.Millisecond,
	}, nil
}

// HealthCheck always passes for mock.
func (m *MockClient) HealthCheck(_ context.Context) error { return nil }
