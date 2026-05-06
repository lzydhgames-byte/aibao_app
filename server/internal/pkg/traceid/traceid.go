package traceid

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey struct{}

// Header is the HTTP header name carrying the trace id end-to-end.
const Header = "X-Trace-Id"

// New returns a new trace id like "tr-<8-char>" (truncated UUID for log brevity).
func New() string {
	return "tr-" + uuid.NewString()[0:8]
}

// WithID returns a copy of ctx that carries the given trace id.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the trace id stored in ctx, if any.
func FromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKey{}).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// Ensure returns a context with a trace id, generating one if absent.
func Ensure(ctx context.Context) (context.Context, string) {
	if id, ok := FromContext(ctx); ok {
		return ctx, id
	}
	id := New()
	return WithID(ctx, id), id
}
