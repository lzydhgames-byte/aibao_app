package story

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/metrics"
)

const chapterHookSystemPrompt = `你是儿童故事的下集预告员。请用 20 字以内一句话写下一集预告，承接刚才故事的氛围，但不要剧透关键转折。直接给出预告句子本身，不要解释、不要标点收尾。`

// ChapterHookExtractor produces a short next-episode teaser from a finished story.
type ChapterHookExtractor struct {
	client      llm.Client
	model       string
	temperature float64
	biz         *metrics.Business
	logger      *slog.Logger
}

// NewChapterHookExtractor constructs a ChapterHookExtractor.
func NewChapterHookExtractor(client llm.Client, model string, temperature float64, biz *metrics.Business, logger *slog.Logger) *ChapterHookExtractor {
	return &ChapterHookExtractor{client: client, model: model, temperature: temperature, biz: biz, logger: logger}
}

// Extract returns a <=20-char Chinese sentence or "" on any error.
func (e *ChapterHookExtractor) Extract(ctx context.Context, storyText string) string {
	if strings.TrimSpace(storyText) == "" {
		return ""
	}
	start := time.Now()
	out, err := e.client.Generate(ctx, llm.GenerateRequest{
		Model:       e.model,
		Temperature: e.temperature,
		MaxTokens:   60,
		Messages: []llm.Message{
			{Role: "system", Content: chapterHookSystemPrompt},
			{Role: "user", Content: storyText},
		},
	})
	dur := time.Since(start).Seconds()
	if e.biz != nil {
		e.biz.ChapterHookExtractDuration.Observe(dur)
	}
	if err != nil {
		if e.biz != nil {
			e.biz.ChapterHookExtractTotal.WithLabelValues("fail").Inc()
			e.biz.LLMFailFallbackTotal.WithLabelValues("doubao", e.model, "chapter_hook").Inc()
		}
		if e.logger != nil {
			e.logger.Warn("story.chapter_hook.fail", "err", err)
		}
		return ""
	}
	if e.biz != nil {
		e.biz.ChapterHookExtractTotal.WithLabelValues("ok").Inc()
	}
	return truncateChinese(strings.TrimSpace(out.Text), 20)
}

// truncateChinese trims to N runes if longer (soft cap).
func truncateChinese(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n])
}
