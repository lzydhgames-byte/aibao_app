package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/aibao/server/internal/model"
)

// OutboxRepo is the data-access surface for outbox_events.
type OutboxRepo interface {
	// FetchPending atomically claims up to limit pending events for processing,
	// marking them status='processing'. Uses SELECT ... FOR UPDATE SKIP LOCKED
	// so multiple workers won't grab the same event.
	FetchPending(ctx context.Context, limit int) ([]*model.OutboxEvent, error)

	// MarkDone sets status='done'.
	MarkDone(ctx context.Context, id int64) error

	// MarkFailed records an attempt failure. If attempts+1 >= maxAttempts,
	// status='dead' (DLQ); otherwise status reverts to 'pending' with
	// next_attempt_at = now + backoff.
	MarkFailed(ctx context.Context, id int64, errMsg string, backoff time.Duration, maxAttempts int) error

	// PendingCount returns the current number of pending events (for metrics).
	PendingCount(ctx context.Context) (int64, error)
}

type outboxRepo struct {
	db *gorm.DB
}

// NewOutboxRepo constructs a GORM-backed OutboxRepo.
func NewOutboxRepo(db *gorm.DB) OutboxRepo { return &outboxRepo{db: db} }

func (r *outboxRepo) FetchPending(ctx context.Context, limit int) ([]*model.OutboxEvent, error) {
	if limit <= 0 {
		limit = 10
	}
	var out []*model.OutboxEvent
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// SELECT ... FOR UPDATE SKIP LOCKED — gorm clause.Locking with `Options: "SKIP LOCKED"`
		err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_attempt_at <= ?", model.OutboxStatusPending, time.Now()).
			Order("id").
			Limit(limit).
			Find(&out).Error
		if err != nil {
			return err
		}
		if len(out) == 0 {
			return nil
		}
		ids := make([]int64, len(out))
		for i, e := range out {
			ids[i] = e.ID
			e.Status = model.OutboxStatusProcessing
		}
		// Single UPDATE for all claimed ids.
		return tx.Model(&model.OutboxEvent{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"status":     model.OutboxStatusProcessing,
				"updated_at": time.Now(),
			}).Error
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *outboxRepo) MarkDone(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&model.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     model.OutboxStatusDone,
			"updated_at": time.Now(),
		}).Error
}

func (r *outboxRepo) MarkFailed(ctx context.Context, id int64, errMsg string, backoff time.Duration, maxAttempts int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e model.OutboxEvent
		if err := tx.First(&e, id).Error; err != nil {
			return err
		}
		e.Attempts++
		e.LastError = errMsg
		e.UpdatedAt = time.Now()
		if e.Attempts >= maxAttempts {
			e.Status = model.OutboxStatusDead
		} else {
			e.Status = model.OutboxStatusPending
			e.NextAttemptAt = time.Now().Add(backoff)
		}
		return tx.Save(&e).Error
	})
}

func (r *outboxRepo) PendingCount(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.OutboxEvent{}).
		Where("status = ?", model.OutboxStatusPending).
		Count(&n).Error
	return n, err
}
