//go:build integration

package outline_test

import (
	"context"
	"os"
	"testing"

	"github.com/aibao/server/internal/service/outline"
	"github.com/aibao/server/internal/service/outlinecontract"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// integrationRedis connects to the local aibao-redis-dev container.
// Override via AIBAO_TEST_REDIS_ADDR env var.
func integrationRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("AIBAO_TEST_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	return rdb
}

func TestCache_SetGet(t *testing.T) {
	rdb := integrationRedis(t)
	c := outline.NewCache(rdb)
	ctx := context.Background()
	outlineID := "ol_test_setget_001"
	defer rdb.Del(ctx, "outline:"+outlineID)

	co := outline.NewCachedOutline(outlinecontract.Outline{
		OutlineID:   outlineID,
		Title:       "test",
		Style:       "冒险探索",
		Themes:      []string{"勇气"},
		DurationMin: 5,
	}, 42, 7, "原 prompt 文本")
	require.NoError(t, c.Set(ctx, co))

	got, err := c.Get(ctx, outlineID)
	require.NoError(t, err)
	if got.Title != "test" {
		t.Errorf("title mismatch: %s", got.Title)
	}
	if got.UserID != 42 || got.ChildID != 7 {
		t.Errorf("ownership: %d/%d", got.UserID, got.ChildID)
	}
	if got.Style != "冒险探索" {
		t.Errorf("style: %s", got.Style)
	}
	if got.PromptText != "原 prompt 文本" {
		t.Errorf("prompt: %s", got.PromptText)
	}
}

func TestCache_Miss(t *testing.T) {
	rdb := integrationRedis(t)
	c := outline.NewCache(rdb)
	_, err := c.Get(context.Background(), "ol_nonexistent_xyz_999")
	if err != outline.ErrCacheMiss {
		t.Errorf("want ErrCacheMiss, got %v", err)
	}
}

func TestCache_Invalidate(t *testing.T) {
	rdb := integrationRedis(t)
	c := outline.NewCache(rdb)
	ctx := context.Background()
	outlineID := "ol_test_invalidate_001"
	defer rdb.Del(ctx, "outline:"+outlineID)

	co := outline.NewCachedOutline(outlinecontract.Outline{OutlineID: outlineID}, 1, 1, "")
	require.NoError(t, c.Set(ctx, co))
	require.NoError(t, c.Invalidate(ctx, outlineID))

	_, err := c.Get(ctx, outlineID)
	if err != outline.ErrCacheMiss {
		t.Errorf("want miss after invalidate, got %v", err)
	}
}
