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

func TestMemoryRepo_RecentByChildTypes(t *testing.T) {
	pg, cfg := startPG(t)
	defer func() { _ = pg.Terminate(context.Background()) }()
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	defer Close(db)

	urepo := NewUserRepo(db)
	crepo := NewChildRepo(db)
	mrepo := NewMemoryRepo(db)

	u, _, _ := urepo.CreateOrGet(context.Background(), &model.User{PhoneHash: "h1", PhoneEncrypted: []byte{1}, Nickname: "n"})
	c := &model.Child{UserID: u.ID, Nickname: "小宇", Gender: "boy", Birthday: timeFromString(t, "2020-08-15"), Profile: []byte(`{}`)}
	require.NoError(t, crepo.Create(context.Background(), c))

	u2, _, _ := urepo.CreateOrGet(context.Background(), &model.User{PhoneHash: "h2", PhoneEncrypted: []byte{2}, Nickname: "n2"})
	c2 := &model.Child{UserID: u2.ID, Nickname: "小红", Gender: "girl", Birthday: timeFromString(t, "2020-08-15"), Profile: []byte(`{}`)}
	require.NoError(t, crepo.Create(context.Background(), c2))

	require.NoError(t, mrepo.Create(context.Background(), &model.Memory{ChildID: c.ID, MemoryType: "story_summary", Payload: []byte(`{"summary":"s1"}`), Weight: 1.0}))
	require.NoError(t, mrepo.Create(context.Background(), &model.Memory{ChildID: c.ID, MemoryType: "interest", Payload: []byte(`{"summary":"i1"}`), Weight: 1.0}))
	require.NoError(t, mrepo.Create(context.Background(), &model.Memory{ChildID: c.ID, MemoryType: "preference", Payload: []byte(`{"summary":"p1"}`), Weight: 1.0}))
	// other child's memory; must NOT leak
	require.NoError(t, mrepo.Create(context.Background(), &model.Memory{ChildID: c2.ID, MemoryType: "story_summary", Payload: []byte(`{"summary":"other"}`), Weight: 1.0}))

	rows, err := mrepo.RecentByChildTypes(context.Background(), c.ID, []string{"story_summary", "interest"}, 5)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
	for _, r := range rows {
		assert.Equal(t, c.ID, r.ChildID)
		assert.Contains(t, []string{"story_summary", "interest"}, r.MemoryType)
	}

	// Empty types short-circuits to empty slice.
	empty, err := mrepo.RecentByChildTypes(context.Background(), c.ID, nil, 5)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func timeFromString(t *testing.T, s string) (out time.Time) {
	t.Helper()
	out, err := time.Parse("2006-01-02", s)
	require.NoError(t, err)
	return
}
