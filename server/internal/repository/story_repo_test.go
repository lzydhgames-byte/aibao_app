//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
)

func setupStoryRepo(t *testing.T) (UserRepo, ChildRepo, StoryRepo, *model.Child, func()) {
	t.Helper()
	pg, cfg := startPG(t)
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))

	urepo := NewUserRepo(db)
	crepo := NewChildRepo(db)
	srepo := NewStoryRepo(db)

	u, _, err := urepo.CreateOrGet(context.Background(), &model.User{
		PhoneHash: "h_x", PhoneEncrypted: []byte{1}, Nickname: "n",
	})
	require.NoError(t, err)
	bday, _ := time.Parse("2006-01-02", "2020-08-15")
	c := &model.Child{UserID: u.ID, Nickname: "小宇", Gender: "boy", Birthday: bday, Profile: []byte(`{}`)}
	require.NoError(t, crepo.Create(context.Background(), c))

	return urepo, crepo, srepo, c, func() {
		Close(db)
		_ = pg.Terminate(context.Background())
	}
}

func TestStoryRepo_CreateWithOutbox_AtomicSuccess(t *testing.T) {
	_, _, srepo, child, cleanup := setupStoryRepo(t)
	defer cleanup()

	story := &model.Story{
		ChildID:         child.ID,
		Title:           "小宇的勇敢冒险",
		TextContent:     "故事正文...",
		DurationMinutes: 10,
		Style:           "温馨治愈",
		Topic:           "勇敢",
		PromptVersion:   "v1",
	}
	elements := []*model.StoryElement{
		{ElementType: "character", Name: "爱宝奥特曼", RecallWeight: 1.0},
		{ElementType: "place", Name: "竹林", RecallWeight: 1.0},
	}
	event := &model.OutboxEvent{
		EventType: model.EventTypeMemoryUpdate,
		Payload:   []byte(`{"foo":"bar"}`),
		Status:    model.OutboxStatusPending,
	}

	require.NoError(t, srepo.CreateWithOutbox(context.Background(), story, elements, []*model.OutboxEvent{event}))
	assert.NotZero(t, story.ID)
	assert.NotZero(t, event.ID)
	for _, e := range elements {
		assert.NotZero(t, e.ID)
		assert.Equal(t, story.ID, e.StoryID)
	}
}

func TestStoryRepo_FindByID(t *testing.T) {
	_, _, srepo, child, cleanup := setupStoryRepo(t)
	defer cleanup()

	story := &model.Story{ChildID: child.ID, TextContent: "x", DurationMinutes: 10, Style: "温馨治愈", PromptVersion: "v1"}
	require.NoError(t, srepo.CreateWithOutbox(context.Background(), story, nil, []*model.OutboxEvent{{
		EventType: model.EventTypeMemoryUpdate, Payload: []byte(`{}`), Status: model.OutboxStatusPending,
	}}))

	got, err := srepo.FindByID(context.Background(), story.ID)
	require.NoError(t, err)
	assert.Equal(t, story.ID, got.ID)
	assert.Equal(t, "温馨治愈", got.Style)
}

func TestStoryRepo_FindByID_NotFound(t *testing.T) {
	_, _, srepo, _, cleanup := setupStoryRepo(t)
	defer cleanup()
	_, err := srepo.FindByID(context.Background(), 9999)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestStoryRepo_MarkAudioReady(t *testing.T) {
	_, _, srepo, child, cleanup := setupStoryRepo(t)
	defer cleanup()

	story := &model.Story{
		ChildID: child.ID, TextContent: "x", DurationMinutes: 10, Style: "温馨治愈", PromptVersion: "v1",
		AudioStatus: model.AudioStatusPending,
	}
	require.NoError(t, srepo.CreateWithOutbox(context.Background(), story, nil, []*model.OutboxEvent{{
		EventType: model.EventTypeMemoryUpdate, Payload: []byte(`{}`), Status: model.OutboxStatusPending,
	}}))

	require.NoError(t, srepo.MarkAudioReady(context.Background(), story.ID,
		"audio/1/42-x.mp3", "mp3", 12345, 600, true))

	got, err := srepo.FindByID(context.Background(), story.ID)
	require.NoError(t, err)
	assert.Equal(t, model.AudioStatusReady, got.AudioStatus)
	assert.Equal(t, "audio/1/42-x.mp3", got.AudioObjectKey)
	assert.Equal(t, int64(12345), got.AudioSizeBytes)
	assert.Equal(t, 600, got.AudioDurationSeconds)
	assert.True(t, got.HasBGM)
}

func TestStoryRepo_MarkAudioFailed(t *testing.T) {
	_, _, srepo, child, cleanup := setupStoryRepo(t)
	defer cleanup()

	story := &model.Story{
		ChildID: child.ID, TextContent: "x", DurationMinutes: 10, Style: "温馨治愈", PromptVersion: "v1",
		AudioStatus: model.AudioStatusPending,
	}
	require.NoError(t, srepo.CreateWithOutbox(context.Background(), story, nil, []*model.OutboxEvent{{
		EventType: model.EventTypeMemoryUpdate, Payload: []byte(`{}`), Status: model.OutboxStatusPending,
	}}))

	require.NoError(t, srepo.MarkAudioFailed(context.Background(), story.ID, "minimax 502"))

	got, err := srepo.FindByID(context.Background(), story.ID)
	require.NoError(t, err)
	assert.Equal(t, model.AudioStatusFailed, got.AudioStatus)
	require.NotNil(t, got.AudioFailedAt)
}

func TestStoryRepo_RecentByStoryline_ReturnsOrderedByEpisodeDesc(t *testing.T) {
	pg, cfg := startPG(t)
	defer func() { _ = pg.Terminate(context.Background()) }()
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	defer Close(db)

	ctx := context.Background()
	urepo := NewUserRepo(db)
	crepo := NewChildRepo(db)
	srepo := NewStoryRepo(db)
	slrepo := NewStorylineRepo(db)

	u, _, err := urepo.CreateOrGet(ctx, &model.User{PhoneHash: "h_rs", PhoneEncrypted: []byte{1}, Nickname: "n"})
	require.NoError(t, err)
	bday, _ := time.Parse("2006-01-02", "2020-08-15")
	child := &model.Child{UserID: u.ID, Nickname: "小宇", Gender: "boy", Birthday: bday, Profile: []byte(`{}`)}
	require.NoError(t, crepo.Create(ctx, child))

	sl := &model.Storyline{ChildID: child.ID, Title: "S", Status: model.StorylineStatusActive}
	require.NoError(t, slrepo.Create(ctx, sl))

	mk := func(ep int) *model.Story {
		epNo := ep
		s := &model.Story{
			ChildID: child.ID, TextContent: "x", DurationMinutes: 10, Style: "温馨治愈", PromptVersion: "v1",
			StorylineID: &sl.ID, EpisodeNo: &epNo,
		}
		require.NoError(t, srepo.CreateWithOutbox(ctx, s, nil, []*model.OutboxEvent{{
			EventType: model.EventTypeMemoryUpdate, Payload: []byte(`{}`), Status: model.OutboxStatusPending,
		}}))
		return s
	}
	mk(1)
	mk(2)
	mk(3)

	list, err := srepo.RecentByStoryline(ctx, sl.ID, 10)
	require.NoError(t, err)
	require.Len(t, list, 3)
	require.NotNil(t, list[0].EpisodeNo)
	assert.Equal(t, 3, *list[0].EpisodeNo)
	assert.Equal(t, 2, *list[1].EpisodeNo)
	assert.Equal(t, 1, *list[2].EpisodeNo)
}

func TestStoryRepo_ElementsByStory_OrderedByWeightDesc(t *testing.T) {
	_, _, srepo, child, cleanup := setupStoryRepo(t)
	defer cleanup()

	story := &model.Story{
		ChildID: child.ID, TextContent: "x", DurationMinutes: 10, Style: "温馨治愈", PromptVersion: "v1",
	}
	elements := []*model.StoryElement{
		{ElementType: "character", Name: "爱宝", RecallWeight: 1.5},
		{ElementType: "place", Name: "竹林", RecallWeight: 0.5},
		{ElementType: "character", Name: "小恐龙", RecallWeight: 2.0},
	}
	require.NoError(t, srepo.CreateWithOutbox(context.Background(), story, elements, []*model.OutboxEvent{{
		EventType: model.EventTypeMemoryUpdate, Payload: []byte(`{}`), Status: model.OutboxStatusPending,
	}}))

	got, err := srepo.ElementsByStory(context.Background(), story.ID, []string{"character", "place"}, 8)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "小恐龙", got[0].Name)
	assert.Equal(t, "爱宝", got[1].Name)
	assert.Equal(t, "竹林", got[2].Name)
}

func TestStoryRepo_RecentByStoryline_EmptyOk(t *testing.T) {
	_, _, srepo, _, cleanup := setupStoryRepo(t)
	defer cleanup()

	list, err := srepo.RecentByStoryline(context.Background(), 999999, 10)
	require.NoError(t, err)
	assert.Empty(t, list)
}
