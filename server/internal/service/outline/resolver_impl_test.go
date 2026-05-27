//go:build integration

package outline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/service/outline"
	"github.com/aibao/server/internal/service/outlinecontract"
	"github.com/stretchr/testify/require"
)

func setupResolver(t *testing.T) (*outline.ResolverImpl, *outline.Cache, *outline.EventStore) {
	t.Helper()
	db := integrationDB(t)
	rdb := integrationRedis(t)
	cache := outline.NewCache(rdb)
	events := outline.NewEventStore(db)
	return outline.NewResolver(cache, events), cache, events
}

func TestResolver_OK(t *testing.T) {
	r, cache, events := setupResolver(t)
	ctx := context.Background()
	outlineID := "ol_resolver_ok_001"
	defer cache.Invalidate(ctx, outlineID)
	defer integrationDB(t).Exec("DELETE FROM outline_events WHERE outline_id = ?", outlineID)

	co := outline.NewCachedOutline(outlinecontract.Outline{
		OutlineID: outlineID, Title: "t", Style: "冒险探索",
	}, 42, 7, "p")
	require.NoError(t, cache.Set(ctx, co))
	require.NoError(t, events.Append(ctx, model.OutlineEvent{
		OutlineID: outlineID, OutlineGroupID: "g", UserID: 42, ChildIDHash: "h",
		Outcome: outline.OutcomePending,
	}))

	out, err := r.Resolve(ctx, outlineID, 42, 7)
	require.NoError(t, err)
	if out.Title != "t" {
		t.Errorf("title: %s", out.Title)
	}
}

func TestResolver_OwnershipMismatch_User(t *testing.T) {
	r, cache, events := setupResolver(t)
	ctx := context.Background()
	outlineID := "ol_resolver_user_mismatch"
	defer cache.Invalidate(ctx, outlineID)
	defer integrationDB(t).Exec("DELETE FROM outline_events WHERE outline_id = ?", outlineID)

	require.NoError(t, cache.Set(ctx, outline.NewCachedOutline(
		outlinecontract.Outline{OutlineID: outlineID}, 42, 7, "")))
	require.NoError(t, events.Append(ctx, model.OutlineEvent{
		OutlineID: outlineID, OutlineGroupID: "g", UserID: 42, ChildIDHash: "h",
		Outcome: outline.OutcomePending,
	}))

	_, err := r.Resolve(ctx, outlineID, 99, 7) // wrong user
	if !errors.Is(err, outlinecontract.ErrOutlineForbidden) {
		t.Fatalf("want Forbidden, got %v", err)
	}
}

func TestResolver_OwnershipMismatch_Child(t *testing.T) {
	r, cache, events := setupResolver(t)
	ctx := context.Background()
	outlineID := "ol_resolver_child_mismatch"
	defer cache.Invalidate(ctx, outlineID)
	defer integrationDB(t).Exec("DELETE FROM outline_events WHERE outline_id = ?", outlineID)

	require.NoError(t, cache.Set(ctx, outline.NewCachedOutline(
		outlinecontract.Outline{OutlineID: outlineID}, 42, 7, "")))
	require.NoError(t, events.Append(ctx, model.OutlineEvent{
		OutlineID: outlineID, OutlineGroupID: "g", UserID: 42, ChildIDHash: "h",
		Outcome: outline.OutcomePending,
	}))

	_, err := r.Resolve(ctx, outlineID, 42, 99) // wrong child
	if !errors.Is(err, outlinecontract.ErrOutlineForbidden) {
		t.Fatalf("want Forbidden, got %v", err)
	}
}

func TestResolver_AlreadyAccepted_Replay(t *testing.T) {
	r, cache, events := setupResolver(t)
	ctx := context.Background()
	outlineID := "ol_resolver_replay"
	defer cache.Invalidate(ctx, outlineID)
	defer integrationDB(t).Exec("DELETE FROM outline_events WHERE outline_id = ?", outlineID)

	require.NoError(t, cache.Set(ctx, outline.NewCachedOutline(
		outlinecontract.Outline{OutlineID: outlineID}, 42, 7, "")))
	// Already accepted — cannot be resolved again.
	require.NoError(t, events.Append(ctx, model.OutlineEvent{
		OutlineID: outlineID, OutlineGroupID: "g", UserID: 42, ChildIDHash: "h",
		Outcome: outline.OutcomeAccepted,
	}))

	_, err := r.Resolve(ctx, outlineID, 42, 7)
	if !errors.Is(err, outlinecontract.ErrOutlineExpired) {
		t.Fatalf("want Expired (replay defense), got %v", err)
	}
}

func TestResolver_AlreadyRefreshed_Replay(t *testing.T) {
	r, cache, events := setupResolver(t)
	ctx := context.Background()
	outlineID := "ol_resolver_refreshed"
	defer cache.Invalidate(ctx, outlineID)
	defer integrationDB(t).Exec("DELETE FROM outline_events WHERE outline_id = ?", outlineID)

	require.NoError(t, cache.Set(ctx, outline.NewCachedOutline(
		outlinecontract.Outline{OutlineID: outlineID}, 42, 7, "")))
	require.NoError(t, events.Append(ctx, model.OutlineEvent{
		OutlineID: outlineID, OutlineGroupID: "g", UserID: 42, ChildIDHash: "h",
		Outcome: outline.OutcomeRefreshed,
	}))

	_, err := r.Resolve(ctx, outlineID, 42, 7)
	if !errors.Is(err, outlinecontract.ErrOutlineExpired) {
		t.Fatalf("want Expired (refreshed replay), got %v", err)
	}
}

func TestResolver_CacheMiss(t *testing.T) {
	r, _, _ := setupResolver(t)
	_, err := r.Resolve(context.Background(), "ol_resolver_nonexistent_zzz", 1, 1)
	if !errors.Is(err, outlinecontract.ErrOutlineExpired) {
		t.Fatalf("want Expired (cache miss), got %v", err)
	}
}
