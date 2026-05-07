//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/aibao/server/internal/pkg/config"
)

func startRedis(t *testing.T) (*redis.RedisContainer, config.RedisConfig) {
	t.Helper()
	ctx := context.Background()
	c, err := redis.Run(ctx, "redis:7-alpine",
		tc.WithWaitStrategy(
			wait.ForListeningPort("6379/tcp").
				WithStartupTimeout(15*time.Second),
		),
	)
	require.NoError(t, err)
	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "6379/tcp")
	return c, config.RedisConfig{Addr: host + ":" + port.Port()}
}

func TestNewRedis_PingPong(t *testing.T) {
	c, cfg := startRedis(t)
	defer func() { _ = c.Terminate(context.Background()) }()

	r, err := NewRedis(cfg)
	require.NoError(t, err)
	defer r.Close()

	assert.NoError(t, PingRedis(context.Background(), r))
}

func TestNewRedis_BadAddr(t *testing.T) {
	_, err := NewRedis(config.RedisConfig{Addr: "127.0.0.1:1"})
	assert.Error(t, err)
}
