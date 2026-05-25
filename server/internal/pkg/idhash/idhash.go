package idhash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type Hasher struct {
	secret []byte
}

func New(secret string) *Hasher {
	return &Hasher{secret: []byte(secret)}
}

// Hash returns HMAC-SHA256(secret, "<domain>:<id>") truncated to 12 hex chars.
// Domain separation prevents same-ID cross-table linkage (user:42 ≠ child:42).
func (h *Hasher) Hash(domain string, id int64) string {
	m := hmac.New(sha256.New, h.secret)
	fmt.Fprintf(m, "%s:%d", domain, id)
	sum := m.Sum(nil)
	return hex.EncodeToString(sum)[:12]
}
