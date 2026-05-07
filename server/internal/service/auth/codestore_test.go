//go:build integration

package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	rdb "github.com/redis/go-redis/v9"
)

func startRedis(t *testing.T) *rdb.Client {
	t.Helper()
	ctx := context.Background()
	c, err := redis.Run(ctx, "redis:7-alpine",
		tc.WithWaitStrategy(wait.ForListeningPort("6379/tcp").WithStartupTimeout(15*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "6379/tcp")
	return rdb.NewClient(&rdb.Options{Addr: host + ":" + port.Port()})
}

func TestCodeStore_SaveAndTake(t *testing.T) {
	cli := startRedis(t)
	store := NewRedisCodeStore(cli)
	require.NoError(t, store.Save(context.Background(), "h_a", "123456", time.Minute, 100*time.Millisecond))

	code, err := store.Take(context.Background(), "h_a")
	require.NoError(t, err)
	assert.Equal(t, "123456", code)
}

func TestCodeStore_TakeIsOneShot(t *testing.T) {
	cli := startRedis(t)
	store := NewRedisCodeStore(cli)
	require.NoError(t, store.Save(context.Background(), "h_b", "123456", time.Minute, 100*time.Millisecond))

	_, err := store.Take(context.Background(), "h_b")
	require.NoError(t, err)
	_, err = store.Take(context.Background(), "h_b")
	assert.True(t, errors.Is(err, ErrCodeNotFound))
}

func TestCodeStore_Cooldown(t *testing.T) {
	cli := startRedis(t)
	store := NewRedisCodeStore(cli)
	require.NoError(t, store.Save(context.Background(), "h_c", "123456", time.Minute, time.Second))
	err := store.Save(context.Background(), "h_c", "999999", time.Minute, time.Second)
	assert.True(t, errors.Is(err, ErrCooldown))
}
