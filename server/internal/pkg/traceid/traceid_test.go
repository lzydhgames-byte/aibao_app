package traceid

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_Unique(t *testing.T) {
	a := New()
	b := New()
	assert.NotEmpty(t, a)
	assert.NotEqual(t, a, b)
	assert.True(t, len(a) >= 10)
}

func TestContext_Roundtrip(t *testing.T) {
	ctx := WithID(context.Background(), "tr-abc")
	got, ok := FromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "tr-abc", got)
}

func TestContext_Missing(t *testing.T) {
	got, ok := FromContext(context.Background())
	assert.False(t, ok)
	assert.Empty(t, got)
}

func TestEnsure_GeneratesWhenMissing(t *testing.T) {
	ctx, id := Ensure(context.Background())
	assert.NotEmpty(t, id)
	got, _ := FromContext(ctx)
	assert.Equal(t, id, got)
}

func TestEnsure_KeepsExisting(t *testing.T) {
	ctx := WithID(context.Background(), "tr-existing")
	_, id := Ensure(ctx)
	assert.Equal(t, "tr-existing", id)
}
