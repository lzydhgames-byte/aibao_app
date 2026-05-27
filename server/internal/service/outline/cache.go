package outline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aibao/server/internal/service/outlinecontract"
	"github.com/redis/go-redis/v9"
)

// cacheTTL is the 5-minute spec §5.2 TTL for outline tickets.
const cacheTTL = 5 * time.Minute

// ErrCacheMiss is returned when the outline_id is absent or TTL expired.
// service/outline/resolver_impl maps this to outlinecontract.ErrOutlineExpired.
var ErrCacheMiss = errors.New("outline cache: miss")

// CachedOutline is what we persist in Redis — the contract DTO plus
// ownership + prompt text + creation timestamp, used by ResolverImpl for
// triple ownership enforcement and by Service.Preview to seed refresh.
// outline_id NEVER appears in logs / metric labels (spec §5.2).
type CachedOutline struct {
	outlinecontract.Outline
	UserID     int64     `json:"user_id"`
	ChildID    int64     `json:"child_id"`
	PromptText string    `json:"prompt_text"`
	CreatedAt  time.Time `json:"created_at"`
}

// NewCachedOutline is the public constructor used by Service.Preview and tests.
// CreatedAt is set to time.Now() at construction.
func NewCachedOutline(o outlinecontract.Outline, userID, childID int64, prompt string) CachedOutline {
	return CachedOutline{
		Outline:    o,
		UserID:     userID,
		ChildID:    childID,
		PromptText: prompt,
		CreatedAt:  time.Now(),
	}
}

// Cache is a thin wrapper around go-redis providing the spec §5.2 contract:
// Set with TTL, Get returns ErrCacheMiss on absent, Invalidate for refresh.
type Cache struct {
	rdb *redis.Client
}

func NewCache(rdb *redis.Client) *Cache { return &Cache{rdb: rdb} }

func cacheKey(outlineID string) string { return "outline:" + outlineID }

// Set stores the outline with 5min TTL.
func (c *Cache) Set(ctx context.Context, co CachedOutline) error {
	b, err := json.Marshal(co)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return c.rdb.Set(ctx, cacheKey(co.OutlineID), b, cacheTTL).Err()
}

// Get retrieves the outline. Returns ErrCacheMiss if not found / expired.
func (c *Cache) Get(ctx context.Context, outlineID string) (*CachedOutline, error) {
	raw, err := c.rdb.Get(ctx, cacheKey(outlineID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var co CachedOutline
	if err := json.Unmarshal(raw, &co); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &co, nil
}

// Invalidate deletes the outline immediately (for refresh path).
func (c *Cache) Invalidate(ctx context.Context, outlineID string) error {
	return c.rdb.Del(ctx, cacheKey(outlineID)).Err()
}
