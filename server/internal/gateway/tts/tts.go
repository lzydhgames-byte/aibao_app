// Package tts is the TTS provider abstraction. Audio worker depends on this
// interface, not on any concrete provider.
package tts

import (
	"context"
	"errors"
	"time"
)

// ErrTimeout is returned when synthesis exceeds its deadline.
var ErrTimeout = errors.New("tts call timeout")

// ErrUpstream is returned when the TTS provider returned an error.
var ErrUpstream = errors.New("tts upstream error")

// SynthesizeRequest is the structured input.
type SynthesizeRequest struct {
	Text       string
	VoiceID    string
	Model      string
	Format     string
	SampleRate int
	Bitrate    int
	Speed      float64
}

// SynthesizeResponse holds the resulting audio bytes plus metadata.
type SynthesizeResponse struct {
	Audio           []byte
	Format          string
	DurationSeconds int
	Provider        string
	Latency         time.Duration
}

// Client is the TTS provider abstraction.
type Client interface {
	Synthesize(ctx context.Context, req SynthesizeRequest) (*SynthesizeResponse, error)
	HealthCheck(ctx context.Context) error
}
