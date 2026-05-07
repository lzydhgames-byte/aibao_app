// Package jwtauth issues and validates HS256 JWTs for the auth flow.
// Two token types are issued: access (short-lived) and refresh (long-lived).
// The Type claim distinguishes them so an attacker cannot present a refresh
// token where an access token is expected.
package jwtauth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Manager issues and validates JWTs.
type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// New constructs a Manager with the given HMAC secret and TTLs.
func New(secret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

// Claims is the JWT claim set we use.
type Claims struct {
	UserID int64  `json:"uid"`
	Type   string `json:"typ"` // "access" or "refresh"
	jwt.RegisteredClaims
}

// IssueAccess issues a short-lived access token for the given user.
func (m *Manager) IssueAccess(userID int64) (string, error) {
	return m.issue(userID, "access", m.accessTTL)
}

// IssueRefresh issues a long-lived refresh token for the given user.
func (m *Manager) IssueRefresh(userID int64) (string, error) {
	return m.issue(userID, "refresh", m.refreshTTL)
}

func (m *Manager) issue(userID int64, typ string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Type:   typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signed, nil
}

// ParseAccess validates the token and returns its claims, but only if Type=="access".
func (m *Manager) ParseAccess(s string) (*Claims, error) {
	return m.parse(s, "access")
}

// ParseRefresh validates the token and returns its claims, but only if Type=="refresh".
func (m *Manager) ParseRefresh(s string) (*Claims, error) {
	return m.parse(s, "refresh")
}

func (m *Manager) parse(s, requiredType string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(s, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.Type != requiredType {
		return nil, fmt.Errorf("expected %s token, got %s", requiredType, claims.Type)
	}
	return claims, nil
}
