package story

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/aibao/server/internal/repository"
	costsvc "github.com/aibao/server/internal/service/cost"
	"github.com/aibao/server/internal/service/outlinecontract"
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

// StorylineRepo is the minimal storyline-repo surface the orchestrator needs.
// (Plan 7 §5.17 ISP — consumer declares its narrow view.)
type StorylineRepo interface {
	Create(ctx context.Context, sl *model.Storyline) error
	IncrementEpisode(ctx context.Context, id int64, hint string) error
}

// StorylineContextBuilderAPI is the slice of StorylineContextBuilder behavior
// the orchestrator depends on. Allows test doubles.
type StorylineContextBuilderAPI interface {
	Build(ctx context.Context, storylineID int64) (*StorylineContext, error)
}

// ChapterHookAPI is the slice of ChapterHookExtractor used by the orchestrator.
type ChapterHookAPI interface {
	Extract(ctx context.Context, storyText string) string
	ExtractForStory(ctx context.Context, storyText string, childID, storyID int64, userID *int64, traceHex string) string
}

// OutlineEventAppender is the narrow append-only surface the orchestrator
// needs from service/outline.EventStore. Defined here so story does not
// reverse-import the outline package (spec §7.5 N5).
type OutlineEventAppender interface {
	Append(ctx context.Context, evt model.OutlineEvent) error
}

// Deps groups Orchestrator dependencies.
type Deps struct {
	Stories         StoryRepo
	Children        ChildRepo
	LLM             llm.Client
	Budget          Budget
	PreCheck        *safety.PreChecker
	PostCheck       *safety.PostChecker
	MemorySelector  MemorySelector // Plan 6
	Storylines      StorylineRepo  // Plan 8 (optional)
	StorylineCtxBld StorylineContextBuilderAPI // Plan 8 (optional)
	ChapterHook     ChapterHookAPI // Plan 8 (optional)
	Biz             *metrics.Business // Plan 8 metrics (nil-safe)
	Recorder        *costsvc.Recorder // Plan 11B cost recording (nil-safe)
	IDHasher        *idhash.Hasher    // Plan 11B child_id_hash HMAC (nil-safe)
	PromptTmpl      string
	FallbackDir     string
	StoryModel      string
	Temperature     float64
	PromptVersion   string

	// Plan 11A — outline preview injection (spec §7.3 Step 0 / §7.5 N5).
	// OutlineResolver is the contract implemented by service/outline.ResolverImpl;
	// story depends on the contract only to avoid bidirectional packages.
	OutlineResolver outlinecontract.OutlineResolver
	// OutlineEvents writes the outcome=accepted lifecycle event when an
	// outline_id is consumed by /stories/generate.
	OutlineEvents OutlineEventAppender
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
	ChildID        int64
	UserID         int64
	Prompt         string
	Duration       int
	Style          string
	Topic          string
	StartStoryline bool   // Plan 8: create a new storyline; this story = episode 1
	StorylineID    *int64 // Plan 8: continue an existing storyline

	// Plan 11A — outline preview path (spec §7.3 Step 0).
	OutlineID        string
	OutlineOverrides *OutlineOverrides
}

// OutlineOverrides is the whitelist of fields the client may override on
// an outline before story generation (spec §6.3). Empty fields = no override.
type OutlineOverrides struct {
	Style            string
	Themes           []string
	EducationalValue string
}

// Generate is the main entry point.
func (o *Orchestrator) Generate(ctx context.Context, p GenerateParams) (*model.Story, error) {
	lg := logger.FromCtx(ctx)

	// Plan 8: defense-in-depth mutual-exclusion check.
	if p.StartStoryline && p.StorylineID != nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid_argument", "不能同时启动新连续剧和续接已有连续剧")
	}

	// Plan 11A §6.6 / §10.1: outline_id is mutually exclusive with storyline modes.
	if p.OutlineID != "" && (p.StartStoryline || p.StorylineID != nil) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "conflicting_modes", "outline_id 与 storyline 模式互斥")
	}

	// Plan 11A Step 0: HydrateFromOutline. Resolve outline_id (ownership +
	// replay defense), apply whitelist overrides, mark accepted, then hydrate
	// p with outline-derived style/topic/duration before the rest of the
	// pipeline runs. Spec §6.6: when outline_id is present, raw style/topic/
	// duration from the request body are ignored.
	if p.OutlineID != "" {
		if o.d.OutlineResolver == nil {
			return nil, apperr.New(apperr.CodeInternal, "outline_resolver_missing", "服务器开小差了")
		}
		ol, rerr := o.d.OutlineResolver.Resolve(ctx, p.OutlineID, p.UserID, p.ChildID)
		if rerr != nil {
			switch {
			case errors.Is(rerr, outlinecontract.ErrOutlineExpired):
				return nil, apperr.New(apperr.CodeOutlineExpired, "outline_expired", "大纲已过期，请重新预览")
			case errors.Is(rerr, outlinecontract.ErrOutlineForbidden):
				return nil, apperr.New(apperr.CodePermissionDenied, "outline_not_yours", "大纲不属于你")
			default:
				return nil, apperr.Wrap(rerr, apperr.CodeInternal, "outline_resolve", "服务器开小差了")
			}
		}
		// Apply whitelist overrides (spec §6.3). Server is source of truth
		// for what is overridable — only these three fields.
		if p.OutlineOverrides != nil {
			if p.OutlineOverrides.Style != "" {
				ol.Style = p.OutlineOverrides.Style
			}
			if len(p.OutlineOverrides.Themes) > 0 {
				ol.Themes = p.OutlineOverrides.Themes
			}
			if p.OutlineOverrides.EducationalValue != "" {
				ol.EducationalValue = p.OutlineOverrides.EducationalValue
			}
		}
		// Hydrate p with outline-derived values (spec §6.6).
		p.Style = ol.Style
		if len(ol.Themes) > 0 {
			p.Topic = ol.Themes[0]
		}
		p.Duration = ol.DurationMin

		// Append accepted lifecycle event before LLM call. Failure is
		// non-fatal — Aggregator can recompute from cost_events if missing.
		if o.d.OutlineEvents != nil {
			// "accepted" literal (not outline.OutcomeAccepted) so story does
			// not reverse-import the outline package — spec §7.5 N5.
			if appendErr := o.d.OutlineEvents.Append(ctx, model.OutlineEvent{
				OutlineID:            p.OutlineID,
				OutlineGroupID:       ol.OutlineGroupID,
				UserID:               p.UserID,
				Outcome:              "accepted",
				OutlinePromptVersion: ol.OutlinePromptVersion,
				DurationMin:          ol.DurationMin,
			}); appendErr != nil {
				lg.Warn("story.outline.append_accepted_failed", "err", appendErr, "outline_id", p.OutlineID)
			}
		}
		_ = ol
	}

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
	for _, w := range preOut.SoftWarnings {
		lg.Warn("story.precheck.soft_warn", "reason", w.Reason, "rule", w.Rule, "category", w.Category)
	}

	// Plan 8: preprocess storyline state.
	var (
		storylineID   *int64
		episodeNumber int
		storylineCtx  *StorylineContext
	)
	switch {
	case p.StartStoryline:
		if o.d.Storylines == nil {
			return nil, apperr.New(apperr.CodeInternal, "storyline_unavailable", "服务暂时不可用，请稍后再试")
		}
		sl := &model.Storyline{ChildID: child.ID, Status: model.StorylineStatusActive}
		if err := o.d.Storylines.Create(ctx, sl); err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "storyline_create_failed", "服务暂时不可用，请稍后再试")
		}
		if o.d.Biz != nil {
			o.d.Biz.StorylineCreatedTotal.Inc()
		}
		storylineID = &sl.ID
		episodeNumber = 1
	case p.StorylineID != nil:
		if o.d.StorylineCtxBld == nil {
			return nil, apperr.New(apperr.CodeInternal, "storyline_unavailable", "服务暂时不可用，请稍后再试")
		}
		slCtx, err := o.d.StorylineCtxBld.Build(ctx, *p.StorylineID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, apperr.New(apperr.CodeNotFound, "storyline_not_found", "找不到该连续剧")
			}
			return nil, apperr.Wrap(err, apperr.CodeInternal, "storyline_load_failed", "服务暂时不可用，请稍后再试")
		}
		if slCtx.ChildID != child.ID {
			return nil, apperr.New(apperr.CodePermissionDenied, "not_owner", "无权访问该连续剧")
		}
		storylineID = p.StorylineID
		episodeNumber = slCtx.EpisodeNumber
		storylineCtx = slCtx
	}

	memCtx := ""
	if o.d.MemorySelector != nil {
		memCtx = o.d.MemorySelector.BuildContext(ctx, child.ID)
		lg.Debug("orchestrator.memory_context", "child_id", child.ID, "len", len(memCtx))
	}

	buildIn := prompt.BuildInput{
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
	}
	if storylineCtx != nil {
		buildIn.StorylineHook = storylineCtx.PreviousHook
		buildIn.StorylineRecentSummaries = storylineCtx.RecentSummaries
		buildIn.EpisodeNumber = storylineCtx.EpisodeNumber
	}
	po := o.builder.Build(buildIn)
	lg.Info("prompt.scene_seed", "seed", po.SceneSeed)

	var llmText string
	var llmInTok, llmOutTok int
	llmFailed := false
	trHex := traceHex(ctx)
	uid := p.UserID
	userIDPtr := &uid
	childHash := ""
	if o.d.IDHasher != nil {
		childHash = o.d.IDHasher.Hash("child", child.ID)
	}
	for attempt := 0; attempt <= 1; attempt++ {
		resp, err := o.d.LLM.Generate(ctx, llm.GenerateRequest{
			Model:       o.d.StoryModel,
			Messages:    []llm.Message{{Role: "system", Content: po.SystemPrompt}, {Role: "user", Content: po.UserPrompt}},
			Temperature: o.d.Temperature,
		})
		outcome := "ok"
		if err != nil {
			outcome = "fail"
		}
		if o.d.Recorder != nil && resp != nil {
			_ = o.d.Recorder.Record(ctx, costsvc.RecordInput{
				EventID:     fmt.Sprintf("%s:story:llm_call:%d", trHex, attempt+1),
				UserID:      userIDPtr,
				ChildIDHash: childHash,
				Purpose:     "story",
				Provider:    resp.Provider,
				Model:       resp.Model,
				Usage:       pkgcost.Usage{TokensIn: resp.InputTokens, TokensOut: resp.OutputTokens},
				Outcome:     outcome,
				DurationMs:  int(resp.Latency.Milliseconds()),
				TraceID:     trHex,
			})
		}
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

	// Length guard: Doubao routinely under-writes (observed 600-1500 chars
	// for a 2300-char 8min target). Up to 2 rewrites with an explicit
	// '上次只写了 X 字，太短' steer. Each rewrite costs one extra LLM call
	// (~¥0.02). Capped at 2 so worst-case latency stays under ~3 LLM
	// roundtrips. We always swap to the longest run observed across
	// attempts — never regress.
	if !llmFailed {
		rmin, _ := prompt.ExpectedRuneBand(p.Duration)
		// 85% threshold (was 70%): observed 1500-char outputs slipping
		// past the 70% gate but producing only 4:40 audio for an 8min
		// slot. Tighter trigger costs at most one more LLM call.
		threshold := rmin * 85 / 100
		got := prompt.CountCJKRunes(llmText)
		for retryNo := 1; retryNo <= 2 && got < threshold; retryNo++ {
			lg.Warn("story.length.too_short", "attempt", retryNo, "got", got, "expected_min", rmin, "threshold", threshold)
			retryUser := fmt.Sprintf(
				"%s\n\n【重要】上次你只写了大约 %d 个汉字，远低于要求的 %d–%d 字硬约束。请重新创作一个完整的故事，必须超过 %d 个汉字。通过增加细节描写、对话、孩子的内心活动、场景刻画、感官细节（看到/听到/闻到/触到的东西）来扩展，不要省略情节，不要急着收尾。",
				po.UserPrompt, got, rmin, rmin*11/9, rmin,
			)
			resp, err := o.d.LLM.Generate(ctx, llm.GenerateRequest{
				Model:       o.d.StoryModel,
				Messages:    []llm.Message{{Role: "system", Content: po.SystemPrompt}, {Role: "user", Content: retryUser}},
				Temperature: o.d.Temperature,
			})
			if o.d.Recorder != nil && resp != nil {
				outcome := "ok"
				if err != nil {
					outcome = "fail"
				}
				_ = o.d.Recorder.Record(ctx, costsvc.RecordInput{
					EventID:     fmt.Sprintf("%s:story:length_retry:%d", trHex, retryNo),
					UserID:      userIDPtr,
					ChildIDHash: childHash,
					Purpose:     "story",
					Provider:    resp.Provider,
					Model:       resp.Model,
					Usage:       pkgcost.Usage{TokensIn: resp.InputTokens, TokensOut: resp.OutputTokens},
					Outcome:     outcome,
					DurationMs:  int(resp.Latency.Milliseconds()),
					TraceID:     trHex,
				})
			}
			if err != nil {
				lg.Warn("story.length.retry_failed", "attempt", retryNo, "err", err.Error())
				break
			}
			newGot := prompt.CountCJKRunes(resp.Text)
			lg.Info("story.length.retry_done", "attempt", retryNo, "old", got, "new", newGot, "expected_min", rmin)
			_ = o.d.Budget.Record(ctx, resp.InputTokens, resp.OutputTokens)
			llmInTok += resp.InputTokens
			llmOutTok += resp.OutputTokens
			// Only swap if the rewrite is strictly longer — defensive
			// against a worse roll.
			if newGot > got {
				llmText = resp.Text
				got = newGot
			}
		}
	}

	usedFallback := false
	if !llmFailed {
		postIn := safety.PostCheckInput{
			StoryText:     llmText,
			ChildNickname: child.Nickname,
			ChildFearList: fearList,
			Duration:      p.Duration,
		}
		if storylineCtx != nil {
			postIn.RequireContinuity = true
			postIn.PreviousElements = pickElements(storylineCtx)
		}
		postOut := o.d.PostCheck.Check(postIn)
		if !postOut.Pass {
			lg.Warn("story.postcheck.fail",
				"reason", postOut.RejectReason,
				"rule", postOut.MatchedRule,
				"category", postOut.MatchedCategory)
			// Soft fails (warn-only, ship the LLM text anyway):
			//   1. continuity miss — fallback (50→1min) 比"不连贯"更糟
			//   2. horror-category redline — 多为悬念铺垫，命中也轻
			//   3. negative_values redline — LLM 在教育性故事里把这类词
			//      用在批判语境（"不要自私"/"嘲笑别人是不好的"），
			//      字符串匹配看不出语境，hard-fail 把"学会分享"等
			//      正面教育主题打成 fallback 反 UX。
			// Hard fails (still drop to fallback):
			//   - violence / sexual / political_religious / dangerous_imitation
			//   - fear_matched (孩子明确害怕的词)
			//   - child_not_protagonist (主角错位)
			cat := postOut.MatchedCategory
			soft := postOut.RejectReason == model.PostCheckReasonNotContinuing ||
				(postOut.RejectReason == "redline_matched" &&
					(cat == "horror" || cat == "negative_values"))
			if !soft {
				llmFailed = true
			}
		}
	}

	if llmFailed {
		fb, err := o.fallback.Load(FallbackKey{Style: p.Style, Duration: p.Duration}, child.Nickname)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "generation_failed", "服务暂时不可用，请稍后再试")
		}
		llmText = fb
		usedFallback = true
		// Plan 8: fallback story must NOT be attached to the storyline (would
		// pollute its continuity / wrongly bump episode_count).
		storylineID = nil
		episodeNumber = 0
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
		StorylineID:     storylineID,
		PromptVersion:   o.d.PromptVersion,
		LLMInputTokens:  llmInTok,
		LLMOutputTokens: llmOutTok,
	}
	if episodeNumber > 0 {
		eno := episodeNumber
		story.EpisodeNo = &eno
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

	// Plan 8: post-write storyline bookkeeping (skip on fallback or non-series).
	if !usedFallback && storylineID != nil && o.d.Storylines != nil {
		hint := ""
		if o.d.ChapterHook != nil {
			hint = o.d.ChapterHook.ExtractForStory(ctx, llmText, child.ID, story.ID, userIDPtr, trHex)
		}
		if err := o.d.Storylines.IncrementEpisode(ctx, *storylineID, hint); err != nil {
			lg.Warn("story.storyline.increment_failed", "err", err.Error())
		}
		if o.d.Biz != nil {
			o.d.Biz.StorylineEpisodesTotal.Inc()
		}
	}

	lg.Info("story.generate.done",
		"story_id", story.ID,
		"child_id", child.ID,
		"used_fallback", usedFallback,
		"input_tokens", llmInTok,
		"output_tokens", llmOutTok,
	)
	return story, nil
}

// pickElements returns previous-episode character/place names for the
// PostCheck not_continuing rule, or nil when no context is available.
func pickElements(c *StorylineContext) []string {
	if c == nil || len(c.PreviousElements) == 0 {
		return nil
	}
	return c.PreviousElements
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

// traceHex returns the 8+hex portion of a project trace id ("tr-abcd1234" → "abcd1234")
// suitable for the Recorder event_id regex (^[a-f0-9]{8,}:...). Falls back to a
// stable zero-prefix when missing — recording continues with a degraded id.
func traceHex(ctx context.Context) string {
	id, ok := traceid.FromContext(ctx)
	if !ok || id == "" {
		return "00000000"
	}
	if strings.HasPrefix(id, "tr-") {
		id = id[3:]
	}
	if len(id) < 8 {
		id = id + strings.Repeat("0", 8-len(id))
	}
	return id
}

func summarize(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
