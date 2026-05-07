//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
)

func setupChildRepo(t *testing.T) (UserRepo, ChildRepo, func()) {
	t.Helper()
	pg, cfg := startPG(t)
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	return NewUserRepo(db), NewChildRepo(db), func() {
		Close(db)
		_ = pg.Terminate(context.Background())
	}
}

func makeUser(t *testing.T, repo UserRepo, hash string) *model.User {
	t.Helper()
	u, _, err := repo.CreateOrGet(context.Background(), &model.User{
		PhoneHash: hash, PhoneEncrypted: []byte{1}, Nickname: "x",
	})
	require.NoError(t, err)
	return u
}

func TestChildRepo_Create_AndFindByUserID(t *testing.T) {
	urepo, crepo, cleanup := setupChildRepo(t)
	defer cleanup()

	u := makeUser(t, urepo, "h_a")
	bday, _ := time.Parse("2006-01-02", "2020-08-15")
	c := &model.Child{
		UserID:   u.ID,
		Nickname: "小宇",
		Gender:   "boy",
		Birthday: bday,
		Profile:  []byte(`{}`),
	}
	require.NoError(t, crepo.Create(context.Background(), c))
	assert.NotZero(t, c.ID)

	got, err := crepo.FindByUserID(context.Background(), u.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)
	assert.Equal(t, "小宇", got.Nickname)
}

func TestChildRepo_Create_RejectsDuplicateForSameUser(t *testing.T) {
	urepo, crepo, cleanup := setupChildRepo(t)
	defer cleanup()

	u := makeUser(t, urepo, "h_b")
	bday, _ := time.Parse("2006-01-02", "2020-08-15")

	require.NoError(t, crepo.Create(context.Background(), &model.Child{
		UserID: u.ID, Nickname: "first", Gender: "boy", Birthday: bday, Profile: []byte(`{}`),
	}))
	err := crepo.Create(context.Background(), &model.Child{
		UserID: u.ID, Nickname: "second", Gender: "girl", Birthday: bday, Profile: []byte(`{}`),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAlreadyExists), "expected ErrAlreadyExists, got %v", err)
}

func TestChildRepo_FindByUserID_NotFound(t *testing.T) {
	urepo, crepo, cleanup := setupChildRepo(t)
	defer cleanup()

	u := makeUser(t, urepo, "h_c")
	_, err := crepo.FindByUserID(context.Background(), u.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestChildRepo_FindByID_AndUpdate(t *testing.T) {
	urepo, crepo, cleanup := setupChildRepo(t)
	defer cleanup()

	u := makeUser(t, urepo, "h_d")
	bday, _ := time.Parse("2006-01-02", "2020-08-15")
	c := &model.Child{UserID: u.ID, Nickname: "n", Gender: "boy", Birthday: bday, Profile: []byte(`{}`)}
	require.NoError(t, crepo.Create(context.Background(), c))

	c.Nickname = "n2"
	require.NoError(t, crepo.Update(context.Background(), c))

	got, err := crepo.FindByID(context.Background(), c.ID)
	require.NoError(t, err)
	assert.Equal(t, "n2", got.Nickname)
}
