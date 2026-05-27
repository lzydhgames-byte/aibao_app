//go:build integration

package outline_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/service/outline"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AIBAO_TEST_DB_DSN")
	if dsn == "" {
		dsn = "postgres://aibao:aibao@127.0.0.1:5432/aibao?sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func cleanupOutlineEvents(t *testing.T, db *gorm.DB, outlineIDs ...string) {
	t.Helper()
	for _, id := range outlineIDs {
		db.Exec("DELETE FROM outline_events WHERE outline_id = ?", id)
	}
}

func TestEventStore_AppendAndLatest(t *testing.T) {
	db := integrationDB(t)
	cleanupOutlineEvents(t, db, "ol_test_append_001")
	defer cleanupOutlineEvents(t, db, "ol_test_append_001")

	s := outline.NewEventStore(db)
	ctx := context.Background()

	require.NoError(t, s.Append(ctx, model.OutlineEvent{
		OutlineID: "ol_test_append_001", OutlineGroupID: "g_test", UserID: 1,
		ChildIDHash: "h", Outcome: outline.OutcomePending,
	}))
	require.NoError(t, s.Append(ctx, model.OutlineEvent{
		OutlineID: "ol_test_append_001", OutlineGroupID: "g_test", UserID: 1,
		ChildIDHash: "h", Outcome: outline.OutcomeAccepted,
	}))

	got, err := s.LatestOutcome(ctx, "ol_test_append_001")
	require.NoError(t, err)
	if got != outline.OutcomeAccepted {
		t.Fatalf("want accepted, got %s", got)
	}
}

func TestEventStore_MarkExpiredIfPending_IdempotentWhenPending(t *testing.T) {
	db := integrationDB(t)
	cleanupOutlineEvents(t, db, "ol_test_expire_001")
	defer cleanupOutlineEvents(t, db, "ol_test_expire_001")

	s := outline.NewEventStore(db)
	ctx := context.Background()

	require.NoError(t, s.Append(ctx, model.OutlineEvent{
		OutlineID: "ol_test_expire_001", OutlineGroupID: "g_test", UserID: 1,
		ChildIDHash: "h", Outcome: outline.OutcomePending,
	}))
	require.NoError(t, s.MarkExpiredIfPending(ctx, model.OutlineEvent{
		OutlineID: "ol_test_expire_001", OutlineGroupID: "g_test", UserID: 1, ChildIDHash: "h",
	}))
	require.NoError(t, s.MarkExpiredIfPending(ctx, model.OutlineEvent{
		OutlineID: "ol_test_expire_001", OutlineGroupID: "g_test", UserID: 1, ChildIDHash: "h",
	}))

	var cnt int64
	db.Model(&model.OutlineEvent{}).Where("outline_id = ? AND outcome = 'expired'", "ol_test_expire_001").Count(&cnt)
	if cnt != 1 {
		t.Fatalf("expected 1 expired row (idempotent), got %d", cnt)
	}
}

func TestEventStore_MarkExpiredIfPending_NoopWhenAccepted(t *testing.T) {
	db := integrationDB(t)
	cleanupOutlineEvents(t, db, "ol_test_expire_noop")
	defer cleanupOutlineEvents(t, db, "ol_test_expire_noop")

	s := outline.NewEventStore(db)
	ctx := context.Background()

	require.NoError(t, s.Append(ctx, model.OutlineEvent{
		OutlineID: "ol_test_expire_noop", OutlineGroupID: "g_test", UserID: 1,
		ChildIDHash: "h", Outcome: outline.OutcomeAccepted,
	}))
	require.NoError(t, s.MarkExpiredIfPending(ctx, model.OutlineEvent{
		OutlineID: "ol_test_expire_noop", OutlineGroupID: "g_test", UserID: 1, ChildIDHash: "h",
	}))

	var cnt int64
	db.Model(&model.OutlineEvent{}).Where("outline_id = ? AND outcome = 'expired'", "ol_test_expire_noop").Count(&cnt)
	if cnt != 0 {
		t.Fatalf("expected NO expired row when accepted exists, got %d", cnt)
	}
}

func TestEventStore_ScanPendingOlderThan_UserFilter(t *testing.T) {
	db := integrationDB(t)
	cleanupOutlineEvents(t, db, "ol_test_scan_old", "ol_test_scan_fresh", "ol_test_scan_other_user")
	defer cleanupOutlineEvents(t, db, "ol_test_scan_old", "ol_test_scan_fresh", "ol_test_scan_other_user")

	s := outline.NewEventStore(db)
	ctx := context.Background()
	old := time.Now().Add(-10 * time.Minute)
	fresh := time.Now().Add(-1 * time.Minute)

	db.Exec(`INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome) VALUES (?, ?, ?, ?, ?, 'pending')`, old, "ol_test_scan_old", "g1", 1, "h")
	db.Exec(`INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome) VALUES (?, ?, ?, ?, ?, 'pending')`, fresh, "ol_test_scan_fresh", "g2", 1, "h")
	db.Exec(`INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome) VALUES (?, ?, ?, ?, ?, 'pending')`, old, "ol_test_scan_other_user", "g3", 2, "h")

	threshold := time.Now().Add(-5 * time.Minute)
	uid := int64(1)
	rows, err := s.ScanPendingOlderThan(ctx, threshold, &uid, 100)
	require.NoError(t, err)

	found := map[string]bool{}
	for _, r := range rows {
		found[r.OutlineID] = true
	}
	if !found["ol_test_scan_old"] {
		t.Errorf("expected ol_test_scan_old in scan, got %v", found)
	}
	if found["ol_test_scan_fresh"] {
		t.Errorf("ol_test_scan_fresh should be too fresh, scan returned it")
	}
	if found["ol_test_scan_other_user"] {
		t.Errorf("ol_test_scan_other_user belongs to user 2, user filter failed")
	}
}
