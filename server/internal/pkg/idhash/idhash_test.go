package idhash_test

import (
	"testing"

	"github.com/aibao/server/internal/pkg/idhash"
)

func TestHash_DomainSeparation(t *testing.T) {
	h := idhash.New("test-secret")
	user42 := h.Hash("user", 42)
	child42 := h.Hash("child", 42)
	if user42 == child42 {
		t.Fatalf("expected domain-separated hashes to differ, both = %s", user42)
	}
}

func TestHash_Stable(t *testing.T) {
	h := idhash.New("test-secret")
	a := h.Hash("user", 42)
	b := h.Hash("user", 42)
	if a != b {
		t.Fatalf("expected stable hash, got %s vs %s", a, b)
	}
	if len(a) != 12 {
		t.Fatalf("expected 12 hex chars, got %d (%s)", len(a), a)
	}
}

func TestHash_SecretChange(t *testing.T) {
	h1 := idhash.New("secret-a")
	h2 := idhash.New("secret-b")
	if h1.Hash("user", 42) == h2.Hash("user", 42) {
		t.Fatalf("expected different secrets to produce different hashes")
	}
}
