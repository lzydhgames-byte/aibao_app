//go:build integration

package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/config"
	"github.com/aibao/server/internal/repository"
)

type echoHandler struct {
	called int
	err    error
}

func (h *echoHandler) Handle(_ context.Context, _ *model.OutboxEvent) error {
	h.called++
	return h.err
}

// freshDB starts a dedicated PG container, runs AutoMigrate, and returns
// an OutboxRepo, the underlying *gorm.DB (for seeding/inspection), and a
// cleanup func. We can't import repository's integration-tagged helpers
// directly (build tag mismatch); instead, inline the minimal setup using
// testcontainers + GORM AutoMigrate against model.OutboxEvent.
func freshDB(t *testing.T) (repository.OutboxRepo, *gorm.DB, func()) {
	t.Helper()
	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("aibao"),
		postgres.WithUsername("aibao"),
		postgres.WithPassword("aibao"),
		tc.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)

	host, err := pg.Host(ctx)
	require.NoError(t, err)
	port, err := pg.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	cfg := config.PostgresConfig{
		Host: host, Port: int(port.Num()), Database: "aibao",
		User: "aibao", Password: "aibao", SSLMode: "disable",
	}
	db, err := repository.NewDB(cfg)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.OutboxEvent{}))
	return repository.NewOutboxRepo(db), db, func() {
		repository.Close(db)
		_ = pg.Terminate(context.Background())
	}
}

func TestWorker_HappyPath(t *testing.T) {
	repo, db, cleanup := freshDB(t)
	defer cleanup()

	w := New(repo, Config{
		PollInterval: 50 * time.Millisecond,
		BatchSize:    10,
		MaxAttempts:  3,
		BackoffBase:  10 * time.Millisecond,
		BackoffMax:   100 * time.Millisecond,
	})
	h := &echoHandler{}
	w.Register("memory_update", h)

	payload, _ := json.Marshal(map[string]any{"hello": "world"})
	ev := &model.OutboxEvent{
		EventType:     "memory_update",
		Payload:       payload,
		Status:        model.OutboxStatusPending,
		NextAttemptAt: time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(ev).Error)
	require.NotZero(t, ev.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	require.Eventually(t, func() bool {
		var got model.OutboxEvent
		if err := db.First(&got, ev.ID).Error; err != nil {
			return false
		}
		return got.Status == model.OutboxStatusDone
	}, 3*time.Second, 50*time.Millisecond, "event should reach status=done")

	assert.GreaterOrEqual(t, h.called, 1)
}

func TestWorker_NoHandler_MarksFailed(t *testing.T) {
	repo, db, cleanup := freshDB(t)
	defer cleanup()

	w := New(repo, Config{
		PollInterval: 50 * time.Millisecond,
		BatchSize:    10,
		MaxAttempts:  2,
		BackoffBase:  10 * time.Millisecond,
		BackoffMax:   50 * time.Millisecond,
	})
	// Intentionally no Register call.

	payload, _ := json.Marshal(map[string]any{"x": 1})
	ev := &model.OutboxEvent{
		EventType:     "unknown_type",
		Payload:       payload,
		Status:        model.OutboxStatusPending,
		NextAttemptAt: time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(ev).Error)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Wait until at least one failed attempt is recorded.
	require.Eventually(t, func() bool {
		var got model.OutboxEvent
		if err := db.First(&got, ev.ID).Error; err != nil {
			return false
		}
		return got.Attempts >= 1 && got.LastError != ""
	}, 3*time.Second, 50*time.Millisecond, "event should record a failed attempt")

	var got model.OutboxEvent
	require.NoError(t, db.First(&got, ev.ID).Error)
	assert.Contains(t, got.LastError, "no handler")
	// After MaxAttempts (2) is reached, status should be 'dead'; otherwise still 'pending'.
	assert.Contains(t, []string{model.OutboxStatusPending, model.OutboxStatusDead}, got.Status)
}
