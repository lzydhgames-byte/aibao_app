// Package auth implements the SMS-code-based login/register flow.
package auth

import (
	"context"
	"errors"
	"time"
)

// ErrCooldown is returned when a new code is requested within the resend
// cooldown window.
var ErrCooldown = errors.New("resend cooldown")

// ErrCodeNotFound is returned when no code is stored for the phone (expired or never sent).
var ErrCodeNotFound = errors.New("code not found")

// CodeStore stores SMS verification codes with TTL and per-phone resend cooldown.
type CodeStore interface {
	// Save persists code under phoneHash with the given codeTTL. If a
	// previous Save happened within cooldown, returns ErrCooldown without
	// overwriting the existing code.
	Save(ctx context.Context, phoneHash, code string, codeTTL, cooldown time.Duration) error

	// Take fetches and atomically deletes the code for phoneHash. Returns
	// ErrCodeNotFound if absent or already taken.
	Take(ctx context.Context, phoneHash string) (string, error)
}
