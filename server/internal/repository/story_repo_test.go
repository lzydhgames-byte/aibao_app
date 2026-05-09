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

	require.NoError(t, srepo.CreateWithOutbox(context.Background(), story, elements, event))
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
	require.NoError(t, srepo.CreateWithOutbox(context.Background(), story, nil, &model.OutboxEvent{
		EventType: model.EventTypeMemoryUpdate, Payload: []byte(`{}`), Status: model.OutboxStatusPending,
	}))

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
