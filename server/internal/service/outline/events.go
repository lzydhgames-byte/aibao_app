package outline

import (
	"context"
	"time"

	"github.com/aibao/server/internal/model"
	"gorm.io/gorm"
)

// outline_events.outcome values (append-only event stream, spec §5.5).
const (
	OutcomePending   = "pending"
	OutcomeAccepted  = "accepted"
	OutcomeRefreshed = "refreshed"
	OutcomeExpired   = "expired"
)

// EventStore writes append-only lifecycle events to outline_events.
// Per spec §5.5 T3, events are NEVER updated; each state transition
// appends a new row. "Latest state" is computed via DISTINCT ON.
type EventStore struct {
	db *gorm.DB
}

func NewEventStore(db *gorm.DB) *EventStore { return &EventStore{db: db} }

// Append inserts a new event row. Caller is responsible for not double-counting
// (e.g. don't append expired if accepted already exists for this outline_id).
// Use MarkExpiredIfPending below for the idempotent expired-marking case.
func (s *EventStore) Append(ctx context.Context, evt model.OutlineEvent) error {
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = time.Now()
	}
	return s.db.WithContext(ctx).Create(&evt).Error
}

// LatestOutcome returns the most recent outcome for an outline_id,
// or "" if no rows exist.
func (s *EventStore) LatestOutcome(ctx context.Context, outlineID string) (string, error) {
	var evt model.OutlineEvent
	err := s.db.WithContext(ctx).
		Where("outline_id = ?", outlineID).
		Order("occurred_at DESC, id DESC").
		Limit(1).
		First(&evt).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return evt.Outcome, nil
}

// MarkExpiredIfPending appends an expired row only if no terminal outcome
// (accepted/refreshed/expired) already exists for this outline_id.
// Idempotent — safe to call repeatedly. Spec §5.5 T3 append-only mode.
func (s *EventStore) MarkExpiredIfPending(ctx context.Context, evt model.OutlineEvent) error {
	const sql = `
INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome, outline_prompt_version, duration_min, trace_id)
SELECT ?, ?, ?, ?, ?, 'expired', ?, ?, ?
WHERE NOT EXISTS (
    SELECT 1 FROM outline_events
    WHERE outline_id = ? AND outcome IN ('accepted', 'refreshed', 'expired')
)`
	return s.db.WithContext(ctx).Exec(sql,
		time.Now(), evt.OutlineID, evt.OutlineGroupID, evt.UserID, evt.ChildIDHash,
		evt.OutlinePromptVersion, evt.DurationMin, evt.TraceID,
		evt.OutlineID,
	).Error
}

// ScanPendingOlderThan returns outline rows still in pending state older than
// threshold, for which no terminal outcome exists.
// Used by housekeeping + on-demand SweepUser (spec §5.5 A2).
// If userID is non-nil, scan is restricted to that user.
func (s *EventStore) ScanPendingOlderThan(ctx context.Context, threshold time.Time, userID *int64, limit int) ([]model.OutlineEvent, error) {
	var out []model.OutlineEvent
	q := s.db.WithContext(ctx)
	if userID != nil {
		err := q.Raw(`
SELECT DISTINCT ON (outline_id) *
FROM outline_events
WHERE occurred_at < ?
  AND outcome = 'pending'
  AND user_id = ?
  AND NOT EXISTS (
      SELECT 1 FROM outline_events e2
      WHERE e2.outline_id = outline_events.outline_id
        AND e2.outcome IN ('accepted','refreshed','expired')
  )
ORDER BY outline_id, occurred_at DESC, id DESC
LIMIT ?`, threshold, *userID, limit).Scan(&out).Error
		return out, err
	}
	err := q.Raw(`
SELECT DISTINCT ON (outline_id) *
FROM outline_events
WHERE occurred_at < ?
  AND outcome = 'pending'
  AND NOT EXISTS (
      SELECT 1 FROM outline_events e2
      WHERE e2.outline_id = outline_events.outline_id
        AND e2.outcome IN ('accepted','refreshed','expired')
  )
ORDER BY outline_id, occurred_at DESC, id DESC
LIMIT ?`, threshold, limit).Scan(&out).Error
	return out, err
}
