package cost

import (
	"context"
	"errors"
	"time"

	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// flushInterval is the periodic tick at which queued events are batched
	// and INSERTed. Worst-case data loss on crash is one tick (1 min) of events.
	flushInterval = 60 * time.Second
	// maxBatch caps a single INSERT statement (PG can take much more, but
	// 200 keeps individual transactions bounded — important under contention).
	maxBatch = 200
	// shutdownGrace is how long the final flush has on SIGTERM before
	// being abandoned (events lost; cost_event_record_failed_total{reason=db_write} bumped).
	shutdownGrace = 5 * time.Second
)

// Flusher consumes Recorder.Drain() and batch-INSERTs to cost_events with
// idempotent ON CONFLICT(event_id) DO NOTHING semantics (spec §3.3 / §5.1).
// Spec invariant: business path NEVER blocks on Flusher failures.
type Flusher struct {
	r   *Recorder
	db  *gorm.DB
	biz *metrics.Business
}

// NewFlusher constructs a Flusher. biz may be nil; nil-safe for unit tests.
func NewFlusher(r *Recorder, db *gorm.DB, biz *metrics.Business) *Flusher {
	return &Flusher{r: r, db: db, biz: biz}
}

// Run blocks until ctx is cancelled, then performs a final flush within
// shutdownGrace before returning. Intended use:
//
//	go flusher.Run(ctx)
//
// where ctx is the main signal.NotifyContext.
func (f *Flusher) Run(ctx context.Context) {
	tick := time.NewTicker(flushInterval)
	defer tick.Stop()
	batch := make([]*model.CostEvent, 0, maxBatch)
	for {
		select {
		case <-ctx.Done():
			drainCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
			f.drainAndFlush(drainCtx)
			cancel()
			return
		case <-tick.C:
			batch = f.collect(batch[:0])
			f.flush(ctx, batch)
		}
	}
}

// collect non-blockingly pulls up to maxBatch events from the queue.
func (f *Flusher) collect(batch []*model.CostEvent) []*model.CostEvent {
	for len(batch) < maxBatch {
		select {
		case evt, ok := <-f.r.Drain():
			if !ok {
				return batch
			}
			batch = append(batch, evt)
		default:
			return batch
		}
	}
	return batch
}

// drainAndFlush is called on shutdown — repeatedly collect+flush until queue
// is empty or ctx expires. Used only by Run; not exposed.
func (f *Flusher) drainAndFlush(ctx context.Context) {
	for {
		batch := f.collect(make([]*model.CostEvent, 0, maxBatch))
		if len(batch) == 0 {
			return
		}
		f.flush(ctx, batch)
	}
}

// flush INSERTs the batch with ON CONFLICT(event_id) DO NOTHING (idempotency
// for spec §5.1.1 business event_id retries).
func (f *Flusher) flush(ctx context.Context, batch []*model.CostEvent) {
	if len(batch) == 0 {
		return
	}
	if f.biz != nil {
		f.biz.CostFlusherBatchSize.Observe(float64(len(batch)))
	}
	err := f.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&batch).Error
	if err != nil && !errors.Is(err, context.Canceled) {
		if f.biz != nil {
			f.biz.CostEventRecordFailedTotal.WithLabelValues("db_write").Inc()
		}
		logger.FromCtx(ctx).Error("cost.flush.failed", "err", err, "batch_size", len(batch))
	}
}
