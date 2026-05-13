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

func setupStorylineTestDB(t *testing.T) (StorylineRepo, *model.Child, func()) {
	t.Helper()
	pg, cfg := startPG(t)
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))

	urepo := NewUserRepo(db)
	crepo := NewChildRepo(db)
	u, _, _ := urepo.CreateOrGet(context.Background(), &model.User{PhoneHash: "h", PhoneEncrypted: []byte{1}, Nickname: "n"})
	c := &model.Child{UserID: u.ID, Nickname: "小宇", Gender: "boy", Birthday: timeFromString(t, "2020-08-15"), Profile: []byte(`{}`)}
	require.NoError(t, crepo.Create(context.Background(), c))

	repo := NewStorylineRepo(db)
	return repo, c, func() {
		Close(db)
		_ = pg.Terminate(context.Background())
	}
}

func TestStorylineRepo_Create_AssignsID(t *testing.T) {
	repo, c, cleanup := setupStorylineTestDB(t)
	defer cleanup()
	ctx := context.Background()

	sl := &model.Storyline{ChildID: c.ID, Title: "森林冒险", Status: model.StorylineStatusActive}
	require.NoError(t, repo.Create(ctx, sl))
	assert.Greater(t, sl.ID, int64(0))
}

func TestStorylineRepo_FindByID_NotFound_ReturnsErrNotFound(t *testing.T) {
	repo, _, cleanup := setupStorylineTestDB(t)
	defer cleanup()
	ctx := context.Background()

	sl, err := repo.FindByID(ctx, 99999)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, sl)
}

func TestStorylineRepo_ListActiveByChild_OrdersByLastEpisodeAtDesc(t *testing.T) {
	repo, c, cleanup := setupStorylineTestDB(t)
	defer cleanup()
	ctx := context.Background()

	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-10 * time.Minute)

	a := &model.Storyline{ChildID: c.ID, Title: "A", Status: model.StorylineStatusActive, LastEpisodeAt: &older}
	b := &model.Storyline{ChildID: c.ID, Title: "B", Status: model.StorylineStatusActive, LastEpisodeAt: &newer}
	require.NoError(t, repo.Create(ctx, a))
	require.NoError(t, repo.Create(ctx, b))

	list, err := repo.ListActiveByChild(ctx, c.ID, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "B", list[0].Title)
	assert.Equal(t, "A", list[1].Title)
}

func TestStorylineRepo_ListActiveByChild_ExcludesCompleted(t *testing.T) {
	repo, c, cleanup := setupStorylineTestDB(t)
	defer cleanup()
	ctx := context.Background()

	active := &model.Storyline{ChildID: c.ID, Title: "Active", Status: model.StorylineStatusActive}
	completed := &model.Storyline{ChildID: c.ID, Title: "Done", Status: model.StorylineStatusCompleted}
	require.NoError(t, repo.Create(ctx, active))
	require.NoError(t, repo.Create(ctx, completed))

	list, err := repo.ListActiveByChild(ctx, c.ID, 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Active", list[0].Title)
}

func TestStorylineRepo_IncrementEpisode_BumpsCountAndHint(t *testing.T) {
	repo, c, cleanup := setupStorylineTestDB(t)
	defer cleanup()
	ctx := context.Background()

	sl := &model.Storyline{ChildID: c.ID, Title: "T", Status: model.StorylineStatusActive, EpisodeCount: 2}
	require.NoError(t, repo.Create(ctx, sl))

	require.NoError(t, repo.IncrementEpisode(ctx, sl.ID, "下次去山洞"))

	got, err := repo.FindByID(ctx, sl.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, got.EpisodeCount)
	assert.Equal(t, "下次去山洞", got.NextEpisodeHint)
	require.NotNil(t, got.LastEpisodeAt)
	assert.WithinDuration(t, time.Now(), *got.LastEpisodeAt, 10*time.Second)
}
