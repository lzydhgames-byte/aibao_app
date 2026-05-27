package outline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	pkgcost "github.com/aibao/server/internal/pkg/cost"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/pkg/idhash"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/pkg/traceid"
	"github.com/aibao/server/internal/service/cost"
	"github.com/aibao/server/internal/service/outlinecontract"
	"github.com/aibao/server/internal/service/safety"
	"github.com/aibao/server/internal/service/story/prompt"
)

// Service is the outline preview orchestrator (Plan 11A §7.1).
// Pipeline:
//
//	PreCheck input → LLM call (1 schema-repair retry)
//	→ OutlineSafetyCheck (1 safety-repair retry) → inject scene_seed
//	→ write Redis cache → append outline_events pending → return.
type Service struct {
	llm      llm.Client
	llmModel string
	matcher  safety.Matcher
	pre      *safety.PreChecker
	cache    *Cache
	events   *EventStore
	recorder *cost.Recorder
	idHasher *idhash.Hasher
	biz      *metrics.Business // may be nil for unit tests
}

// Deps bundles Service dependencies for explicit injection.
type Deps struct {
	LLM      llm.Client
	LLMModel string
	Matcher  safety.Matcher
	PreCheck *safety.PreChecker
	Cache    *Cache
	Events   *EventStore
	Recorder *cost.Recorder
	IDHasher *idhash.Hasher
	Biz      *metrics.Business
}

func NewService(d Deps) *Service {
	return &Service{
		llm:      d.LLM,
		llmModel: d.LLMModel,
		matcher:  d.Matcher,
		pre:      d.PreCheck,
		cache:    d.Cache,
		events:   d.Events,
		recorder: d.Recorder,
		idHasher: d.IDHasher,
		biz:      d.Biz,
	}
}

// PreviewInput is what /outlines/preview handler (Task 19) constructs.
type PreviewInput struct {
	UserID        int64
	ChildID       int64
	ChildNickname string
	ChildAge      int
	ChildFears    []string
	IPBlacklist   []string
	IPWhitelist   []string
	Prompt        string
	DurationMin   int

	// ParentOutlineID, when non-empty, denotes a refresh: a new outline_id is
	// minted but OutlineGroupID is inherited from the parent and VariantIndex
	// is incremented.
	ParentOutlineID string
}

// PreviewResult is what /outlines/preview handler returns to client.
type PreviewResult struct {
	OutlineID string
	Outline   outlinecontract.Outline
	ExpiresAt time.Time
}

// Preview runs the full pipeline. Returns *apperr.AppError on user-facing
// failures (PreCheck reject / safety reject / LLM fail); handler converts
// to HTTP code via apperr.AppError.HTTPStatus().
func (s *Service) Preview(ctx context.Context, in PreviewInput) (*PreviewResult, error) {
	start := time.Now()
	traceID := traceIDFromCtx(ctx)

	// 1. Input PreCheck
	pre := s.pre.Check(ctx, safety.PreCheckInput{
		UserPrompt:    in.Prompt,
		ChildFearList: in.ChildFears,
	})
	if !pre.Pass {
		return nil, apperr.New(
			apperr.CodeInvalidArgument,
			"safety_rejected:"+pre.MatchedCategory,
			pre.RejectReason,
		)
	}

	// 2. LLM call with 1 schema-repair retry
	sys, usr := BuildPrompt(OutlinePromptInput{
		ChildNickname: in.ChildNickname,
		ChildAge:      in.ChildAge,
		UserPrompt:    in.Prompt,
		DurationMin:   in.DurationMin,
	})

	var ro *RawOutline
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		messages := []llm.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: usr},
		}
		if attempt == 2 && lastErr != nil {
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: fmt.Sprintf("上次返回不合规：%v。请重新返回严格符合 schema 的 JSON。", lastErr),
			})
		}
		resp, err := s.llm.Generate(ctx, llm.GenerateRequest{
			Model:       s.llmModel,
			Messages:    messages,
			Temperature: 0.8,
		})
		if err != nil {
			lastErr = err
			continue
		}
		s.recordCost(ctx, in, traceID, "outline", "llm_call", attempt, resp, "ok")
		ro, lastErr = Parse(resp.Text)
		if lastErr == nil {
			break
		}
	}
	if ro == nil {
		logger.FromCtx(ctx).Warn("outline.preview.llm_failed", "err", lastErr)
		return nil, apperr.New(apperr.CodeInternal, "llm_failed", "大纲生成失败，请稍后再试")
	}

	// 3. OutlineSafetyCheck with 1 safety-repair retry
	safetyIn := SafetyCheckInput{
		Outline:       *ro,
		ChildNickname: in.ChildNickname,
		ChildFears:    in.ChildFears,
		IPBlacklist:   in.IPBlacklist,
		IPWhitelist:   in.IPWhitelist,
	}
	res := Check(ctx, s.matcher, safetyIn)
	if !res.OK {
		s.bumpSafetyRepair(res.Category, "retry")
		hint := fmt.Sprintf("上一版本命中安全规则（%s），请重新生成，避免该类内容。", res.Category)
		messages := []llm.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: usr},
			{Role: "user", Content: hint},
		}
		resp, err := s.llm.Generate(ctx, llm.GenerateRequest{
			Model:       s.llmModel,
			Messages:    messages,
			Temperature: 0.6,
		})
		if err != nil {
			s.bumpSafetyRepair(res.Category, "give_up")
			return nil, apperr.New(
				apperr.CodeInvalidArgument,
				"safety_rejected:"+res.Category,
				res.Reason.Error(),
			)
		}
		s.recordCost(ctx, in, traceID, "outline", "safety_repair", 1, resp, "ok")
		ro2, perr := Parse(resp.Text)
		if perr != nil {
			s.bumpSafetyRepair(res.Category, "give_up")
			return nil, apperr.New(
				apperr.CodeInvalidArgument,
				"safety_rejected:"+res.Category,
				res.Reason.Error(),
			)
		}
		safetyIn.Outline = *ro2
		res2 := Check(ctx, s.matcher, safetyIn)
		if !res2.OK {
			s.bumpSafetyRepair(res.Category, "give_up")
			return nil, apperr.New(
				apperr.CodeInvalidArgument,
				"safety_rejected:"+res2.Category,
				res2.Reason.Error(),
			)
		}
		ro = ro2
		s.bumpSafetyRepair(res.Category, "success")
	}

	// 4. Inject scene_seed + group/variant metadata
	outlineID := newOutlineID()
	groupID := outlineID
	variantIdx := 0
	if in.ParentOutlineID != "" {
		if parent, err := s.cache.Get(ctx, in.ParentOutlineID); err == nil {
			groupID = parent.OutlineGroupID
			variantIdx = parent.VariantIndex + 1
		}
	}
	sceneSeed := prompt.PickSceneSeed()

	resolved := outlinecontract.Outline{
		OutlineID:            outlineID,
		Title:                ro.Title,
		Synopsis:             ro.Synopsis,
		Themes:               ro.Themes,
		Style:                ro.Style,
		EducationalValue:     ro.EducationalValue,
		DurationMin:          in.DurationMin,
		SceneSeed:            sceneSeed,
		OutlineGroupID:       groupID,
		VariantIndex:         variantIdx,
		ParentOutlineID:      in.ParentOutlineID,
		OutlinePromptVersion: OutlinePromptVersion,
	}

	// 5. Persist: Redis + outline_events pending
	co := NewCachedOutline(resolved, in.UserID, in.ChildID, in.Prompt)
	if err := s.cache.Set(ctx, co); err != nil {
		return nil, fmt.Errorf("cache set: %w", err)
	}
	if err := s.events.Append(ctx, model.OutlineEvent{
		OutlineID:            outlineID,
		OutlineGroupID:       groupID,
		UserID:               in.UserID,
		ChildIDHash:          s.idHasher.Hash("child", in.ChildID),
		Outcome:              OutcomePending,
		OutlinePromptVersion: OutlinePromptVersion,
		DurationMin:          in.DurationMin,
		TraceID:              traceID,
	}); err != nil {
		_ = s.cache.Invalidate(ctx, outlineID)
		return nil, fmt.Errorf("events append: %w", err)
	}

	if s.biz != nil {
		s.biz.OutlineOutcomeTotal.WithLabelValues(OutcomePending).Inc()
		s.biz.OutlinePreviewDurationSeconds.Observe(time.Since(start).Seconds())
	}

	return &PreviewResult{
		OutlineID: outlineID,
		Outline:   resolved,
		ExpiresAt: time.Now().Add(cacheTTL),
	}, nil
}

func (s *Service) recordCost(ctx context.Context, in PreviewInput, traceID, purpose, stage string, attempt int, resp *llm.GenerateResponse, outcome string) {
	if resp == nil || s.recorder == nil {
		return
	}
	userID := in.UserID
	_ = s.recorder.Record(ctx, cost.RecordInput{
		EventID:              fmt.Sprintf("%s:%s:%s:%d", traceID, purpose, stage, attempt),
		UserID:               &userID,
		ChildIDHash:          s.idHasher.Hash("child", in.ChildID),
		Purpose:              purpose,
		Provider:             resp.Provider,
		Model:                resp.Model,
		Usage:                pkgcost.Usage{TokensIn: resp.InputTokens, TokensOut: resp.OutputTokens},
		Outcome:              outcome,
		DurationMs:           int(resp.Latency.Milliseconds()),
		OutlinePromptVersion: OutlinePromptVersion,
		TraceID:              traceID,
	})
}

func (s *Service) bumpSafetyRepair(category, result string) {
	if s.biz == nil {
		return
	}
	s.biz.OutlineSafetyRepairTotal.WithLabelValues(category, result).Inc()
}

// newOutlineID returns a crypto-random "ol_<32hex>" identifier (spec §5.2).
func newOutlineID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "ol_" + hex.EncodeToString([]byte(time.Now().Format("20060102150405.000")))
	}
	return "ol_" + hex.EncodeToString(b)
}

// traceIDFromCtx returns the trace_id hex (without "tr-" prefix) for use in
// event_id construction (spec §5.1.1 Recorder regex demands [a-f0-9]{8,}).
// Falls back to "00000000" if context has no trace id (e.g. unit tests).
func traceIDFromCtx(ctx context.Context) string {
	id, ok := traceid.FromContext(ctx)
	if !ok || id == "" {
		return "00000000"
	}
	return strings.TrimPrefix(id, "tr-")
}
