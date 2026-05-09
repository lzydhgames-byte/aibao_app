package safety

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/gateway/llm"
)

func TestLLMIntentProvider_Classify_Safe(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "safe"
	p := NewLLMIntentProvider(mock, "doubao-lite")
	got, err := p.Classify(context.Background(), "讲个奥特曼故事")
	require.NoError(t, err)
	assert.Equal(t, IntentSafe, got)
}

func TestLLMIntentProvider_Classify_Unsafe(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "unsafe"
	p := NewLLMIntentProvider(mock, "doubao-lite")
	got, err := p.Classify(context.Background(), "我想要血腥的故事")
	require.NoError(t, err)
	assert.Equal(t, IntentUnsafe, got)
}

func TestLLMIntentProvider_Classify_Uncertain(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "uncertain"
	p := NewLLMIntentProvider(mock, "doubao-lite")
	got, err := p.Classify(context.Background(), "讲个奇怪的故事")
	require.NoError(t, err)
	assert.Equal(t, IntentUncertain, got)
}

func TestLLMIntentProvider_Classify_UnknownDefaultsSafe(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "garbage_response"
	p := NewLLMIntentProvider(mock, "doubao-lite")
	got, err := p.Classify(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, IntentSafe, got, "fallback to safe when unparseable")
}

func TestLLMIntentProvider_LLMErrorReturnsSafe(t *testing.T) {
	mock := llm.NewMock()
	mock.Err = errors.New("upstream down")
	p := NewLLMIntentProvider(mock, "doubao-lite")
	got, err := p.Classify(context.Background(), "x")
	assert.NoError(t, err)
	assert.Equal(t, IntentSafe, got)
}
