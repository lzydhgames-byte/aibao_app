// Package userctx stores the authenticated user id in request context.
// It exists in a tiny dedicated package to avoid api ↔ middleware import cycles.
package userctx

import "context"

type ctxKey struct{}

// WithUserID returns ctx carrying the given user id.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext extracts the user id, returning ok=false when absent.
func FromContext(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(ctxKey{}).(int64)
	if !ok || v == 0 {
		return 0, false
	}
	return v, true
}
