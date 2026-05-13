package story

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

func TestExtract_Success_TrimmedTo20(t *testing.T) {
	long := "啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊啊"
	mock := &llm.MockClient{Response: &llm.GenerateResponse{Text: long}}
	e := NewChapterHookExtractor(mock, "test", 0.4, newTestBiz(), quietLogger())
	got := e.Extract(context.Background(), "故事正文")
	assert.Equal(t, 20, len([]rune(got)))
	assert.Equal(t, 1, mock.Calls)
}

func TestExtract_LLMError_ReturnsEmpty(t *testing.T) {
	mock := &llm.MockClient{Err: errors.New("boom")}
	biz := metrics.NewBusiness(prometheus.NewRegistry())
	e := NewChapterHookExtractor(mock, "test", 0.4, biz, quietLogger())
	got := e.Extract(context.Background(), "故事正文")
	assert.Equal(t, "", got)
	assert.Equal(t, float64(1), counterVal(t, biz.LLMFailFallbackTotal.WithLabelValues("doubao", "test", "chapter_hook")))
	assert.Equal(t, float64(1), counterVal(t, biz.ChapterHookExtractTotal.WithLabelValues("fail")))
}

func TestExtract_EmptyInput_NoLLMCall(t *testing.T) {
	mock := &llm.MockClient{Response: &llm.GenerateResponse{Text: "x"}}
	e := NewChapterHookExtractor(mock, "test", 0.4, newTestBiz(), quietLogger())
	got := e.Extract(context.Background(), "   ")
	require.Equal(t, "", got)
	assert.Equal(t, 0, mock.Calls)
}
