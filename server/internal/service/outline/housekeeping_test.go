//go:build integration

package outline_test

import (
	"context"
	"testing"
	"time"

	"github.com/aibao/server/internal/service/outline"
	"github.com/stretchr/testify/require"
)

func TestHousekeeper_SweepUser_OnlyOlderThanGrace(t *testing.T) {
	db := integrationDB(t)
	cleanupOutlineEvents(t, db, "ol_hk_old", "ol_hk_fresh", "ol_hk_other_user")
	defer cleanupOutlineEvents(t, db, "ol_hk_old", "ol_hk_fresh", "ol_hk_other_user")

	es := outline.NewEventStore(db)
	hk := outline.NewHousekeeper(es, nil)
	ctx := context.Background()

	old := time.Now().Add(-6 * time.Minute)    // > userSweepGrace (5m30s)
	fresh := time.Now().Add(-1 * time.Minute) // < userSweepGrace
	db.Exec(`INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome) VALUES (?, ?, ?, ?, ?, 'pending')`,
		old, "ol_hk_old", "g1", int64(1), "h")
	db.Exec(`INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome) VALUES (?, ?, ?, ?, ?, 'pending')`,
		fresh, "ol_hk_fresh", "g2", int64(1), "h")
	db.Exec(`INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome) VALUES (?, ?, ?, ?, ?, 'pending')`,
		old, "ol_hk_other_user", "g3", int64(2), "h")

	hk.SweepUser(ctx, 1)

	oldOutcome, err := es.LatestOutcome(ctx, "ol_hk_old")
	require.NoError(t, err)
	freshOutcome, err := es.LatestOutcome(ctx, "ol_hk_fresh")
	require.NoError(t, err)
	otherOutcome, err := es.LatestOutcome(ctx, "ol_hk_other_user")
	require.NoError(t, err)

	if oldOutcome != "expired" {
		t.Errorf("ol_hk_old should be expired, got %s", oldOutcome)
	}
	if freshOutcome != "pending" {
		t.Errorf("ol_hk_fresh should still be pending, got %s", freshOutcome)
	}
	if otherOutcome != "pending" {
		t.Errorf("ol_hk_other_user belongs to user 2, should not be touched, got %s", otherOutcome)
	}
}

func TestHousekeeper_RunOnce_IncludesAllUsers(t *testing.T) {
	db := integrationDB(t)
	cleanupOutlineEvents(t, db, "ol_hk_all_u1", "ol_hk_all_u2")
	defer cleanupOutlineEvents(t, db, "ol_hk_all_u1", "ol_hk_all_u2")

	es := outline.NewEventStore(db)
	hk := outline.NewHousekeeper(es, nil)
	ctx := context.Background()

	old := time.Now().Add(-15 * time.Minute) // > pendingThreshold (10m)
	db.Exec(`INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome) VALUES (?, ?, ?, ?, ?, 'pending')`,
		old, "ol_hk_all_u1", "g1", int64(1), "h")
	db.Exec(`INSERT INTO outline_events (occurred_at, outline_id, outline_group_id, user_id, child_id_hash, outcome) VALUES (?, ?, ?, ?, ?, 'pending')`,
		old, "ol_hk_all_u2", "g2", int64(2), "h")

	require.NoError(t, hk.RunOnce(ctx))

	u1, _ := es.LatestOutcome(ctx, "ol_hk_all_u1")
	u2, _ := es.LatestOutcome(ctx, "ol_hk_all_u2")
	if u1 != "expired" || u2 != "expired" {
		t.Errorf("both users' orphan outlines should be expired; got u1=%s u2=%s", u1, u2)
	}
}
