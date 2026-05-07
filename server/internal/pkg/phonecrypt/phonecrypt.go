// Package phonecrypt provides AES-256-GCM encryption for phone numbers stored
// at rest. Plaintext is needed only when sending real SMS; queries always use
// the safehash representation. Each call to Encrypt produces a fresh random
// nonce, prefixed to the ciphertext so Decrypt can recover it.
package phonecrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// Cipher encrypts/decrypts strings with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// New constructs a Cipher from a 64-char hex key (32 bytes / 256 bits).
func New(keyHex string) (*Cipher, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("phone aes key must be 32 bytes (got %d)", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns a byte slice of the form: nonce || ciphertext || tag.
func (c *Cipher) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("read random nonce: %w", err)
	}
	out := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return out, nil
}

// Decrypt parses a value produced by Encrypt and returns the plaintext.
func (c *Cipher) Decrypt(blob []byte) (string, error) {
	ns := c.aead.NonceSize()
	if len(blob) < ns+c.aead.Overhead() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := blob[:ns], blob[ns:]
	pt, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("aes-gcm open: %w", err)
	}
	return string(pt), nil
}
