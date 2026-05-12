//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
)

func setupBGMTestDB(t *testing.T) (BGMRepo, func()) {
	t.Helper()
	pg, cfg := startPG(t)
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	repo := NewBGMRepo(db)
	return repo, func() {
		Close(db)
		_ = pg.Terminate(context.Background())
	}
}

func TestBGMRepo_PickByMood(t *testing.T) {
	repo, cleanup := setupBGMTestDB(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, &model.BGMAsset{
		Mood: model.MoodWarm, Filename: "warm_01.mp3",
		ObjectKey: "bgm/warm/warm_01.mp3", DurationSec: 60, License: "CC0", Active: true,
	}))
	require.NoError(t, repo.Upsert(ctx, &model.BGMAsset{
		Mood: model.MoodWarm, Filename: "warm_02.mp3",
		ObjectKey: "bgm/warm/warm_02.mp3", DurationSec: 60, License: "CC0", Active: true,
	}))
	require.NoError(t, repo.Upsert(ctx, &model.BGMAsset{
		Mood: model.MoodAdventure, Filename: "adv_01.mp3",
		ObjectKey: "bgm/adventure/adv_01.mp3", DurationSec: 60, License: "CC0", Active: true,
	}))

	pick, err := repo.PickByMood(ctx, model.MoodWarm)
	require.NoError(t, err)
	require.NotNil(t, pick)
	assert.Equal(t, model.MoodWarm, pick.Mood)
	assert.Contains(t, []string{"warm_01.mp3", "warm_02.mp3"}, pick.Filename)
}

func TestBGMRepo_PickByMood_InactiveExcluded(t *testing.T) {
	repo, cleanup := setupBGMTestDB(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, &model.BGMAsset{
		Mood: model.MoodWarm, Filename: "warm_inactive.mp3",
		ObjectKey: "bgm/warm/warm_inactive.mp3", DurationSec: 60, License: "CC0", Active: false,
	}))

	pick, err := repo.PickByMood(ctx, model.MoodWarm)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, pick)
}

func TestBGMRepo_PickByMood_NotFound(t *testing.T) {
	repo, cleanup := setupBGMTestDB(t)
	defer cleanup()
	ctx := context.Background()

	pick, err := repo.PickByMood(ctx, model.MoodMagic)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, pick)
}

func TestBGMRepo_Upsert_Insert(t *testing.T) {
	repo, cleanup := setupBGMTestDB(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, &model.BGMAsset{
		Mood: model.MoodFunny, Filename: "funny_01.mp3",
		ObjectKey: "bgm/funny/funny_01.mp3", DurationSec: 45, License: "CC0", Active: true,
	}))

	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, "funny_01.mp3", all[0].Filename)
}

func TestBGMRepo_Upsert_Update(t *testing.T) {
	repo, cleanup := setupBGMTestDB(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, &model.BGMAsset{
		Mood: model.MoodCurious, Filename: "cur_01.mp3",
		ObjectKey: "bgm/curious/cur_01.mp3", DurationSec: 30, License: "CC0", Active: true,
	}))
	// Upsert same filename, change license + duration.
	require.NoError(t, repo.Upsert(ctx, &model.BGMAsset{
		Mood: model.MoodCurious, Filename: "cur_01.mp3",
		ObjectKey: "bgm/curious/cur_01.mp3", DurationSec: 99, License: "UPDATED", Active: true,
	}))

	all, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, 99, all[0].DurationSec)
	assert.Equal(t, "UPDATED", all[0].License)
}
