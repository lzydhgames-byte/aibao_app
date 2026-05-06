package safehash

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHash_Stable(t *testing.T) {
	h := New("test-salt")
	a := h.HashString("13800138000")
	b := h.HashString("13800138000")
	assert.Equal(t, a, b, "same input must produce same hash")
}

func TestHash_DifferentSaltDifferentResult(t *testing.T) {
	a := New("salt1").HashString("x")
	b := New("salt2").HashString("x")
	assert.NotEqual(t, a, b)
}

func TestHash_Prefix(t *testing.T) {
	h := New("salt")
	got := h.HashString("foo")
	assert.True(t, len(got) > 4)
	assert.Equal(t, "h_", got[:2])
}

func TestHash_EmptyInput(t *testing.T) {
	h := New("salt")
	assert.Equal(t, "", h.HashString(""), "empty input returns empty string")
}

func TestNew_RequiresSalt(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on empty salt")
		}
	}()
	_ = New("")
}
