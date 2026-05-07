package phonecrypt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRoundTrip(t *testing.T) {
	c, err := New(testKeyHex)
	require.NoError(t, err)

	enc, err := c.Encrypt("13800138000")
	require.NoError(t, err)
	require.NotEmpty(t, enc)

	dec, err := c.Decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, "13800138000", dec)
}

func TestEncrypt_DifferentNonceEachCall(t *testing.T) {
	c, _ := New(testKeyHex)
	a, _ := c.Encrypt("13800138000")
	b, _ := c.Encrypt("13800138000")
	assert.NotEqual(t, a, b, "AES-GCM with random nonce must produce different ciphertexts")
}

func TestNew_RejectsShortKey(t *testing.T) {
	_, err := New("deadbeef")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "32 bytes")
}

func TestNew_RejectsBadHex(t *testing.T) {
	_, err := New("not-hex-and-too-shortzzz" + strings.Repeat("z", 40))
	require.Error(t, err)
}

func TestDecrypt_RejectsTampered(t *testing.T) {
	c, _ := New(testKeyHex)
	enc, _ := c.Encrypt("13800138000")
	enc[len(enc)-1] ^= 0x01
	_, err := c.Decrypt(enc)
	assert.Error(t, err)
}
