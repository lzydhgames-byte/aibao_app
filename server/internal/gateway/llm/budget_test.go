//go:build integration

package llm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/redis/go-redis/v9"
)

func startRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()
	c, err := tcredis.Run(ctx, "redis:7-alpine",
		tc.WithWaitStrategy(wait.ForListeningPort("6379/tcp").WithStartupTimeout(15*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "6379/tcp")
	return redis.NewClient(&redis.Options{Addr: host + ":" + port.Port()})
}

func TestBudget_AllowWhenUnderLimit(t *testing.T) {
	cli := startRedis(t)
	bg := NewBudgetGate(cli, BudgetConfig{DailyLimitYuan: 100, PriceInputPerMTok: 0.8, PriceOutputPerMTok: 2.0})

	require.NoError(t, bg.PreCheck(context.Background()))

	require.NoError(t, bg.Record(context.Background(), 1000, 500))

	used, err := bg.UsedYuan(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 0.0008+0.001, used, 0.0001)
}

func TestBudget_BlockWhenOverLimit(t *testing.T) {
	cli := startRedis(t)
	bg := NewBudgetGate(cli, BudgetConfig{DailyLimitYuan: 0.001, PriceInputPerMTok: 0.8, PriceOutputPerMTok: 2.0})

	// burn the budget
	require.NoError(t, bg.Record(context.Background(), 1000, 1000))

	err := bg.PreCheck(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBudgetExceeded)
}

func TestBudget_DateKeyResets(t *testing.T) {
	cli := startRedis(t)
	bg := NewBudgetGate(cli, BudgetConfig{DailyLimitYuan: 100, PriceInputPerMTok: 0.8, PriceOutputPerMTok: 2.0})
	require.NoError(t, bg.Record(context.Background(), 1000, 500))

	// fake "tomorrow" by overriding now func
	bg.nowFn = func() time.Time { return time.Now().Add(25 * time.Hour) }
	used, err := bg.UsedYuan(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 0.0, used, 0.0001)
}
