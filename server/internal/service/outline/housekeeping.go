// Package outline — housekeeping for orphan pending outline_events rows.
//
// Spec §5.5 A2 dual path:
//   - Proactive: SweepUser is called by /stories list + /heartbeat handlers
//     on every request to scan that user's recently-abandoned pending rows.
//   - Fallback: Run is launched as a background goroutine from main.go and
//     ticks every houseKeepInterval doing a full-table scan for orphan rows
//     belonging to users with no active session.
package outline

import (
	"context"
	"time"

	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/logger"
)

const (
	// houseKeepInterval is the periodic sweep tick for orphan outlines
	// (no active user to drive SweepUser). 10min is loose since the
	// Redis cache TTL (5min) means most pending rows get swept by the
	// owner's next /stories or /heartbeat hit.
	houseKeepInterval = 10 * time.Minute

	// pendingThreshold is the age past which a pending outline is
	// considered abandoned. Slightly larger than Redis TTL so we don't
	// race normal Service.Preview pending writes.
	pendingThreshold = 10 * time.Minute

	// userSweepGrace is what /stories + /heartbeat use when sweeping the
	// active user's own pending outlines — tighter (just past Redis TTL)
	// since the user is online and we know they've left those tickets.
	userSweepGrace = 5*time.Minute + 30*time.Second

	// batchLimit caps a single sweep pass to avoid long-running queries
	// on /stories or /heartbeat (user-facing).
	batchLimit = 200
)

// Housekeeper expires abandoned pending outline_events rows.
// Spec §5.5 A2: dual path — proactive SweepUser at request time +
// fallback Run loop for orphan cleanup.
type Housekeeper struct {
	events *EventStore
	biz    *metrics.Business // nil-safe
}

// NewHousekeeper constructs a Housekeeper. biz may be nil for tests.
func NewHousekeeper(events *EventStore, biz *metrics.Business) *Housekeeper {
	return &Housekeeper{events: events, biz: biz}
}

// Run blocks until ctx is cancelled, performing a full-table sweep on every
// houseKeepInterval tick. Intended use: `go hk.Run(ctx)` from main.go.
func (h *Housekeeper) Run(ctx context.Context) {
	tick := time.NewTicker(houseKeepInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			h.sweepAll(ctx)
		}
	}
}

// RunOnce performs a single full-table sweep and returns. Useful for tests
// and as a future manual trigger from an admin endpoint. Production loop
// uses Run() instead.
func (h *Housekeeper) RunOnce(ctx context.Context) error {
	h.sweepAll(ctx)
	return nil
}

// sweepAll scans the entire outline_events table for pending rows older than
// pendingThreshold and appends an expired event for each (idempotent).
func (h *Housekeeper) sweepAll(ctx context.Context) {
	threshold := time.Now().Add(-pendingThreshold)
	rows, err := h.events.ScanPendingOlderThan(ctx, threshold, nil, batchLimit)
	if err != nil {
		logger.FromCtx(ctx).Error("outline.housekeeping.scan_failed", "err", err)
		return
	}
	for _, row := range rows {
		if err := h.events.MarkExpiredIfPending(ctx, model.OutlineEvent{
			OutlineID: row.OutlineID, OutlineGroupID: row.OutlineGroupID,
			UserID: row.UserID, ChildIDHash: row.ChildIDHash,
			OutlinePromptVersion: row.OutlinePromptVersion,
			DurationMin:          row.DurationMin, TraceID: row.TraceID,
		}); err != nil {
			logger.FromCtx(ctx).Warn("outline.housekeeping.mark_failed",
				"outline_id", row.OutlineID, "err", err)
			continue
		}
		if h.biz != nil {
			h.biz.OutlineOutcomeTotal.WithLabelValues(OutcomeExpired).Inc()
		}
	}
	if len(rows) > 0 {
		logger.FromCtx(ctx).Info("outline.housekeeping.swept",
			"expired_count", len(rows))
	}
}

// SweepUser performs on-demand expired-marking for a single user's pending
// outlines. Called from /stories list + /heartbeat handlers (spec §5.5 A2
// proactive path). Best-effort: errors are logged but do not block the caller.
func (h *Housekeeper) SweepUser(ctx context.Context, userID int64) {
	threshold := time.Now().Add(-userSweepGrace)
	uid := userID
	rows, err := h.events.ScanPendingOlderThan(ctx, threshold, &uid, batchLimit)
	if err != nil {
		logger.FromCtx(ctx).Warn("outline.housekeeping.user_sweep_failed",
			"user_id", userID, "err", err)
		return
	}
	for _, row := range rows {
		_ = h.events.MarkExpiredIfPending(ctx, model.OutlineEvent{
			OutlineID: row.OutlineID, OutlineGroupID: row.OutlineGroupID,
			UserID: row.UserID, ChildIDHash: row.ChildIDHash,
			OutlinePromptVersion: row.OutlinePromptVersion,
			DurationMin:          row.DurationMin, TraceID: row.TraceID,
		})
		if h.biz != nil {
			h.biz.OutlineOutcomeTotal.WithLabelValues(OutcomeExpired).Inc()
		}
	}
}
