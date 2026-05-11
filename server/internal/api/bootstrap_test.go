package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/api/userctx"
	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/service/bootstrap"
)

type fakeBootstrapSvc struct {
	profile *bootstrap.Profile
	err     error
	gotUID  int64
	gotCID  int64
	gotAns  []bootstrap.Answer
}

func (f *fakeBootstrapSvc) Submit(_ context.Context, uid, cid int64, ans []bootstrap.Answer) (*bootstrap.Profile, error) {
	f.gotUID = uid
	f.gotCID = cid
	f.gotAns = ans
	if f.err != nil {
		return nil, f.err
	}
	return f.profile, nil
}

func mkBootstrapRouter(h *BootstrapHandler, uid int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if uid != 0 {
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(userctx.WithUserID(c.Request.Context(), uid))
			c.Next()
		})
	}
	g := r.Group("/api/v1")
	h.RegisterRoutes(g)
	return r
}

func TestBootstrap_GetQuestions(t *testing.T) {
	fake := &fakeBootstrapSvc{}
	h := NewBootstrapHandler(fake)
	r := mkBootstrapRouter(h, 1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/bootstrap/questions", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body, "questions")
	qs, ok := body["questions"].([]any)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(qs), 6)
	assert.EqualValues(t, 1, body["version"])
}

func TestBootstrap_PostAnswers_OK(t *testing.T) {
	fake := &fakeBootstrapSvc{
		profile: &bootstrap.Profile{Version: 1, Description: "好孩子"},
	}
	h := NewBootstrapHandler(fake)
	r := mkBootstrapRouter(h, 42)

	payload := map[string]any{
		"child_id": 7,
		"answers": []map[string]any{
			{"q_id": "story_style", "value": "温馨治愈"},
		},
	}
	b, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/bootstrap/answers", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "好孩子", body["description"])
	assert.EqualValues(t, 42, fake.gotUID)
	assert.EqualValues(t, 7, fake.gotCID)
}

func TestBootstrap_PostAnswers_NoJWT(t *testing.T) {
	fake := &fakeBootstrapSvc{}
	h := NewBootstrapHandler(fake)
	r := mkBootstrapRouter(h, 0)

	b, _ := json.Marshal(map[string]any{"child_id": 1, "answers": []any{}})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/bootstrap/answers", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, 401, w.Code)
}

func TestBootstrap_PostAnswers_BadJSON(t *testing.T) {
	fake := &fakeBootstrapSvc{}
	h := NewBootstrapHandler(fake)
	r := mkBootstrapRouter(h, 42)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/bootstrap/answers", bytes.NewReader([]byte("not-json{")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, 400, w.Code)
}

func TestBootstrap_PostAnswers_NotOwner(t *testing.T) {
	fake := &fakeBootstrapSvc{
		err: apperr.New(apperr.CodePermissionDenied, "not_owner", "无权"),
	}
	h := NewBootstrapHandler(fake)
	r := mkBootstrapRouter(h, 42)

	b, _ := json.Marshal(map[string]any{"child_id": 7, "answers": []any{}})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/bootstrap/answers", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, 403, w.Code)
}

func TestBootstrap_PostAnswers_MissingRequired(t *testing.T) {
	fake := &fakeBootstrapSvc{
		err: apperr.New(apperr.CodeInvalidArgument, "missing_required", "缺"),
	}
	h := NewBootstrapHandler(fake)
	r := mkBootstrapRouter(h, 42)

	b, _ := json.Marshal(map[string]any{"child_id": 7, "answers": []any{}})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/bootstrap/answers", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, 400, w.Code)
}
