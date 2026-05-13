package api

import (
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

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
)

type fakeHBChildRepo struct {
	child *model.Child
	err   error
}

func (f *fakeHBChildRepo) FindByID(_ context.Context, id int64) (*model.Child, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.child == nil || f.child.ID != id {
		return nil, repository.ErrNotFound
	}
	return f.child, nil
}

type fakeHBStorylineRepo struct {
	out []*model.Storyline
	err error
}

func (f *fakeHBStorylineRepo) ListActiveByChild(_ context.Context, _ int64, _ int) ([]*model.Storyline, error) {
	return f.out, f.err
}

func setupHB(t *testing.T, uid int64, child *model.Child, lines []*model.Storyline, now time.Time) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(userctx.WithUserID(c.Request.Context(), uid))
		c.Next()
	})
	h := NewHeartbeatHandler(
		&fakeHBChildRepo{child: child},
		&fakeHBStorylineRepo{out: lines},
		func() time.Time { return now },
	)
	v1 := r.Group("/api/v1")
	h.RegisterRoutes(v1)
	return r
}

func mkHBChild() *model.Child {
	return &model.Child{ID: 1, UserID: 7, Nickname: "小宇"}
}

func TestHeartbeat_TimeSlices(t *testing.T) {
	cases := []struct {
		name string
		hour int
		want string
	}{
		{"morning", 6, "小宇早上好呀～"},
		{"noon", 12, "小宇中午好呀～"},
		{"afternoon", 16, "小宇下午好呀～"},
		{"evening", 20, "小宇晚上好呀～"},
		{"late23", 23, "小宇夜深了"},
		{"late3", 3, "小宇夜深了"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 5, 14, tc.hour, 0, 0, 0, time.Local)
			r := setupHB(t, 7, mkHBChild(), nil, now)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeat?child_id=1", nil)
			r.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Contains(t, out["greeting"], tc.want)
		})
	}
}

func TestHeartbeat_AppendsContinuationPromptWhenStorylinesExist(t *testing.T) {
	lines := []*model.Storyline{
		{ID: 1, ChildID: 1, EpisodeCount: 2, NextEpisodeHint: "h"},
		{ID: 2, ChildID: 1, EpisodeCount: 1},
	}
	now := time.Date(2026, 5, 14, 16, 0, 0, 0, time.Local)
	r := setupHB(t, 7, mkHBChild(), lines, now)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeat?child_id=1", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Contains(t, out["greeting"], "想继续之前的冒险吗？")
	items, _ := out["active_storylines"].([]any)
	assert.Len(t, items, 2)
}

func TestHeartbeat_LateNightDoesNotAppendContinuation(t *testing.T) {
	lines := []*model.Storyline{{ID: 1, ChildID: 1, EpisodeCount: 1}}
	now := time.Date(2026, 5, 14, 23, 30, 0, 0, time.Local)
	r := setupHB(t, 7, mkHBChild(), lines, now)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeat?child_id=1", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.NotContains(t, out["greeting"], "想继续之前的冒险吗？")
}

func TestHeartbeat_NoStorylines_GreetingOnly(t *testing.T) {
	now := time.Date(2026, 5, 14, 16, 0, 0, 0, time.Local)
	r := setupHB(t, 7, mkHBChild(), nil, now)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeat?child_id=1", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	items, _ := out["active_storylines"].([]any)
	assert.Len(t, items, 0)
	assert.NotEmpty(t, out["greeting"])
}

func TestHeartbeat_ChildNotOwnedByUser_403(t *testing.T) {
	now := time.Date(2026, 5, 14, 16, 0, 0, 0, time.Local)
	other := &model.Child{ID: 1, UserID: 999, Nickname: "小宇"}
	r := setupHB(t, 7, other, nil, now)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeat?child_id=1", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "not_owner")
}

func TestHeartbeat_ChildNotFound_404(t *testing.T) {
	now := time.Date(2026, 5, 14, 16, 0, 0, 0, time.Local)
	r := setupHB(t, 7, nil, nil, now)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeat?child_id=42", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHeartbeat_MissingChildID_400(t *testing.T) {
	now := time.Date(2026, 5, 14, 16, 0, 0, 0, time.Local)
	r := setupHB(t, 7, mkHBChild(), nil, now)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeat", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// errChildRepo simulates a non-ErrNotFound failure (e.g. DB blew up).
type errChildRepo struct{}

func (errChildRepo) FindByID(_ context.Context, _ int64) (*model.Child, error) {
	return nil, errors.New("db gone")
}

func TestHeartbeat_ChildLookupError_500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(userctx.WithUserID(c.Request.Context(), 7))
		c.Next()
	})
	h := NewHeartbeatHandler(errChildRepo{}, &fakeHBStorylineRepo{}, time.Now)
	h.RegisterRoutes(r.Group("/api/v1"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeat?child_id=1", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
