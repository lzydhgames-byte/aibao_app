package story

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/service/safety"
	"github.com/aibao/server/internal/service/story/prompt"
)

// StoryRepo is the minimal repo surface Orchestrator needs.
type StoryRepo interface {
	CreateWithOutbox(ctx context.Context, story *model.Story, elements []*model.StoryElement, events []*model.OutboxEvent) error
	FindByID(ctx context.Context, id int64) (*model.Story, error)
}

// ChildRepo is the minimal repo surface Orchestrator needs.
type ChildRepo interface {
	FindByID(ctx context.Context, id int64) (*model.Child, error)
}

// Budget abstracts the LLM budget gate.
type Budget interface {
	PreCheck(ctx context.Context) error
	Record(ctx context.Context, inputTokens, outputTokens int) error
}

// MemorySelector returns a short prompt-injectable memory context.
// Plan 6: fail-open — returns "" on any error/no-recall.
type MemorySelector interface {
	BuildContext(ctx context.Context, childID int64) string
}

// Deps groups Orchestrator dependencies.
type Deps struct {
	Stories        StoryRepo
	Children       ChildRepo
	LLM            llm.Client
	Budget         Budget
	PreCheck       *safety.PreChecker
	PostCheck      *safety.PostChecker
	MemorySelector MemorySelector // Plan 6
	PromptTmpl     string
	FallbackDir    string
	StoryModel     string
	Temperature    float64
	PromptVersion  string
}

// Orchestrator runs the PreCheck → PromptBuild → LLM → PostCheck → Persist
// pipeline.
type Orchestrator struct {
	d        Deps
	builder  *prompt.Builder
	fallback *Fallback
}

// NewOrchestrator constructs an Orchestrator.
func NewOrchestrator(d Deps) (*Orchestrator, error) {
	b, err := prompt.NewBuilder(d.PromptTmpl)
	if err != nil {
		return nil, err
	}
	return &Orchestrator{
		d:        d,
		builder:  b,
		fallback: NewFallback(d.FallbackDir),
	}, nil
}

// GenerateParams is the structured input.
type GenerateParams struct {
	ChildID  int64
	UserID   int64
	Prompt   string
	Duration int
	Style    string
	Topic    string
}

// Generate is the main entry point.
func (o *Orchestrator) Generate(ctx context.Context, p GenerateParams) (*model.Story, error) {
	lg := logger.FromCtx(ctx)

	child, err := o.d.Children.FindByID(ctx, p.ChildID)
	if err != nil {
		return nil, apperr.New(apperr.CodeNotFound, "child_not_found", "未找到该孩子档案")
	}
	if child.UserID != p.UserID {
		return nil, apperr.New(apperr.CodePermissionDenied, "not_owner", "无权为该孩子生成故事")
	}

	if err := o.d.Budget.PreCheck(ctx); err != nil {
		return nil, apperr.New(apperr.CodeBudgetExceeded, "budget_exceeded", "今日额度已用完，请明天再来")
	}

	fearList := extractFearList(child.Profile)

	preOut := o.d.PreCheck.Check(ctx, safety.PreCheckInput{
		UserPrompt:    p.Prompt,
		ChildFearList: fearList,
	})
	if !preOut.Pass {
		return nil, mapSafetyReject(preOut.RejectReason, preOut.MatchedRule)
	}

	memCtx := ""
	if o.d.MemorySelector != nil {
		memCtx = o.d.MemorySelector.BuildContext(ctx, child.ID)
		lg.Debug("orchestrator.memory_context", "child_id", child.ID, "len", len(memCtx))
	}

	po := o.builder.Build(prompt.BuildInput{
		ChildNickname:            child.Nickname,
		ChildAgeYears:            ageYearsFromBirthday(child.Birthday),
		ChildGender:              child.Gender,
		ChildFearList:            fearList,
		Duration:                 p.Duration,
		Style:                    p.Style,
		Topic:                    p.Topic,
		UserPromptCleaned:        preOut.NormalizedPrompt,
		NormalizedIPs:            preOut.NormalizedIPs,
		NormalizedIPInstructions: preOut.IPInstructions,
		MemorySummary:            memCtx,
		PromptVersion:            o.d.PromptVersion,
	})

	var llmText string
	var llmInTok, llmOutTok int
	llmFailed := false
	for attempt := 0; attempt <= 1; attempt++ {
		resp, err := o.d.LLM.Generate(ctx, llm.GenerateRequest{
			Model:       o.d.StoryModel,
			Messages:    []llm.Message{{Role: "system", Content: po.SystemPrompt}, {Role: "user", Content: po.UserPrompt}},
			Temperature: o.d.Temperature,
		})
		if err == nil {
			llmText = resp.Text
			llmInTok = resp.InputTokens
			llmOutTok = resp.OutputTokens
			_ = o.d.Budget.Record(ctx, llmInTok, llmOutTok)
			break
		}
		lg.Warn("story.llm.attempt_failed", "attempt", attempt, "err", err.Error())
		if attempt == 1 {
			llmFailed = true
		}
	}

	usedFallback := false
	if !llmFailed {
		postOut := o.d.PostCheck.Check(safety.PostCheckInput{
			StoryText:     llmText,
			ChildNickname: child.Nickname,
			ChildFearList: fearList,
			Duration:      p.Duration,
		})
		if !postOut.Pass {
			lg.Warn("story.postcheck.fail", "reason", postOut.RejectReason, "rule", postOut.MatchedRule)
			llmFailed = true
		}
	}

	if llmFailed {
		fb, err := o.fallback.Load(FallbackKey{Style: p.Style, Duration: p.Duration}, child.Nickname)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "generation_failed", "服务暂时不可用，请稍后再试")
		}
		llmText = fb
		usedFallback = true
	}

	elemRaw := ExtractElements(llmText, preOut.NormalizedIPs)
	elements := make([]*model.StoryElement, 0, len(elemRaw))
	for _, e := range elemRaw {
		elements = append(elements, &model.StoryElement{
			ElementType:  e.ElementType,
			Name:         e.Name,
			Description:  e.Description,
			RecallWeight: e.RecallWeight,
		})
	}

	story := &model.Story{
		ChildID:         child.ID,
		Title:           extractTitle(llmText),
		TextContent:     llmText,
		DurationMinutes: p.Duration,
		Style:           p.Style,
		Topic:           p.Topic,
		PromptVersion:   o.d.PromptVersion,
		LLMInputTokens:  llmInTok,
		LLMOutputTokens: llmOutTok,
	}
	if usedFallback {
		story.LLMModel = "fallback"
	} else {
		story.LLMModel = o.d.StoryModel
	}

	memPayload, _ := json.Marshal(map[string]any{
		"story_id":      0,
		"child_id":      child.ID,
		"title":         story.Title,
		"summary":       summarize(llmText, 200),
		"used_fallback": usedFallback,
	})
	memEvent := &model.OutboxEvent{
		EventType: model.EventTypeMemoryUpdate,
		Payload:   memPayload,
		Status:    model.OutboxStatusPending,
	}

	ttsPayload, _ := json.Marshal(map[string]any{
		"story_id": 0,
		"child_id": child.ID,
	})
	ttsEvent := &model.OutboxEvent{
		EventType: model.EventTypeTTSSynthesis,
		Payload:   ttsPayload,
		Status:    model.OutboxStatusPending,
	}

	story.AudioStatus = model.AudioStatusPending

	if err := o.d.Stories.CreateWithOutbox(ctx, story, elements, []*model.OutboxEvent{memEvent, ttsEvent}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "story_persist_failed", "服务暂时不可用，请稍后再试")
	}

	patched, _ := json.Marshal(map[string]any{
		"story_id":      story.ID,
		"child_id":      child.ID,
		"title":         story.Title,
		"summary":       summarize(llmText, 200),
		"used_fallback": usedFallback,
	})
	memEvent.Payload = patched

	patchedTTS, _ := json.Marshal(map[string]any{
		"story_id": story.ID,
		"child_id": child.ID,
	})
	ttsEvent.Payload = patchedTTS

	lg.Info("story.generate.done",
		"story_id", story.ID,
		"child_id", child.ID,
		"used_fallback", usedFallback,
		"input_tokens", llmInTok,
		"output_tokens", llmOutTok,
	)
	return story, nil
}

func mapSafetyReject(reason, matched string) error {
	switch reason {
	case "redline_matched", "fear_matched":
		ae := apperr.New(apperr.CodeInvalidArgument, reason, "您的请求包含不适合儿童故事的内容")
		ae.Reason = reason
		_ = matched
		return ae
	case "ip_blacklisted":
		return apperr.New(apperr.CodeInvalidArgument, "ip_blacklisted", "该 IP 暂不支持，请换一个故事方向")
	case "too_long":
		return apperr.New(apperr.CodeInvalidArgument, "too_long", "请求太长，请简短一些")
	case "danger_chars":
		return apperr.New(apperr.CodeInvalidArgument, "danger_chars", "请求包含非法字符")
	case "intent_unsafe":
		return apperr.New(apperr.CodeInvalidArgument, "intent_unsafe", "请求被安全审核拒绝")
	default:
		return apperr.New(apperr.CodeInvalidArgument, "precheck_rejected", "请求被拒绝")
	}
}

func extractFearList(profile []byte) []string {
	if len(profile) == 0 {
		return nil
	}
	var p struct {
		Fears []string `json:"fears"`
	}
	if err := json.Unmarshal(profile, &p); err != nil {
		return nil
	}
	return p.Fears
}

func ageYearsFromBirthday(b time.Time) int {
	if b.IsZero() {
		return 0
	}
	now := time.Now()
	years := now.Year() - b.Year()
	if now.YearDay() < b.YearDay() {
		years--
	}
	if years < 0 {
		years = 0
	}
	return years
}

func extractTitle(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[BGM") || strings.HasPrefix(line, "[音效") {
			continue
		}
		runes := []rune(line)
		if len(runes) > 60 {
			runes = runes[:60]
		}
		return string(runes)
	}
	return ""
}

func summarize(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
