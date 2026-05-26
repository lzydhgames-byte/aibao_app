package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
)

type fakeMem struct {
	created []*model.Memory
}

func (f *fakeMem) Create(_ context.Context, m *model.Memory) error {
	f.created = append(f.created, m)
	return nil
}
func (f *fakeMem) RecentByChild(_ context.Context, _ int64, _ string, _ int) ([]*model.Memory, error) {
	return f.created, nil
}
func (f *fakeMem) RecentByChildTypes(_ context.Context, _ int64, _ []string, _ int) ([]*model.Memory, error) {
	return f.created, nil
}

type fakeStoryRepo struct {
	story *model.Story
	err   error
}

func (f *fakeStoryRepo) Create(_ context.Context, _ *model.Story) error { return nil }
func (f *fakeStoryRepo) FindByID(_ context.Context, _ int64) (*model.Story, error) {
	return f.story, f.err
}
func (f *fakeStoryRepo) UpdateAudio(_ context.Context, _ int64, _ string, _ int) error {
	return nil
}

type fakeSummarizer struct{ out string }

func (f *fakeSummarizer) Summarize(_ context.Context, _ string) string { return f.out }
func (f *fakeSummarizer) SummarizeForStory(_ context.Context, _ string, _ int64, _ int64, _ *int64, _ string) string {
	return f.out
}

func TestMemoryUpdate_HandleHappy(t *testing.T) {
	mem := &fakeMem{}
	h := NewMemoryUpdateHandler(mem, nil, nil)
	payload, _ := json.Marshal(map[string]any{
		"story_id": 100, "child_id": 7, "title": "T", "summary": "S", "used_fallback": false,
	})
	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{Payload: payload}))
	require.Len(t, mem.created, 1)
	assert.Equal(t, int64(7), mem.created[0].ChildID)
	assert.Equal(t, "story_summary", mem.created[0].MemoryType)
	require.NotNil(t, mem.created[0].StoryID)
	assert.Equal(t, int64(100), *mem.created[0].StoryID)
}

func TestMemoryUpdate_BadPayload(t *testing.T) {
	mem := &fakeMem{}
	h := NewMemoryUpdateHandler(mem, nil, nil)
	err := h.Handle(context.Background(), &model.OutboxEvent{Payload: []byte("not json")})
	assert.Error(t, err)
}

func TestMemoryUpdate_SummarizerSuccess_WritesSecondRow(t *testing.T) {
	mem := &fakeMem{}
	sr := &fakeStoryRepo{story: &model.Story{ID: 100, TextContent: "从前有个小朋友救了小恐龙。"}}
	sum := &fakeSummarizer{out: "小朋友救了小恐龙"}
	h := NewMemoryUpdateHandler(mem, sr, sum)
	payload, _ := json.Marshal(map[string]any{
		"story_id": 100, "child_id": 7, "title": "T", "summary": "long...", "used_fallback": false,
	})
	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{Payload: payload}))
	require.Len(t, mem.created, 2)
	var second map[string]any
	require.NoError(t, json.Unmarshal(mem.created[1].Payload, &second))
	assert.Equal(t, "小朋友救了小恐龙", second["summary"])
	require.NotNil(t, mem.created[1].StoryID)
	assert.Equal(t, int64(100), *mem.created[1].StoryID)
	assert.InDelta(t, 1.2, mem.created[1].Weight, 0.001)
}

func TestMemoryUpdate_SummarizerEmpty_OnlyOneRow(t *testing.T) {
	mem := &fakeMem{}
	sr := &fakeStoryRepo{story: &model.Story{ID: 100, TextContent: "..."}}
	sum := &fakeSummarizer{out: ""}
	h := NewMemoryUpdateHandler(mem, sr, sum)
	payload, _ := json.Marshal(map[string]any{
		"story_id": 100, "child_id": 7, "title": "T", "summary": "S", "used_fallback": false,
	})
	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{Payload: payload}))
	require.Len(t, mem.created, 1)
}

func TestMemoryUpdate_StoryFetchFails_FailOpen(t *testing.T) {
	mem := &fakeMem{}
	sr := &fakeStoryRepo{err: errors.New("not found")}
	sum := &fakeSummarizer{out: "x"}
	h := NewMemoryUpdateHandler(mem, sr, sum)
	payload, _ := json.Marshal(map[string]any{
		"story_id": 100, "child_id": 7, "title": "T", "summary": "S", "used_fallback": false,
	})
	require.NoError(t, h.Handle(context.Background(), &model.OutboxEvent{Payload: payload}))
	require.Len(t, mem.created, 1)
}
