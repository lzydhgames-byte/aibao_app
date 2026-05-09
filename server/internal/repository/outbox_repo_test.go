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

func TestOutboxRepo_FetchPendingMarksProcessing(t *testing.T) {
	pg, cfg := startPG(t)
	defer func() { _ = pg.Terminate(context.Background()) }()
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	defer Close(db)
	repo := NewOutboxRepo(db)

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&model.OutboxEvent{
			EventType: model.EventTypeMemoryUpdate,
			Payload:   []byte(`{}`),
			Status:    model.OutboxStatusPending,
		}).Error)
	}

	got, err := repo.FetchPending(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, got, 3)
	for _, e := range got {
		assert.Equal(t, model.OutboxStatusProcessing, e.Status)
	}
}

func TestOutboxRepo_MarkDone(t *testing.T) {
	pg, cfg := startPG(t)
	defer func() { _ = pg.Terminate(context.Background()) }()
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	defer Close(db)
	repo := NewOutboxRepo(db)

	e := &model.OutboxEvent{EventType: model.EventTypeMemoryUpdate, Payload: []byte(`{}`), Status: model.OutboxStatusProcessing}
	require.NoError(t, db.Create(e).Error)

	require.NoError(t, repo.MarkDone(context.Background(), e.ID))

	var reloaded model.OutboxEvent
	require.NoError(t, db.First(&reloaded, e.ID).Error)
	assert.Equal(t, model.OutboxStatusDone, reloaded.Status)
}

func TestOutboxRepo_MarkFailed_Backoff(t *testing.T) {
	pg, cfg := startPG(t)
	defer func() { _ = pg.Terminate(context.Background()) }()
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	defer Close(db)
	repo := NewOutboxRepo(db)

	e := &model.OutboxEvent{EventType: "memory_update", Payload: []byte(`{}`), Status: model.OutboxStatusProcessing}
	require.NoError(t, db.Create(e).Error)

	before := time.Now()
	require.NoError(t, repo.MarkFailed(context.Background(), e.ID, "boom", time.Minute, 5))
	var reloaded model.OutboxEvent
	require.NoError(t, db.First(&reloaded, e.ID).Error)
	assert.Equal(t, 1, reloaded.Attempts)
	assert.Equal(t, model.OutboxStatusPending, reloaded.Status)
	assert.True(t, reloaded.NextAttemptAt.After(before))
	assert.Equal(t, "boom", reloaded.LastError)
}

func TestOutboxRepo_MarkFailed_DLQOnMaxAttempts(t *testing.T) {
	pg, cfg := startPG(t)
	defer func() { _ = pg.Terminate(context.Background()) }()
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	defer Close(db)
	repo := NewOutboxRepo(db)

	e := &model.OutboxEvent{EventType: "memory_update", Payload: []byte(`{}`), Status: model.OutboxStatusProcessing, Attempts: 4}
	require.NoError(t, db.Create(e).Error)

	require.NoError(t, repo.MarkFailed(context.Background(), e.ID, "perma-fail", time.Minute, 5))

	var reloaded model.OutboxEvent
	require.NoError(t, db.First(&reloaded, e.ID).Error)
	assert.Equal(t, model.OutboxStatusDead, reloaded.Status)
}

func TestOutboxRepo_PendingCount(t *testing.T) {
	pg, cfg := startPG(t)
	defer func() { _ = pg.Terminate(context.Background()) }()
	db, err := NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, autoMigrateForTest(db))
	defer Close(db)
	repo := NewOutboxRepo(db)

	for i := 0; i < 4; i++ {
		require.NoError(t, db.Create(&model.OutboxEvent{
			EventType: "memory_update",
			Payload:   []byte(`{}`),
			Status:    model.OutboxStatusPending,
		}).Error)
	}
	n, err := repo.PendingCount(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 4, n)
}
