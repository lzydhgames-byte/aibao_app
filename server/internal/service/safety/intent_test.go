package safety

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopIntentProvider_AlwaysSafe(t *testing.T) {
	p := NewNoopIntentProvider()
	out, err := p.Classify(context.Background(), "我想要血腥的故事")
	assert.NoError(t, err)
	assert.Equal(t, IntentSafe, out)
}

func TestIntent_String(t *testing.T) {
	assert.Equal(t, "safe", IntentSafe.String())
	assert.Equal(t, "uncertain", IntentUncertain.String())
	assert.Equal(t, "unsafe", IntentUnsafe.String())
}
