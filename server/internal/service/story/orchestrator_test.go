package story

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/model"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/repository"
	"github.com/aibao/server/internal/service/safety"
)

type fakeStoryRepo struct {
	created *model.Story
	events  []*model.OutboxEvent
}

func (f *fakeStoryRepo) CreateWithOutbox(_ context.Context, s *model.Story, els []*model.StoryElement, evs []*model.OutboxEvent) error {
	s.ID = 100
	f.created = s
	for i, ev := range evs {
		ev.ID = int64(200 + i)
		f.events = append(f.events, ev)
	}
	for _, e := range els {
		e.StoryID = s.ID
	}
	return nil
}

func (f *fakeStoryRepo) FindByID(_ context.Context, id int64) (*model.Story, error) {
	if f.created != nil && f.created.ID == id {
		return f.created, nil
	}
	return nil, errors.New("not found")
}

type fakeChildRepo struct {
	c *model.Child
}

func (f *fakeChildRepo) FindByID(_ context.Context, id int64) (*model.Child, error) {
	if f.c != nil && f.c.ID == id {
		return f.c, nil
	}
	return nil, errors.New("not found")
}

type stubBudget struct {
	allow bool
}

func (s *stubBudget) PreCheck(_ context.Context) error {
	if !s.allow {
		return llm.ErrBudgetExceeded
	}
	return nil
}
func (s *stubBudget) Record(_ context.Context, _, _ int) error { return nil }

func mkChild() *model.Child {
	bday, _ := time.Parse("2006-01-02", "2020-08-15")
	return &model.Child{ID: 7, UserID: 42, Nickname: "小宇", Gender: "boy", Birthday: bday, Profile: []byte(`{"fears":["蜘蛛"]}`)}
}

func newOrch(t *testing.T, llmClient llm.Client) (*Orchestrator, *fakeStoryRepo) {
	t.Helper()
	rs := &safety.RuleSet{
		Redlines:        map[string][]string{"violence": {"血腥"}},
		AllRedlinesFlat: []string{"血腥"},
		IPWhitelist:     map[string]string{"奥特曼": "本故事中爱宝变身为爱宝奥特曼。"},
	}
	srepo := &fakeStoryRepo{}
	crepo := &fakeChildRepo{c: mkChild()}
	orch, err := NewOrchestrator(Deps{
		Stories:       srepo,
		Children:      crepo,
		LLM:           llmClient,
		Budget:        &stubBudget{allow: true},
		PreCheck:      safety.NewPreChecker(rs, safety.NewNoopIntentProvider()),
		PostCheck:     safety.NewPostChecker(rs),
		PromptTmpl:    "../../../safety/system_prompt.tmpl",
		FallbackDir:   "../../../safety/fallback_stories",
		StoryModel:    "doubao-1.5-pro-32k",
		Temperature:   0.8,
		PromptVersion: "v1",
	})
	require.NoError(t, err)
	return orch, srepo
}

func TestOrchestrator_HappyPath(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "小宇推开了门，决定走进竹林。爱宝跟着小宇。小宇说我们出发吧。小宇带着大家前进。"

	orch, repo := newOrch(t, mock)
	out, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "讲个奥特曼睡前故事",
		Duration: 5, Style: "温馨治愈", Topic: "勇敢",
	})
	require.NoError(t, err)
	assert.NotZero(t, out.ID)
	assert.Equal(t, "doubao-1.5-pro-32k", out.LLMModel)
	require.NotNil(t, repo.created)
	require.Len(t, repo.events, 2)
	types := []string{repo.events[0].EventType, repo.events[1].EventType}
	assert.Contains(t, types, model.EventTypeMemoryUpdate)
	assert.Contains(t, types, model.EventTypeTTSSynthesis)

	// memory_update payload should have story_id patched to story.ID
	var memEv *model.OutboxEvent
	var ttsEv *model.OutboxEvent
	for _, e := range repo.events {
		if e.EventType == model.EventTypeMemoryUpdate {
			memEv = e
		}
		if e.EventType == model.EventTypeTTSSynthesis {
			ttsEv = e
		}
	}
	require.NotNil(t, memEv)
	require.NotNil(t, ttsEv)

	var memPayload map[string]any
	require.NoError(t, json.Unmarshal(memEv.Payload, &memPayload))
	assert.Equal(t, float64(out.ID), memPayload["story_id"])

	var ttsPayload map[string]any
	require.NoError(t, json.Unmarshal(ttsEv.Payload, &ttsPayload))
	assert.Equal(t, float64(out.ID), ttsPayload["story_id"])

	// audio_status is set to pending on the response Story
	assert.Equal(t, model.AudioStatusPending, out.AudioStatus)
}

func TestOrchestrator_PreCheck_RejectsRedline(t *testing.T) {
	mock := llm.NewMock()
	orch, _ := newOrch(t, mock)
	_, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "我要血腥的故事",
		Duration: 5, Style: "温馨治愈",
	})
	require.Error(t, err)
	ae, ok := apperr.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperr.CodeInvalidArgument, ae.Code)
	assert.Equal(t, "redline_matched", ae.Reason)
	assert.Equal(t, 0, mock.Calls, "should NOT call LLM after PreCheck rejection")
}

func TestOrchestrator_ChildNotOwned(t *testing.T) {
	mock := llm.NewMock()
	orch, _ := newOrch(t, mock)
	_, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 999,
		Prompt: "讲个故事", Duration: 5, Style: "温馨治愈",
	})
	require.Error(t, err)
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodePermissionDenied, ae.Code)
}

func TestOrchestrator_BudgetExceeded(t *testing.T) {
	mock := llm.NewMock()
	rs := &safety.RuleSet{AllRedlinesFlat: []string{}}
	srepo := &fakeStoryRepo{}
	crepo := &fakeChildRepo{c: mkChild()}
	orch, err := NewOrchestrator(Deps{
		Stories: srepo, Children: crepo, LLM: mock,
		Budget:      &stubBudget{allow: false},
		PreCheck:    safety.NewPreChecker(rs, safety.NewNoopIntentProvider()),
		PostCheck:   safety.NewPostChecker(rs),
		PromptTmpl:  "../../../safety/system_prompt.tmpl",
		FallbackDir: "../../../safety/fallback_stories",
		StoryModel:  "x", Temperature: 0.8, PromptVersion: "v1",
	})
	require.NoError(t, err)

	_, err = orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "讲故事", Duration: 5, Style: "温馨治愈",
	})
	require.Error(t, err)
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodeBudgetExceeded, ae.Code)
}

func TestOrchestrator_LLMErrorFallsBackToTemplate(t *testing.T) {
	mock := llm.NewMock()
	mock.Err = errors.New("upstream timeout")
	orch, repo := newOrch(t, mock)

	out, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "讲个故事",
		Duration: 5, Style: "温馨治愈",
	})
	require.NoError(t, err)
	assert.Contains(t, out.TextContent, "小宇")
	require.NotNil(t, repo.created)
	assert.Equal(t, "fallback", repo.created.LLMModel)
}

func TestOrchestrator_PostCheckRejectionTriggersFallback(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "小宇看到血腥的怪兽。小宇害怕。小宇跑掉了。"
	orch, repo := newOrch(t, mock)

	out, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "讲个故事",
		Duration: 5, Style: "温馨治愈",
	})
	require.NoError(t, err)
	assert.NotContains(t, out.TextContent, "血腥")
	assert.Contains(t, out.TextContent, "小宇")
	_ = repo
}

type fakeMemorySelector struct {
	out    string
	called bool
}

func (f *fakeMemorySelector) BuildContext(_ context.Context, _ int64) string {
	f.called = true
	return f.out
}

func newOrchWithSelector(t *testing.T, mock llm.Client, sel MemorySelector) *Orchestrator {
	rs := &safety.RuleSet{
		Redlines:        map[string][]string{"violence": {"血腥"}},
		AllRedlinesFlat: []string{"血腥"},
		IPWhitelist:     map[string]string{},
	}
	orch, err := NewOrchestrator(Deps{
		Stories:        &fakeStoryRepo{},
		Children:       &fakeChildRepo{c: mkChild()},
		LLM:            mock,
		Budget:         &stubBudget{allow: true},
		PreCheck:       safety.NewPreChecker(rs, safety.NewNoopIntentProvider()),
		PostCheck:      safety.NewPostChecker(rs),
		MemorySelector: sel,
		PromptTmpl:     "../../../safety/system_prompt.tmpl",
		FallbackDir:    "../../../safety/fallback_stories",
		StoryModel:     "doubao-1.5-pro-32k",
		Temperature:    0.8,
		PromptVersion:  "v1",
	})
	require.NoError(t, err)
	return orch
}

type capturingLLM struct {
	lastSystem string
	text       string
}

func (c *capturingLLM) Generate(_ context.Context, r llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if len(r.Messages) > 0 {
		c.lastSystem = r.Messages[0].Content
	}
	return &llm.GenerateResponse{Text: c.text}, nil
}

func (c *capturingLLM) HealthCheck(_ context.Context) error { return nil }

func TestOrchestrator_MemorySelector_NonEmpty(t *testing.T) {
	cl := &capturingLLM{text: "小宇决定出发。爱宝跟着。小宇带头前进。"}
	sel := &fakeMemorySelector{out: "上次救了小恐龙"}
	orch := newOrchWithSelector(t, cl, sel)
	_, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "讲个故事",
		Duration: 5, Style: "温馨治愈",
	})
	require.NoError(t, err)
	assert.True(t, sel.called)
	assert.Contains(t, cl.lastSystem, "上次救了小恐龙")
}

// --- Plan 8 fakes ---

type fakeStorylineRepo struct {
	createCalls    int
	created        *model.Storyline
	incCalls       int
	lastIncID      int64
	lastIncHint    string
	createErr      error
	incErr         error
	nextCreateID   int64
}

func (f *fakeStorylineRepo) Create(_ context.Context, sl *model.Storyline) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	if f.nextCreateID == 0 {
		f.nextCreateID = 555
	}
	sl.ID = f.nextCreateID
	f.created = sl
	return nil
}

func (f *fakeStorylineRepo) IncrementEpisode(_ context.Context, id int64, hint string) error {
	f.incCalls++
	f.lastIncID = id
	f.lastIncHint = hint
	return f.incErr
}

type fakeCtxBuilder struct {
	out *StorylineContext
	err error
}

func (f *fakeCtxBuilder) Build(_ context.Context, _ int64) (*StorylineContext, error) {
	return f.out, f.err
}

type fakeChapterHook struct {
	out      string
	calls    int
}

func (f *fakeChapterHook) Extract(_ context.Context, _ string) string {
	f.calls++
	return f.out
}

func newOrchPlan8(t *testing.T, mock llm.Client, slRepo StorylineRepo, ctxBld StorylineContextBuilderAPI, hook ChapterHookAPI) (*Orchestrator, *fakeStoryRepo) {
	t.Helper()
	rs := &safety.RuleSet{AllRedlinesFlat: []string{"血腥"}, Redlines: map[string][]string{"violence": {"血腥"}}}
	srepo := &fakeStoryRepo{}
	crepo := &fakeChildRepo{c: mkChild()}
	orch, err := NewOrchestrator(Deps{
		Stories:         srepo,
		Children:        crepo,
		LLM:             mock,
		Budget:          &stubBudget{allow: true},
		PreCheck:        safety.NewPreChecker(rs, safety.NewNoopIntentProvider()),
		PostCheck:       safety.NewPostChecker(rs),
		Storylines:      slRepo,
		StorylineCtxBld: ctxBld,
		ChapterHook:     hook,
		PromptTmpl:      "../../../safety/system_prompt.tmpl",
		FallbackDir:     "../../../safety/fallback_stories",
		StoryModel:      "doubao-1.5-pro-32k",
		Temperature:     0.8,
		PromptVersion:   "v1",
	})
	require.NoError(t, err)
	return orch, srepo
}

func TestGenerate_StartStoryline_CreatesRowAndEpisode1(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "小宇推开门走进竹林。爱宝跟着小宇。小宇说我们出发。"
	slRepo := &fakeStorylineRepo{nextCreateID: 77}
	hook := &fakeChapterHook{out: "下集预告"}
	orch, _ := newOrchPlan8(t, mock, slRepo, nil, hook)
	out, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "讲个奥特曼睡前故事",
		Duration: 5, Style: "温馨治愈", StartStoryline: true,
	})
	require.NoError(t, err)
	require.NotNil(t, out.StorylineID)
	assert.Equal(t, int64(77), *out.StorylineID)
	require.NotNil(t, out.EpisodeNo)
	assert.Equal(t, 1, *out.EpisodeNo)
	assert.Equal(t, 1, slRepo.createCalls)
	assert.Equal(t, 1, slRepo.incCalls)
	assert.Equal(t, "下集预告", slRepo.lastIncHint)
}

func TestGenerate_ContinueStoryline_PassesEpisode2AndContext(t *testing.T) {
	cl := &capturingLLM{text: "小宇又遇到小恐龙。爱宝跟着小宇。小宇说我们继续。"}
	bld := &fakeCtxBuilder{out: &StorylineContext{
		StorylineID: 88, ChildID: 7, EpisodeNumber: 2,
		PreviousHook:    "钩子A", RecentSummaries: []string{"上集摘要"},
		PreviousElements: []string{"小恐龙"},
	}}
	slID := int64(88)
	slRepo := &fakeStorylineRepo{}
	hook := &fakeChapterHook{out: "下集"}
	orch, _ := newOrchPlan8(t, cl, slRepo, bld, hook)
	out, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "继续",
		Duration: 5, Style: "温馨治愈", StorylineID: &slID,
	})
	require.NoError(t, err)
	require.NotNil(t, out.StorylineID)
	assert.Equal(t, int64(88), *out.StorylineID)
	require.NotNil(t, out.EpisodeNo)
	assert.Equal(t, 2, *out.EpisodeNo)
	assert.Contains(t, cl.lastSystem, "钩子A")
	assert.Contains(t, cl.lastSystem, "上集摘要")
	assert.Equal(t, 1, slRepo.incCalls)
	assert.Equal(t, int64(88), slRepo.lastIncID)
}

func TestGenerate_StorylineNotOwnedByChild_Returns403(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "x"
	bld := &fakeCtxBuilder{out: &StorylineContext{StorylineID: 1, ChildID: 999, EpisodeNumber: 2}}
	slID := int64(1)
	orch, _ := newOrchPlan8(t, mock, &fakeStorylineRepo{}, bld, &fakeChapterHook{})
	_, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "x",
		Duration: 5, Style: "温馨治愈", StorylineID: &slID,
	})
	require.Error(t, err)
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodePermissionDenied, ae.Code)
}

func TestGenerate_StorylineNotFound_Returns404(t *testing.T) {
	mock := llm.NewMock()
	bld := &fakeCtxBuilder{err: repository.ErrNotFound}
	slID := int64(1)
	orch, _ := newOrchPlan8(t, mock, &fakeStorylineRepo{}, bld, &fakeChapterHook{})
	_, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "x",
		Duration: 5, Style: "温馨治愈", StorylineID: &slID,
	})
	require.Error(t, err)
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodeNotFound, ae.Code)
}

func TestGenerate_BothStartAndContinue_400(t *testing.T) {
	mock := llm.NewMock()
	slID := int64(1)
	orch, _ := newOrchPlan8(t, mock, &fakeStorylineRepo{}, &fakeCtxBuilder{}, &fakeChapterHook{})
	_, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "x",
		Duration: 5, Style: "温馨治愈",
		StartStoryline: true, StorylineID: &slID,
	})
	require.Error(t, err)
	ae, _ := apperr.AsAppError(err)
	require.NotNil(t, ae)
	assert.Equal(t, apperr.CodeInvalidArgument, ae.Code)
}

func TestGenerate_PostCheckNotContinuing_ShipsLLMTextAnyway(t *testing.T) {
	// Plan 9c: continuity miss is now a soft signal — we ship the LLM
	// output instead of falling back to a 150-char canned template, which
	// would otherwise produce ~45-second audio for any duration slot.
	mock := llm.NewMock()
	mock.Response.Text = "小宇看到一只小猫。爱宝跟着小宇。小宇说我们走。"
	bld := &fakeCtxBuilder{out: &StorylineContext{
		StorylineID: 99, ChildID: 7, EpisodeNumber: 3,
		PreviousElements: []string{"小恐龙", "竹林"},
	}}
	slID := int64(99)
	slRepo := &fakeStorylineRepo{}
	hook := &fakeChapterHook{out: "x"}
	orch, repo := newOrchPlan8(t, mock, slRepo, bld, hook)
	out, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "讲个故事",
		Duration: 5, Style: "温馨治愈", StorylineID: &slID,
	})
	require.NoError(t, err)
	// Storyline still progresses with the LLM text — not nulled out.
	assert.NotNil(t, out.StorylineID)
	assert.NotNil(t, out.EpisodeNo)
	assert.Equal(t, 1, slRepo.incCalls)
	assert.NotEqual(t, "fallback", repo.created.LLMModel)
}

func TestGenerate_ChapterHookFails_StoryStillShipsHintEmpty(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "小宇推开门走进竹林。爱宝跟着小宇。小宇说我们出发。"
	slRepo := &fakeStorylineRepo{nextCreateID: 33}
	hook := &fakeChapterHook{out: ""} // simulates fail-open
	orch, _ := newOrchPlan8(t, mock, slRepo, nil, hook)
	out, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "讲个故事",
		Duration: 5, Style: "温馨治愈", StartStoryline: true,
	})
	require.NoError(t, err)
	require.NotNil(t, out.StorylineID)
	assert.Equal(t, 1, slRepo.incCalls)
	assert.Equal(t, "", slRepo.lastIncHint)
}

func TestOrchestrator_MemorySelector_EmptyStillGenerates(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "小宇决定出发。爱宝跟着。小宇带头前进。"
	sel := &fakeMemorySelector{out: ""}
	orch := newOrchWithSelector(t, mock, sel)
	out, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "讲个故事",
		Duration: 5, Style: "温馨治愈",
	})
	require.NoError(t, err)
	assert.True(t, sel.called)
	assert.NotZero(t, out.ID)
}
