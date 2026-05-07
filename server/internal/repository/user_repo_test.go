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

func setupUserRepo(t *testing.T) (UserRepo, func()) {
	t.Helper()
	pg, cfg := startPG(t)
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	return NewUserRepo(db), func() {
		Close(db)
		_ = pg.Terminate(context.Background())
	}
}

func TestUserRepo_CreateOrGet_New(t *testing.T) {
	repo, cleanup := setupUserRepo(t)
	defer cleanup()

	u, created, err := repo.CreateOrGet(context.Background(), &model.User{
		PhoneHash:        "h_aaa",
		PhoneEncrypted:   []byte{1, 2, 3},
		Nickname:         "妈妈",
		SubscriptionTier: "free",
	})
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotZero(t, u.ID)
	assert.Equal(t, "妈妈", u.Nickname)
}

func TestUserRepo_CreateOrGet_Existing(t *testing.T) {
	repo, cleanup := setupUserRepo(t)
	defer cleanup()

	first, _, err := repo.CreateOrGet(context.Background(), &model.User{
		PhoneHash:      "h_bbb",
		PhoneEncrypted: []byte{4, 5, 6},
		Nickname:       "first",
	})
	require.NoError(t, err)

	second, created, err := repo.CreateOrGet(context.Background(), &model.User{
		PhoneHash:      "h_bbb",
		PhoneEncrypted: []byte{99, 99, 99},
		Nickname:       "second", // should be ignored, original kept
	})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, "first", second.Nickname)
}

func TestUserRepo_FindByID_Missing(t *testing.T) {
	repo, cleanup := setupUserRepo(t)
	defer cleanup()

	_, err := repo.FindByID(context.Background(), 9999)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUserRepo_FindByID_Existing(t *testing.T) {
	repo, cleanup := setupUserRepo(t)
	defer cleanup()

	u, _, _ := repo.CreateOrGet(context.Background(), &model.User{
		PhoneHash: "h_ccc", PhoneEncrypted: []byte{7}, Nickname: "n",
	})

	got, err := repo.FindByID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	assert.WithinDuration(t, time.Now(), got.CreatedAt, time.Minute)
}
