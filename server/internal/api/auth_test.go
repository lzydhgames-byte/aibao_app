package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/jwtauth"
	"github.com/aibao/server/internal/pkg/safehash"
	"github.com/aibao/server/internal/service/auth"
)

type fakeUserRepo struct{ created *model.User }

func (f *fakeUserRepo) CreateOrGet(_ context.Context, u *model.User) (*model.User, bool, error) {
	u.ID = 7
	f.created = u
	return u, true, nil
}
func (f *fakeUserRepo) FindByID(_ context.Context, id int64) (*model.User, error) {
	if f.created != nil && f.created.ID == id {
		return f.created, nil
	}
	return nil, errors.New("not found")
}

type fakeStore struct{ saved string }

func (f *fakeStore) Save(_ context.Context, _, c string, _, _ time.Duration) error {
	f.saved = c
	return nil
}
func (f *fakeStore) Peek(_ context.Context, _ string) (string, error) {
	if f.saved == "" {
		return "", auth.ErrCodeNotFound
	}
	return f.saved, nil
}
func (f *fakeStore) Consume(_ context.Context, _ string) error {
	f.saved = ""
	return nil
}

type fakeSMS struct{}

func (fakeSMS) SendCode(_ context.Context, _, _ string) error { return nil }

type fakePC struct{}

func (fakePC) Encrypt(s string) ([]byte, error) { return []byte(s), nil }
func (fakePC) Decrypt(b []byte) (string, error) { return string(b), nil }

func setupAuth(t *testing.T) (*gin.Engine, *fakeUserRepo, *fakeStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &fakeUserRepo{}
	cs := &fakeStore{}
	jwt := jwtauth.New("s", time.Hour, time.Hour)
	svc := auth.New(auth.Deps{
		Users: repo, CodeStore: cs, SMS: fakeSMS{}, JWT: jwt,
		PhoneCipher: fakePC{}, Hasher: safehash.New("salt"),
		FixedDevCode: "123456", CodeTTL: time.Minute, Cooldown: time.Second,
	})
	r := gin.New()
	v1 := r.Group("/api/v1")
	NewAuthHandler(svc).RegisterRoutes(v1)
	return r, repo, cs
}

func postJSON(r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestSmsSend_OK(t *testing.T) {
	r, _, cs := setupAuth(t)
	rec := postJSON(r, "/api/v1/auth/sms/send", map[string]string{"phone": "13800138000"})
	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "123456", cs.saved)
}

func TestSmsSend_InvalidPhone(t *testing.T) {
	r, _, _ := setupAuth(t)
	rec := postJSON(r, "/api/v1/auth/sms/send", map[string]string{"phone": "abc"})
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "phone_invalid")
}

func TestLoginOrRegister_OK(t *testing.T) {
	r, _, _ := setupAuth(t)
	require.Equal(t, 200, postJSON(r, "/api/v1/auth/sms/send", map[string]string{"phone": "13800138000"}).Code)

	rec := postJSON(r, "/api/v1/auth/login_or_register", map[string]string{
		"phone": "13800138000", "code": "123456", "nickname": "妈妈",
	})
	require.Equal(t, 200, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.NotEmpty(t, out["access_token"])
	assert.NotEmpty(t, out["refresh_token"])
	user := out["user"].(map[string]any)
	assert.Equal(t, "妈妈", user["nickname"])
}

func TestLoginOrRegister_RejectsNonUTF8Nickname(t *testing.T) {
	r, _, _ := setupAuth(t)
	require.Equal(t, 200, postJSON(r, "/api/v1/auth/sms/send", map[string]string{"phone": "13800138000"}).Code)

	// Build raw body with invalid UTF-8 bytes in nickname (cannot use json.Marshal
	// which would replace invalid bytes with U+FFFD).
	prefix := []byte(`{"phone":"13800138000","code":"123456","nickname":"`)
	invalid := []byte{0xc8, 0xed, 0xa1, 0xa1}
	suffix := []byte(`"}`)
	body := append(append(prefix, invalid...), suffix...)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login_or_register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid_nickname")
}

func TestLoginOrRegister_BadCode(t *testing.T) {
	r, _, _ := setupAuth(t)
	require.Equal(t, 200, postJSON(r, "/api/v1/auth/sms/send", map[string]string{"phone": "13800138000"}).Code)

	rec := postJSON(r, "/api/v1/auth/login_or_register", map[string]string{
		"phone": "13800138000", "code": "999999",
	})
	assert.Equal(t, 401, rec.Code)
	assert.Contains(t, rec.Body.String(), "code_mismatch")
}
