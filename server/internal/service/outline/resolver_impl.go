package outline

import (
	"context"
	"errors"

	"github.com/aibao/server/internal/service/outlinecontract"
)

// ResolverImpl is the production implementation of outlinecontract.OutlineResolver.
// It enforces three layers of validation (spec §5.2 + §7.5):
//   1. Cache existence (TTL not elapsed, key not invalidated)
//   2. Triple ownership: user_id + child_id + outline_id must match cache payload
//   3. Replay defense: latest outline_events outcome must still be Pending
//
// Production wiring in main.go injects this into service/story orchestrator
// via the outlinecontract.OutlineResolver interface so story/ never imports outline/.
type ResolverImpl struct {
	cache  *Cache
	events *EventStore
}

// NewResolver constructs a ResolverImpl. Both cache and events are required.
func NewResolver(cache *Cache, events *EventStore) *ResolverImpl {
	return &ResolverImpl{cache: cache, events: events}
}

// Resolve enforces user_id + child_id + outline_id triple ownership and
// rejects replay attempts on already-terminal outlines.
//
// Error mapping (caller maps to HTTP):
//   - outlinecontract.ErrOutlineExpired  → 410 outline_expired (cache miss / TTL / terminal state)
//   - outlinecontract.ErrOutlineForbidden → 403 forbidden (ownership mismatch)
//   - other error → 500 (DB/Redis infra failure)
func (r *ResolverImpl) Resolve(ctx context.Context, outlineID string, userID, childID int64) (*outlinecontract.Outline, error) {
	co, err := r.cache.Get(ctx, outlineID)
	if errors.Is(err, ErrCacheMiss) {
		return nil, outlinecontract.ErrOutlineExpired
	}
	if err != nil {
		return nil, err
	}
	if co.UserID != userID || co.ChildID != childID {
		return nil, outlinecontract.ErrOutlineForbidden
	}
	latest, err := r.events.LatestOutcome(ctx, outlineID)
	if err != nil {
		return nil, err
	}
	if latest != OutcomePending {
		// Replay defense — already accepted / refreshed / expired.
		// Returning ErrOutlineExpired keeps the client UX uniform:
		// "outline 已失效，请重新预览" regardless of terminal cause.
		return nil, outlinecontract.ErrOutlineExpired
	}
	// Defensive copy: return the embedded contract DTO by value so callers
	// can't mutate the cache's internal state (Outline contains slices).
	out := co.Outline
	return &out, nil
}

// Compile-time assertion that ResolverImpl satisfies the contract interface.
var _ outlinecontract.OutlineResolver = (*ResolverImpl)(nil)
