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

func TestMemoryRepo_CreateAndList(t *testing.T) {
	pg, cfg := startPG(t)
	defer func() { _ = pg.Terminate(context.Background()) }()
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	defer Close(db)

	urepo := NewUserRepo(db)
	crepo := NewChildRepo(db)
	mrepo := NewMemoryRepo(db)

	u, _, _ := urepo.CreateOrGet(context.Background(), &model.User{PhoneHash: "h", PhoneEncrypted: []byte{1}, Nickname: "n"})
	c := &model.Child{UserID: u.ID, Nickname: "小宇", Gender: "boy", Birthday: timeFromString(t, "2020-08-15"), Profile: []byte(`{}`)}
	require.NoError(t, crepo.Create(context.Background(), c))

	require.NoError(t, mrepo.Create(context.Background(), &model.Memory{
		ChildID: c.ID, MemoryType: "story_summary", Payload: []byte(`{"title":"a"}`), Weight: 1.0,
	}))
	require.NoError(t, mrepo.Create(context.Background(), &model.Memory{
		ChildID: c.ID, MemoryType: "story_summary", Payload: []byte(`{"title":"b"}`), Weight: 1.0,
	}))

	memos, err := mrepo.RecentByChild(context.Background(), c.ID, "story_summary", 10)
	require.NoError(t, err)
	assert.Len(t, memos, 2)
	// most recent first
	assert.Contains(t, string(memos[0].Payload), "b")
}

func timeFromString(t *testing.T, s string) (out time.Time) {
	t.Helper()
	out, err := time.Parse("2006-01-02", s)
	require.NoError(t, err)
	return
}
