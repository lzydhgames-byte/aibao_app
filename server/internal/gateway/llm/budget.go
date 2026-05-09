// Package llm contains the LLM gateway abstraction (interface, doubao impl,
// mock) plus a budget gate that bounds daily token spending.
package llm

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrBudgetExceeded is returned by BudgetGate.PreCheck when today's spend
// has already crossed the configured daily limit.
var ErrBudgetExceeded = errors.New("daily llm budget exceeded")

// BudgetConfig holds the rate / price parameters for the budget gate.
type BudgetConfig struct {
	DailyLimitYuan     float64 // e.g. 100.0
	PriceInputPerMTok  float64 // e.g. 0.8 yuan / 1M input tokens
	PriceOutputPerMTok float64 // e.g. 2.0 yuan / 1M output tokens
}

// BudgetGate accumulates daily LLM cost in Redis (per-day key with 25h TTL)
// and refuses new calls once the daily limit is crossed.
type BudgetGate struct {
	c     *redis.Client
	cfg   BudgetConfig
	nowFn func() time.Time
}

// NewBudgetGate constructs a BudgetGate.
func NewBudgetGate(c *redis.Client, cfg BudgetConfig) *BudgetGate {
	return &BudgetGate{c: c, cfg: cfg, nowFn: time.Now}
}

// dayKey returns the Redis key for the current local day, e.g. "budget:llm:daily:20260508".
func (b *BudgetGate) dayKey() string {
	return "budget:llm:daily:" + b.nowFn().Format("20060102")
}

// PreCheck refuses with ErrBudgetExceeded if today's spend ≥ limit.
func (b *BudgetGate) PreCheck(ctx context.Context) error {
	used, err := b.UsedYuan(ctx)
	if err != nil {
		return fmt.Errorf("budget read: %w", err)
	}
	if used >= b.cfg.DailyLimitYuan {
		return ErrBudgetExceeded
	}
	return nil
}

// Record adds the cost of (inputTokens, outputTokens) to today's bucket.
func (b *BudgetGate) Record(ctx context.Context, inputTokens, outputTokens int) error {
	cost := EstimateCost(inputTokens, outputTokens, b.cfg.PriceInputPerMTok, b.cfg.PriceOutputPerMTok)
	key := b.dayKey()
	// IncrByFloat is atomic; SETXX would race with concurrent calls.
	if _, err := b.c.IncrByFloat(ctx, key, cost).Result(); err != nil {
		return fmt.Errorf("incr budget: %w", err)
	}
	// Set TTL once per key (idempotent — Redis EXPIRE only updates the TTL,
	// so calling it on subsequent records is harmless and safer than missing it).
	if _, err := b.c.Expire(ctx, key, 25*time.Hour).Result(); err != nil {
		return fmt.Errorf("expire budget: %w", err)
	}
	return nil
}

// UsedYuan returns today's accumulated spend in yuan.
func (b *BudgetGate) UsedYuan(ctx context.Context) (float64, error) {
	v, err := b.c.Get(ctx, b.dayKey()).Result()
	if errors.Is(err, redis.Nil) {
		return 0.0, nil
	}
	if err != nil {
		return 0.0, err
	}
	used, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0.0, fmt.Errorf("parse used: %w", err)
	}
	return used, nil
}

// EstimateCost computes the yuan cost given token counts and per-million prices.
func EstimateCost(inputTokens, outputTokens int, priceInPerMTok, priceOutPerMTok float64) float64 {
	return (float64(inputTokens)/1_000_000.0)*priceInPerMTok + (float64(outputTokens)/1_000_000.0)*priceOutPerMTok
}
