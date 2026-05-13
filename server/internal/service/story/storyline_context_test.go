package story

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
)

type fakeSLRepo struct {
	sl  *model.Storyline
	err error
}

func (f *fakeSLRepo) FindByID(_ context.Context, _ int64) (*model.Storyline, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sl, nil
}

type fakeStoryCtxRepo struct {
	stories     []*model.Story
	elements    []*model.StoryElement
	recentErr   error
	elementsErr error
}

func (f *fakeStoryCtxRepo) RecentByStoryline(_ context.Context, _ int64, _ int) ([]*model.Story, error) {
	return f.stories, f.recentErr
}

func (f *fakeStoryCtxRepo) ElementsByStory(_ context.Context, _ int64, _ []string, _ int) ([]*model.StoryElement, error) {
	return f.elements, f.elementsErr
}

type fakeMemCtxRepo struct {
	mems []*model.Memory
	err  error
}

func (f *fakeMemCtxRepo) RecentByChildTypes(_ context.Context, _ int64, _ []string, _ int) ([]*model.Memory, error) {
	return f.mems, f.err
}

func TestBuild_Success_WithAll4Fields(t *testing.T) {
	id1, id2 := int64(101), int64(102)
	sl := &model.Storyline{ID: 7, ChildID: 42, Title: "勇敢之旅", EpisodeCount: 2, NextEpisodeHint: "他们能找到宝藏吗"}
	stories := []*model.Story{{ID: id2, ChildID: 42}, {ID: id1, ChildID: 42}}
	mems := []*model.Memory{
		{ChildID: 42, MemoryType: model.MemoryTypeStorySummary, StoryID: &id2, Payload: []byte(`{"summary":"第二集摘要"}`)},
		{ChildID: 42, MemoryType: model.MemoryTypeStorySummary, StoryID: &id1, Payload: []byte(`{"summary":"第一集摘要"}`)},
	}
	elems := []*model.StoryElement{
		{Name: "小宇", ElementType: "character", RecallWeight: 2.0},
		{Name: "竹林", ElementType: "place", RecallWeight: 1.0},
	}
	b := NewStorylineContextBuilder(
		&fakeSLRepo{sl: sl},
		&fakeStoryCtxRepo{stories: stories, elements: elems},
		&fakeMemCtxRepo{mems: mems},
		nil,
	)
	out, err := b.Build(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, int64(7), out.StorylineID)
	assert.Equal(t, "勇敢之旅", out.Title)
	assert.Equal(t, 3, out.EpisodeNumber)
	assert.Equal(t, "他们能找到宝藏吗", out.PreviousHook)
	assert.Equal(t, []string{"第二集摘要", "第一集摘要"}, out.RecentSummaries)
	assert.Equal(t, []string{"小宇", "竹林"}, out.PreviousElements)
}

func TestBuild_NoMemoriesForStory_SummariesEmptyButNoError(t *testing.T) {
	sl := &model.Storyline{ID: 7, ChildID: 42, Title: "T", EpisodeCount: 1}
	stories := []*model.Story{{ID: 999, ChildID: 42}}
	b := NewStorylineContextBuilder(
		&fakeSLRepo{sl: sl},
		&fakeStoryCtxRepo{stories: stories, elements: nil},
		&fakeMemCtxRepo{mems: nil},
		nil,
	)
	out, err := b.Build(context.Background(), 7)
	require.NoError(t, err)
	assert.Empty(t, out.RecentSummaries)
	assert.Equal(t, 2, out.EpisodeNumber)
}

func TestBuild_NoPreviousElements_StillReturnsContext(t *testing.T) {
	sl := &model.Storyline{ID: 7, ChildID: 42, Title: "T", EpisodeCount: 0, NextEpisodeHint: "hook"}
	b := NewStorylineContextBuilder(
		&fakeSLRepo{sl: sl},
		&fakeStoryCtxRepo{stories: nil, elements: nil},
		&fakeMemCtxRepo{mems: nil},
		nil,
	)
	out, err := b.Build(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, "hook", out.PreviousHook)
	assert.Empty(t, out.PreviousElements)
	assert.Empty(t, out.RecentSummaries)
	assert.Equal(t, 1, out.EpisodeNumber)
}

func TestBuild_StorylineNotFound_ReturnsError(t *testing.T) {
	b := NewStorylineContextBuilder(
		&fakeSLRepo{err: repository.ErrNotFound},
		&fakeStoryCtxRepo{},
		&fakeMemCtxRepo{},
		nil,
	)
	_, err := b.Build(context.Background(), 999)
	assert.ErrorIs(t, err, repository.ErrNotFound)
}

func TestBuild_RecentStoriesError_FailOpen(t *testing.T) {
	sl := &model.Storyline{ID: 7, ChildID: 42, Title: "T", EpisodeCount: 5}
	b := NewStorylineContextBuilder(
		&fakeSLRepo{sl: sl},
		&fakeStoryCtxRepo{recentErr: errors.New("db boom")},
		&fakeMemCtxRepo{},
		nil,
	)
	out, err := b.Build(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, 6, out.EpisodeNumber)
	assert.Empty(t, out.RecentSummaries)
	assert.Empty(t, out.PreviousElements)
}
