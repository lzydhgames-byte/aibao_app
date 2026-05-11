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
		Duration: 10, Style: "温馨治愈", Topic: "勇敢",
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
		Duration: 10, Style: "温馨治愈",
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
		Prompt: "讲个故事", Duration: 10, Style: "温馨治愈",
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
		ChildID: 7, UserID: 42, Prompt: "讲故事", Duration: 10, Style: "温馨治愈",
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
		Duration: 10, Style: "温馨治愈",
	})
	require.NoError(t, err)
	assert.True(t, sel.called)
	assert.Contains(t, cl.lastSystem, "上次救了小恐龙")
}

func TestOrchestrator_MemorySelector_EmptyStillGenerates(t *testing.T) {
	mock := llm.NewMock()
	mock.Response.Text = "小宇决定出发。爱宝跟着。小宇带头前进。"
	sel := &fakeMemorySelector{out: ""}
	orch := newOrchWithSelector(t, mock, sel)
	out, err := orch.Generate(context.Background(), GenerateParams{
		ChildID: 7, UserID: 42, Prompt: "讲个故事",
		Duration: 10, Style: "温馨治愈",
	})
	require.NoError(t, err)
	assert.True(t, sel.called)
	assert.NotZero(t, out.ID)
}
