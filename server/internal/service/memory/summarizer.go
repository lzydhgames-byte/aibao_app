// Package memory provides post-story memory writing (summarizer) and
// pre-story memory injection (selector). Both layers are deliberately
// fail-open: a failure here must NEVER block story generation.
package memory

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/metrics"
)

const summarizerSystemPrompt = `你是一个儿童故事总结器。请用一句不超过 30 个汉字的中文，总结下面这个儿童故事的主要情节和情感。只输出这句话本身，不要加引号、不要解释、不要其他说明。`

// Summarizer turns a finished story text into a single-sentence memory.
type Summarizer struct {
	client      llm.Client
	model       string
	temperature float64
	biz         *metrics.Business
	logger      *slog.Logger
}

// NewSummarizer constructs a Summarizer.
func NewSummarizer(client llm.Client, model string, temperature float64, biz *metrics.Business, logger *slog.Logger) *Summarizer {
	return &Summarizer{client: client, model: model, temperature: temperature, biz: biz, logger: logger}
}

// Summarize returns a <=30-char Chinese sentence or "" on any error.
func (s *Summarizer) Summarize(ctx context.Context, storyText string) string {
	if strings.TrimSpace(storyText) == "" {
		return ""
	}
	start := time.Now()
	out, err := s.client.Generate(ctx, llm.GenerateRequest{
		Model:       s.model,
		Temperature: s.temperature,
		MaxTokens:   80,
		Messages: []llm.Message{
			{Role: "system", Content: summarizerSystemPrompt},
			{Role: "user", Content: storyText},
		},
	})
	dur := time.Since(start).Seconds()
	if s.biz != nil {
		s.biz.MemorySummaryDuration.Observe(dur)
	}
	if err != nil {
		if s.biz != nil {
			s.biz.MemorySummaryTotal.WithLabelValues("fail").Inc()
		}
		if s.logger != nil {
			s.logger.Warn("memory.summarize.fail", "err", err)
		}
		return ""
	}
	if s.biz != nil {
		s.biz.MemorySummaryTotal.WithLabelValues("ok").Inc()
	}
	return truncateChinese(strings.TrimSpace(out.Text), 30)
}

// truncateChinese trims to N runes if longer (soft cap).
func truncateChinese(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n])
}
