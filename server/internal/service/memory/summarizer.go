// Package memory provides post-story memory writing (summarizer) and
// pre-story memory injection (selector). Both layers are deliberately
// fail-open: a failure here must NEVER block story generation.
package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/metrics"
	pkgcost "github.com/aibao/server/internal/pkg/cost"
	"github.com/aibao/server/internal/pkg/idhash"
	costsvc "github.com/aibao/server/internal/service/cost"
)

const summarizerSystemPrompt = `你是一个儿童故事总结器。请用一句不超过 30 个汉字的中文，总结下面这个儿童故事的主要情节和情感。只输出这句话本身，不要加引号、不要解释、不要其他说明。`

// Summarizer turns a finished story text into a single-sentence memory.
type Summarizer struct {
	client      llm.Client
	model       string
	temperature float64
	biz         *metrics.Business
	logger      *slog.Logger
	recorder    *costsvc.Recorder // Plan 11B (nil-safe)
	hasher      *idhash.Hasher    // Plan 11B (nil-safe)
}

// WithCost wires Plan 11B cost recording. Returns receiver for chaining.
func (s *Summarizer) WithCost(r *costsvc.Recorder, h *idhash.Hasher) *Summarizer {
	s.recorder = r
	s.hasher = h
	return s
}

// NewSummarizer constructs a Summarizer.
func NewSummarizer(client llm.Client, model string, temperature float64, biz *metrics.Business, logger *slog.Logger) *Summarizer {
	return &Summarizer{client: client, model: model, temperature: temperature, biz: biz, logger: logger}
}

// Summarize returns a <=30-char Chinese sentence or "" on any error. Wraps
// SummarizeForStory with zero IDs (no cost recording).
func (s *Summarizer) Summarize(ctx context.Context, storyText string) string {
	return s.SummarizeForStory(ctx, storyText, 0, 0, nil, "")
}

// SummarizeForStory is the cost-aware variant used by the memory_update worker.
// traceHex must be the 8+hex prefix already stripped of "tr-".
func (s *Summarizer) SummarizeForStory(ctx context.Context, storyText string, childID, storyID int64, userID *int64, traceHex string) string {
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
	if s.recorder != nil && out != nil && traceHex != "" {
		outcome := "ok"
		if err != nil {
			outcome = "fail"
		}
		childHash := ""
		if s.hasher != nil && childID > 0 {
			childHash = s.hasher.Hash("child", childID)
		}
		var sidPtr *int64
		if storyID > 0 {
			sid := storyID
			sidPtr = &sid
		}
		_ = s.recorder.Record(ctx, costsvc.RecordInput{
			EventID:     fmt.Sprintf("%s:memory_summary:llm_call:1", traceHex),
			UserID:      userID,
			ChildIDHash: childHash,
			Purpose:     "memory_summary",
			Provider:    out.Provider,
			Model:       out.Model,
			Usage:       pkgcost.Usage{TokensIn: out.InputTokens, TokensOut: out.OutputTokens},
			Outcome:     outcome,
			DurationMs:  int(out.Latency.Milliseconds()),
			StoryID:     sidPtr,
			TraceID:     traceHex,
		})
	}
	if err != nil {
		if s.biz != nil {
			s.biz.MemorySummaryTotal.WithLabelValues("fail").Inc()
			s.biz.LLMFailFallbackTotal.WithLabelValues("doubao", s.model, "upstream_error").Inc()
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
