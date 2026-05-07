package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/aibao/server/internal/pkg/config"
)

// NewRedis opens a Redis client and verifies connectivity with a ping.
// Timeouts are conservative for a local 127.0.0.1 deployment.
func NewRedis(cfg config.RedisConfig) (*redis.Client, error) {
	c := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return c, nil
}

// PingRedis verifies the Redis server is reachable. Used by /ready.
func PingRedis(ctx context.Context, c *redis.Client) error {
	return c.Ping(ctx).Err()
}
