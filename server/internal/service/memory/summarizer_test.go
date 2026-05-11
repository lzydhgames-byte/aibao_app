package memory

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/metrics"
)

func counterVal(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, c.Write(&m))
	return m.GetCounter().GetValue()
}

func newTestBiz() *metrics.Business {
	return metrics.NewBusiness(prometheus.NewRegistry())
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSummarizer_HappyPath(t *testing.T) {
	mock := &llm.MockClient{Response: &llm.GenerateResponse{Text: "小宇和爱宝救了小恐龙，学会了勇敢"}}
	s := NewSummarizer(mock, "test", 0.2, newTestBiz(), quietLogger())
	got := s.Summarize(context.Background(), "故事文本")
	assert.Equal(t, "小宇和爱宝救了小恐龙，学会了勇敢", got)
	assert.Equal(t, 1, mock.Calls)
}

func TestSummarizer_TruncatesLongOutput(t *testing.T) {
	long := "啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊"
	mock := &llm.MockClient{Response: &llm.GenerateResponse{Text: long}}
	s := NewSummarizer(mock, "test", 0.2, newTestBiz(), quietLogger())
	got := s.Summarize(context.Background(), "故事文本")
	assert.Equal(t, 30, len([]rune(got)))
}

func TestSummarizer_LLMErrorReturnsEmpty(t *testing.T) {
	mock := &llm.MockClient{Err: errors.New("boom")}
	biz := metrics.NewBusiness(prometheus.NewRegistry())
	s := NewSummarizer(mock, "test", 0.2, biz, quietLogger())
	got := s.Summarize(context.Background(), "故事文本")
	assert.Equal(t, "", got)
	assert.Equal(t, float64(1), counterVal(t, biz.LLMFailFallbackTotal.WithLabelValues("doubao", "test", "upstream_error")))
}

func TestSummarizer_EmptyInputSkipsLLM(t *testing.T) {
	mock := &llm.MockClient{Response: &llm.GenerateResponse{Text: "x"}}
	s := NewSummarizer(mock, "test", 0.2, newTestBiz(), quietLogger())
	got := s.Summarize(context.Background(), "   ")
	require.Equal(t, "", got)
	assert.Equal(t, 0, mock.Calls)
}
