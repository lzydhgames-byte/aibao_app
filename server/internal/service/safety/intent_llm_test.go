package safety

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/metrics"
)

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, c.Write(&m))
	return m.GetCounter().GetValue()
}

func TestLLMIntentProvider_FailFallbackCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	biz := metrics.NewBusiness(reg)

	mockErr := llm.NewMock()
	mockErr.Err = errors.New("boom")
	p1 := NewLLMIntentProvider(mockErr, "doubao-lite", biz)
	_, err := p1.Classify(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, float64(1), counterValue(t, biz.LLMFailFallbackTotal.WithLabelValues("doubao", "doubao-lite", "upstream_error")))

	mockGarbage := llm.NewMock()
	mockGarbage.Response.Text = "garbage"
	p2 := NewLLMIntentProvider(mockGarbage, "doubao-lite", biz)
	_, err = p2.Classify(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, float64(1), counterValue(t, biz.LLMFailFallbackTotal.WithLabelValues("doubao", "doubao-lite", "unparseable")))
}

func TestLLMIntentProvider_Classify_Safe(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "safe"
	p := NewLLMIntentProvider(mock, "doubao-lite", nil)
	got, err := p.Classify(context.Background(), "讲个奥特曼故事")
	require.NoError(t, err)
	assert.Equal(t, IntentSafe, got)
}

func TestLLMIntentProvider_Classify_Unsafe(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "unsafe"
	p := NewLLMIntentProvider(mock, "doubao-lite", nil)
	got, err := p.Classify(context.Background(), "我想要血腥的故事")
	require.NoError(t, err)
	assert.Equal(t, IntentUnsafe, got)
}

func TestLLMIntentProvider_Classify_Uncertain(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "uncertain"
	p := NewLLMIntentProvider(mock, "doubao-lite", nil)
	got, err := p.Classify(context.Background(), "讲个奇怪的故事")
	require.NoError(t, err)
	assert.Equal(t, IntentUncertain, got)
}

func TestLLMIntentProvider_Classify_UnknownDefaultsSafe(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "garbage_response"
	p := NewLLMIntentProvider(mock, "doubao-lite", nil)
	got, err := p.Classify(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, IntentSafe, got, "fallback to safe when unparseable")
}

func TestLLMIntentProvider_LLMErrorReturnsSafe(t *testing.T) {
	mock := llm.NewMock()
	mock.Err = errors.New("upstream down")
	p := NewLLMIntentProvider(mock, "doubao-lite", nil)
	got, err := p.Classify(context.Background(), "x")
	assert.NoError(t, err)
	assert.Equal(t, IntentSafe, got)
}
