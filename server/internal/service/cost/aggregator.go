package cost

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Aggregator runs read-only PG queries to summarize cost_events + outline_events.
// All methods accept [since, until) time bounds. Returns zero values on empty range.
// Plan 11B §8.2 Full Build entry point — backs cmd/cost-report (Task 30).
type Aggregator struct {
	db *gorm.DB
}

func NewAggregator(db *gorm.DB) *Aggregator { return &Aggregator{db: db} }

// OverallStats — top-level summary numbers for the period.
type OverallStats struct {
	TotalYuan         float64
	StoriesAccepted   int64
	OutlinesPreviewed int64
	OutlinesAccepted  int64
	OutlinesRefreshed int64
	OutlinesExpired   int64
}

// Overall computes top-level counts + total yuan in [since, until).
func (a *Aggregator) Overall(ctx context.Context, since, until time.Time) (OverallStats, error) {
	var s OverallStats
	if err := a.db.WithContext(ctx).Raw(
		`SELECT COALESCE(SUM(cost_yuan), 0) FROM cost_events WHERE occurred_at >= ? AND occurred_at < ?`,
		since, until,
	).Scan(&s.TotalYuan).Error; err != nil {
		return s, err
	}
	if err := a.db.WithContext(ctx).Raw(
		`SELECT COUNT(DISTINCT story_id) FROM cost_events
         WHERE purpose='story' AND outcome='ok' AND story_id IS NOT NULL
           AND occurred_at >= ? AND occurred_at < ?`,
		since, until,
	).Scan(&s.StoriesAccepted).Error; err != nil {
		return s, err
	}
	if err := a.db.WithContext(ctx).Raw(
		`SELECT COUNT(DISTINCT outline_id) FROM outline_events
         WHERE occurred_at >= ? AND occurred_at < ?`,
		since, until,
	).Scan(&s.OutlinesPreviewed).Error; err != nil {
		return s, err
	}

	// outline outcome counts — count rows, since append-only model means each
	// outcome write is one row (an outline that's pending then accepted has 2 rows).
	for _, oc := range []struct {
		outcome string
		dst     *int64
	}{
		{"accepted", &s.OutlinesAccepted},
		{"refreshed", &s.OutlinesRefreshed},
		{"expired", &s.OutlinesExpired},
	} {
		if err := a.db.WithContext(ctx).Raw(
			`SELECT COUNT(*) FROM outline_events
             WHERE outcome = ? AND occurred_at >= ? AND occurred_at < ?`,
			oc.outcome, since, until,
		).Scan(oc.dst).Error; err != nil {
			return s, err
		}
	}
	return s, nil
}

// PurposeRow is one row in ByPurpose result.
type PurposeRow struct {
	Purpose  string  `gorm:"column:purpose"`
	CostYuan float64 `gorm:"column:cost_yuan"`
}

// ByPurpose groups cost in [since, until) by purpose, ordered by cost desc.
func (a *Aggregator) ByPurpose(ctx context.Context, since, until time.Time) ([]PurposeRow, error) {
	var rows []PurposeRow
	err := a.db.WithContext(ctx).Raw(`
SELECT purpose, SUM(cost_yuan) AS cost_yuan
FROM cost_events
WHERE occurred_at >= ? AND occurred_at < ?
GROUP BY purpose
ORDER BY cost_yuan DESC`,
		since, until,
	).Scan(&rows).Error
	return rows, err
}

// UserRow is one row in TopUsers result.
// UserIDHash is the HMAC-hashed id (spec §6.3) — never reverse-mappable to plaintext.
type UserRow struct {
	UserIDHash string  `gorm:"column:user_id_hash"`
	Stories    int64   `gorm:"column:stories"`
	Outlines   int64   `gorm:"column:outlines"`
	TotalYuan  float64 `gorm:"column:total_yuan"`
}

// TopUsers returns the top-spending users in [since, until), ordered by cost desc.
// Uses child_id_hash as the bucket key (one user typically has one child;
// multi-child users get separate rows — acceptable for cost ranking).
func (a *Aggregator) TopUsers(ctx context.Context, since, until time.Time, limit int) ([]UserRow, error) {
	var rows []UserRow
	err := a.db.WithContext(ctx).Raw(`
SELECT
    child_id_hash AS user_id_hash,
    COUNT(*) FILTER (WHERE purpose = 'story') AS stories,
    COUNT(DISTINCT outline_id) FILTER (WHERE outline_id != '') AS outlines,
    SUM(cost_yuan) AS total_yuan
FROM cost_events
WHERE occurred_at >= ? AND occurred_at < ?
GROUP BY child_id_hash
ORDER BY total_yuan DESC
LIMIT ?`,
		since, until, limit,
	).Scan(&rows).Error
	return rows, err
}

// OutlineSaving computes the spec §3.4 "rejected outline saved cost" formula:
//
//   saved = N × avg_full_pipeline - actual_outline_spend
//
//   N                  = count of outline_id whose latest outline_events
//                        terminal outcome is refreshed OR expired
//   avg_full_pipeline  = avg over accepted stories of SUM(cost_yuan) per story_id
//                        (includes story + tts + storage_put + chapter_hook + memory_summary)
//   actual_outline_spend = SUM(cost_yuan) for cost_events tagged with one of those rejected outline_ids
//
// Returns 0 (not an error) on empty range — the formula naturally degenerates.
func (a *Aggregator) OutlineSaving(ctx context.Context, since, until time.Time) (float64, error) {
	// Part 1: avg full pipeline cost per accepted story.
	var avgFullPipeline float64
	if err := a.db.WithContext(ctx).Raw(`
WITH accepted_stories AS (
    SELECT DISTINCT story_id FROM cost_events
    WHERE purpose = 'story' AND outcome = 'ok' AND story_id IS NOT NULL
      AND occurred_at >= ? AND occurred_at < ?
),
pipeline AS (
    SELECT story_id, SUM(cost_yuan) AS pipeline_cost
    FROM cost_events
    WHERE story_id IN (SELECT story_id FROM accepted_stories)
    GROUP BY story_id
)
SELECT COALESCE(AVG(pipeline_cost), 0) FROM pipeline`,
		since, until,
	).Scan(&avgFullPipeline).Error; err != nil {
		return 0, err
	}

	// Part 2: rejected outlines + their spend.
	// Uses DISTINCT ON for "latest outcome per outline_id" (spec §5.5
	// append-only — pick the most recent row per outline).
	type rejectedAgg struct {
		Count       int64
		ActualSpent float64
	}
	var r rejectedAgg
	if err := a.db.WithContext(ctx).Raw(`
WITH latest AS (
    SELECT DISTINCT ON (outline_id) outline_id, outcome
    FROM outline_events
    WHERE occurred_at >= ? AND occurred_at < ?
    ORDER BY outline_id, occurred_at DESC, id DESC
),
rejected_outlines AS (
    SELECT outline_id FROM latest WHERE outcome IN ('refreshed', 'expired')
)
SELECT
    COUNT(DISTINCT outline_id) AS count,
    COALESCE(SUM(cost_yuan), 0) AS actual_spent
FROM cost_events
WHERE outline_id IN (SELECT outline_id FROM rejected_outlines)`,
		since, until,
	).Scan(&r).Error; err != nil {
		return 0, err
	}

	return float64(r.Count)*avgFullPipeline - r.ActualSpent, nil
}
