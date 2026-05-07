package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewRedisCodeStore returns a CodeStore backed by go-redis.
func NewRedisCodeStore(c *redis.Client) CodeStore { return &redisStore{c: c} }

type redisStore struct {
	c *redis.Client
}

func codeKey(phoneHash string) string     { return "auth:code:" + phoneHash }
func cooldownKey(phoneHash string) string { return "auth:cd:" + phoneHash }

func (s *redisStore) Save(ctx context.Context, phoneHash, code string, codeTTL, cooldown time.Duration) error {
	ok, err := s.c.SetNX(ctx, cooldownKey(phoneHash), "1", cooldown).Result()
	if err != nil {
		return fmt.Errorf("set cooldown: %w", err)
	}
	if !ok {
		return ErrCooldown
	}
	if err := s.c.Set(ctx, codeKey(phoneHash), code, codeTTL).Err(); err != nil {
		return fmt.Errorf("set code: %w", err)
	}
	return nil
}

func (s *redisStore) Take(ctx context.Context, phoneHash string) (string, error) {
	got, err := s.c.GetDel(ctx, codeKey(phoneHash)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCodeNotFound
	}
	if err != nil {
		return "", err
	}
	return got, nil
}
