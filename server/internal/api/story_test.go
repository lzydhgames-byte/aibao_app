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

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/service/safety"
	"github.com/aibao/server/internal/service/story"
)

type fakeStoryRepo struct {
	last *model.Story
}

func (f *fakeStoryRepo) CreateWithOutbox(_ context.Context, s *model.Story, _ []*model.StoryElement, evs []*model.OutboxEvent) error {
	s.ID = 555
	s.CreatedAt = time.Now()
	f.last = s
	for i, ev := range evs {
		ev.ID = int64(999 + i)
	}
	return nil
}
func (f *fakeStoryRepo) FindByID(_ context.Context, id int64) (*model.Story, error) {
	if f.last != nil && f.last.ID == id {
		return f.last, nil
	}
	return nil, errors.New("not found")
}

type fakeChildRepo struct{}

func (fakeChildRepo) FindByID(_ context.Context, id int64) (*model.Child, error) {
	bday, _ := time.Parse("2006-01-02", "2020-08-15")
	return &model.Child{ID: id, UserID: 7, Nickname: "小宇", Gender: "boy", Birthday: bday, Profile: []byte(`{}`)}, nil
}

type allowBudget struct{}

func (allowBudget) PreCheck(_ context.Context) error          { return nil }
func (allowBudget) Record(_ context.Context, _, _ int) error { return nil }

func setupStoryHandler(t *testing.T) (*gin.Engine, *fakeStoryRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	srepo := &fakeStoryRepo{}
	mock := llm.NewMock()
	mock.Response.Text = "小宇推开门，决定走进竹林。爱宝跟着小宇。小宇说我们走吧。小宇带头前进。"
	rs := &safety.RuleSet{AllRedlinesFlat: []string{"血腥"}, Redlines: map[string][]string{"violence": {"血腥"}}}
	orch, err := story.NewOrchestrator(story.Deps{
		Stories: srepo, Children: fakeChildRepo{}, LLM: mock,
		Budget:        allowBudget{},
		PreCheck:      safety.NewPreChecker(rs, safety.NewNoopIntentProvider()),
		PostCheck:     safety.NewPostChecker(rs),
		PromptTmpl:    "../../safety/system_prompt.tmpl",
		FallbackDir:   "../../safety/fallback_stories",
		StoryModel:    "doubao-1.5-pro-32k",
		Temperature:   0.8,
		PromptVersion: "v1",
	})
	require.NoError(t, err)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(userctx.WithUserID(c.Request.Context(), 7))
		c.Next()
	})
	v1 := r.Group("/api/v1")
	NewStoryHandler(orch, srepo).RegisterRoutes(v1)
	return r, srepo
}

func TestStoryHandler_Generate_OK(t *testing.T) {
	r, _ := setupStoryHandler(t)
	body, _ := json.Marshal(map[string]any{
		"child_id": 1, "prompt": "讲个奥特曼睡前故事",
		"duration": 10, "style": "温馨治愈", "topic": "勇敢",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stories/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.NotZero(t, out["id"])
	assert.Contains(t, out["text"], "小宇")
}

func TestStoryHandler_Generate_InvalidDuration(t *testing.T) {
	r, _ := setupStoryHandler(t)
	body, _ := json.Marshal(map[string]any{
		"child_id": 1, "prompt": "x", "duration": 7, "style": "温馨治愈",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stories/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid_duration")
}

func TestStoryHandler_Generate_PreCheckRejection(t *testing.T) {
	r, _ := setupStoryHandler(t)
	body, _ := json.Marshal(map[string]any{
		"child_id": 1, "prompt": "我要血腥的故事",
		"duration": 10, "style": "温馨治愈",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stories/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "redline_matched")
}

func TestStoryHandler_Get_OK(t *testing.T) {
	r, repo := setupStoryHandler(t)
	body, _ := json.Marshal(map[string]any{
		"child_id": 1, "prompt": "讲个故事",
		"duration": 10, "style": "温馨治愈",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stories/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.last)

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/stories/555", nil)
	r.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "id")
}
