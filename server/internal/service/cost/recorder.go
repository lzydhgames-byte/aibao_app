// Package cost implements the Plan 11B cost observability service layer:
// Recorder (sync Prometheus + async PG queue) + Flusher (batch INSERT).
// Calculation is delegated to pkg/cost (pure Calc + PriceBook); recorder
// composes business context (user/child/story/outline) around it.
package cost

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"time"

	"github.com/aibao/server/internal/metrics"
	"github.com/aibao/server/internal/model"
	pkgcost "github.com/aibao/server/internal/pkg/cost"
	"github.com/aibao/server/internal/pkg/logger"
)

const queueCapacity = 10_000

// eventIDRe enforces business-semantic event_id format from spec §5.1.1:
// {trace_id 8+hex}:{purpose snake_case}:{stage snake_case}:{attempt digits}
var eventIDRe = regexp.MustCompile(`^[a-f0-9]{8,}:[a-z_]+:[a-z_]+:\d+$`)

// ErrBadEventID is returned when caller supplies a malformed event_id.
// Business path must NOT break on this; caller logs + ignores.
var ErrBadEventID = errors.New("recorder: bad event_id format")

// RecordInput is what callers pass. Business code constructs this; Recorder
// does NOT auto-generate event_id (spec §5.1.1 — keeps the idempotency key
// reproducible across process restarts).
type RecordInput struct {
	EventID              string
	UserID               *int64
	ChildIDHash          string
	Purpose              string // outline|story|tts|chapter_hook|memory_summary|storage_put
	Provider             string
	Model                string
	BillingMode          string // empty → "standard"
	Usage                pkgcost.Usage
	Outcome              string // ok|fallback|fail
	DurationMs           int
	StoryID              *int64
	OutlineID            string
	OutlineGroupID       string
	OutlinePromptVersion string
	TraceID              string
}

// Recorder validates input, calculates cost via pkg/cost, increments
// Prometheus counters, and enqueues to the Flusher. It is goroutine-safe.
// Business path must NEVER break on cost recording (spec §3.3).
type Recorder struct {
	pb     pkgcost.PriceBook
	biz    *metrics.Business
	queue  chan *model.CostEvent
	once   sync.Once
	closed chan struct{}
}

// NewRecorder constructs a Recorder. biz may be nil for tests; nil-safe paths
// silently skip Prometheus updates.
func NewRecorder(pb pkgcost.PriceBook, biz *metrics.Business) *Recorder {
	return &Recorder{
		pb:     pb,
		biz:    biz,
		queue:  make(chan *model.CostEvent, queueCapacity),
		closed: make(chan struct{}),
	}
}

// Record is the only public entry point. It returns ErrBadEventID for
// malformed input; otherwise returns nil even on Prometheus / queue failures
// (so business path is never broken by cost recording).
func (r *Recorder) Record(ctx context.Context, in RecordInput) error {
	if !eventIDRe.MatchString(in.EventID) {
		r.bumpFailed("bad_event_id")
		logger.FromCtx(ctx).Warn("cost.record.bad_event_id", "event_id", in.EventID)
		return ErrBadEventID
	}
	billing := in.BillingMode
	if billing == "" {
		billing = "standard"
	}
	key := pkgcost.PriceBookKey{Provider: in.Provider, Model: in.Model, BillingMode: billing}
	entry, err := r.pb.Lookup(key)
	if err != nil {
		r.bumpFailed("price_miss")
		logger.FromCtx(ctx).Warn("cost.record.price_miss", "provider", key.Provider, "model", key.Model)
		return nil // business continues
	}
	yuan := pkgcost.Calc(entry, in.Usage)

	// Snapshot price entry for audit (spec §5.1 unit_price_snapshot).
	snap := model.PriceSnapshot{
		"unit":             entry.Unit,
		"input":            entry.InputPrice,
		"output":           entry.OutputPrice,
		"chars":            entry.CharsPrice,
		"put_per_10k":      entry.PutPer10kRequests,
		"bandwidth_per_gb": entry.BandwidthYuanPerGB,
	}

	// Prometheus (sync, lock-free).
	if r.biz != nil {
		r.biz.CostYuanTotal.WithLabelValues(in.Provider, in.Model, in.Purpose, in.Outcome).Add(yuan)
	}

	evt := &model.CostEvent{
		EventID:              in.EventID,
		OccurredAt:           time.Now(),
		UserID:               in.UserID,
		ChildIDHash:          in.ChildIDHash,
		Purpose:              in.Purpose,
		Provider:             in.Provider,
		Model:                in.Model,
		BillingMode:          billing,
		TokensIn:             in.Usage.TokensIn,
		TokensOut:            in.Usage.TokensOut,
		TokensCached:         in.Usage.TokensCached,
		Chars:                in.Usage.Chars,
		Bytes:                in.Usage.Bytes,
		AudioSeconds:         in.Usage.AudioSeconds,
		CostYuan:             yuan,
		Currency:             "CNY",
		PriceVersion:         r.pb.Version(),
		UnitPriceSnapshot:    snap,
		Outcome:              in.Outcome,
		DurationMs:           in.DurationMs,
		StoryID:              in.StoryID,
		OutlineID:            in.OutlineID,
		OutlineGroupID:       in.OutlineGroupID,
		OutlinePromptVersion: in.OutlinePromptVersion,
		TraceID:              in.TraceID,
	}

	// Async enqueue (non-blocking). Queue full → drop + metric (spec §3.3).
	select {
	case r.queue <- evt:
	default:
		r.bumpFailed("queue_full")
		logger.FromCtx(ctx).Warn("cost.record.queue_full")
	}
	return nil
}

func (r *Recorder) bumpFailed(reason string) {
	if r.biz != nil {
		r.biz.CostEventRecordFailedTotal.WithLabelValues(reason).Inc()
	}
}

// Drain returns the underlying queue for the Flusher to consume.
// Only the Flusher should read from this channel.
func (r *Recorder) Drain() <-chan *model.CostEvent { return r.queue }

// Close signals shutdown. Idempotent; safe to call multiple times.
func (r *Recorder) Close() {
	r.once.Do(func() {
		close(r.closed)
		close(r.queue)
	})
}

// Closed returns a channel that is closed when Close has been called.
func (r *Recorder) Closed() <-chan struct{} { return r.closed }
