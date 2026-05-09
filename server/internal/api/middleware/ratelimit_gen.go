package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/aibao/server/internal/api/userctx"
)

// Counter is the minimal Redis-backed counter surface this middleware needs.
type Counter interface {
	IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// RedisCounter is the production Counter implementation.
type RedisCounter struct {
	c *redis.Client
}

// NewRedisCounter constructs a Counter backed by the given Redis client.
func NewRedisCounter(c *redis.Client) *RedisCounter { return &RedisCounter{c: c} }

// IncrWithTTL atomically increments key and sets TTL to ttl.
func (r *RedisCounter) IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := r.c.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// GenerateRateLimit limits each authenticated user to maxPerWindow requests per window.
func GenerateRateLimit(counter Counter, maxPerWindow int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := userctx.FromContext(c.Request.Context())
		if !ok {
			c.Next()
			return
		}
		key := fmt.Sprintf("rate:gen:%d", uid)
		count, err := counter.IncrWithTTL(c.Request.Context(), key, window)
		if err != nil {
			c.Next()
			return
		}
		if count > int64(maxPerWindow) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"reason":   "rate_limited",
				"user_msg": "请求过于频繁，请稍后再试",
			})
			return
		}
		c.Next()
	}
}
