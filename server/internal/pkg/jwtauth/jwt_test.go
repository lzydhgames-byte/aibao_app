package jwtauth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueAndParseAccess_RoundTrip(t *testing.T) {
	m := New("secret-x", time.Hour, 7*24*time.Hour)
	tok, err := m.IssueAccess(42)
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	claims, err := m.ParseAccess(tok)
	require.NoError(t, err)
	assert.Equal(t, int64(42), claims.UserID)
	assert.Equal(t, "access", claims.Type)
}

func TestParseAccess_RejectsRefreshToken(t *testing.T) {
	m := New("secret-x", time.Hour, 7*24*time.Hour)
	tok, err := m.IssueRefresh(42)
	require.NoError(t, err)

	_, err = m.ParseAccess(tok)
	assert.Error(t, err, "refresh token should not be accepted by ParseAccess")
}

func TestParseAccess_RejectsBadSignature(t *testing.T) {
	a := New("secret-a", time.Hour, time.Hour)
	b := New("secret-b", time.Hour, time.Hour)
	tok, _ := a.IssueAccess(1)
	_, err := b.ParseAccess(tok)
	assert.Error(t, err)
}

func TestParseAccess_RejectsExpired(t *testing.T) {
	m := New("secret-x", -time.Minute, time.Hour) // already-expired
	tok, _ := m.IssueAccess(1)
	_, err := m.ParseAccess(tok)
	assert.Error(t, err)
}

func TestParseAccess_RejectsMalformed(t *testing.T) {
	m := New("secret-x", time.Hour, time.Hour)
	_, err := m.ParseAccess("not-a-jwt")
	assert.Error(t, err)
}
