// Package safehash provides salted SHA256 hashing for log-safe identifiers.
// It produces stable, non-reversible hashes used to correlate logs without
// leaking sensitive plaintext (phone numbers, child names, etc).
package safehash

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hasher applies a fixed salt to all values it hashes.
type Hasher struct {
	salt string
}

// New constructs a Hasher with the given salt.
// Panics if salt is empty — safehash without salt is meaningless (vulnerable
// to rainbow-table attacks).
func New(salt string) *Hasher {
	if salt == "" {
		panic("safehash: salt must not be empty")
	}
	return &Hasher{salt: salt}
}

// HashString returns "h_<first-12-hex-chars>" for non-empty input,
// or empty string when input is empty.
func (h *Hasher) HashString(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(h.salt + ":" + s))
	return "h_" + hex.EncodeToString(sum[:])[:12]
}
