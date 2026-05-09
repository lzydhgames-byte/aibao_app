// Package worker hosts the outbox event consumer. Each Handler is registered
// against an event_type; the main loop polls the outbox table and dispatches.
package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/logger"
	"github.com/aibao/server/internal/repository"
)

// Handler processes a single outbox event. Implementations must be idempotent:
// a duplicate payload (e.g. retry after partial success) must produce the same
// final state.
type Handler interface {
	Handle(ctx context.Context, event *model.OutboxEvent) error
}

// Worker is the outbox poller.
type Worker struct {
	repo         repository.OutboxRepo
	handlers     map[string]Handler
	pollInterval time.Duration
	batchSize    int
	maxAttempts  int
	backoffBase  time.Duration
	backoffMax   time.Duration
}

// Config is the Worker's runtime config.
type Config struct {
	PollInterval time.Duration
	BatchSize    int
	MaxAttempts  int
	BackoffBase  time.Duration
	BackoffMax   time.Duration
}

// New constructs a Worker. Register handlers via Register before Run.
func New(repo repository.OutboxRepo, cfg Config) *Worker {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 10
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.BackoffBase == 0 {
		cfg.BackoffBase = 2 * time.Second
	}
	if cfg.BackoffMax == 0 {
		cfg.BackoffMax = 10 * time.Minute
	}
	return &Worker{
		repo:         repo,
		handlers:     map[string]Handler{},
		pollInterval: cfg.PollInterval,
		batchSize:    cfg.BatchSize,
		maxAttempts:  cfg.MaxAttempts,
		backoffBase:  cfg.BackoffBase,
		backoffMax:   cfg.BackoffMax,
	}
}

// Register attaches a Handler to an event_type.
func (w *Worker) Register(eventType string, h Handler) {
	w.handlers[eventType] = h
}

// Run blocks until ctx is canceled, polling at PollInterval.
func (w *Worker) Run(ctx context.Context) {
	lg := logger.Default().With("module", "worker")
	lg.Info("worker.start", "poll", w.pollInterval.String(), "batch", w.batchSize)
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			lg.Info("worker.stop")
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

// tick processes one batch of pending events.
func (w *Worker) tick(ctx context.Context) {
	lg := logger.Default().With("module", "worker")
	events, err := w.repo.FetchPending(ctx, w.batchSize)
	if err != nil {
		lg.Error("worker.fetch_failed", "err", err.Error())
		return
	}
	for _, e := range events {
		w.processOne(ctx, e)
	}
}

// processOne dispatches one event to its handler.
func (w *Worker) processOne(ctx context.Context, e *model.OutboxEvent) {
	lg := logger.Default().With("module", "worker", "event_id", e.ID, "event_type", e.EventType)
	h, ok := w.handlers[e.EventType]
	if !ok {
		lg.Warn("worker.no_handler")
		_ = w.repo.MarkFailed(ctx, e.ID, fmt.Sprintf("no handler for %s", e.EventType), w.backoff(e.Attempts), w.maxAttempts)
		return
	}
	if err := h.Handle(ctx, e); err != nil {
		lg.Warn("worker.handle_failed", "err", err.Error(), "attempts", e.Attempts)
		_ = w.repo.MarkFailed(ctx, e.ID, err.Error(), w.backoff(e.Attempts), w.maxAttempts)
		return
	}
	if err := w.repo.MarkDone(ctx, e.ID); err != nil {
		lg.Error("worker.mark_done_failed", "err", err.Error())
	}
}

// backoff returns the wait time for an event with `attempts` past failures.
func (w *Worker) backoff(attempts int) time.Duration {
	d := w.backoffBase
	for i := 0; i < attempts && d < w.backoffMax; i++ {
		d *= 2
	}
	if d > w.backoffMax {
		d = w.backoffMax
	}
	return d
}

// ErrNoHandler is exported for tests.
var ErrNoHandler = errors.New("no handler registered for event type")
