package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/gateway/storage"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
)

type stubStoryReader struct {
	s   *model.Story
	err error
}

func (r *stubStoryReader) FindByID(_ context.Context, _ int64) (*model.Story, error) {
	return r.s, r.err
}

type stubChildReader struct {
	c   *model.Child
	err error
}

func (r *stubChildReader) FindByID(_ context.Context, _ int64) (*model.Child, error) {
	return r.c, r.err
}

func mkAudioRouter(h *AudioHandler, uid int64) *gin.Engine {
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

func TestAudioURL_Pending(t *testing.T) {
	st := storage.NewMock()
	h := NewAudioHandler(
		&stubStoryReader{s: &model.Story{ID: 1, ChildID: 7, AudioStatus: model.AudioStatusPending}},
		&stubChildReader{c: &model.Child{ID: 7, UserID: 99}},
		st, 15*time.Minute,
	)
	r := mkAudioRouter(h, 99)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/stories/1/audio_url", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var body audioURLResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "pending", body.AudioStatus)
	assert.Equal(t, 5, body.RetryAfter)
}

func TestAudioURL_Ready(t *testing.T) {
	st := storage.NewMock()
	_, upErr := st.Upload(context.Background(), storage.UploadInput{
		Key: "audio/7/1-x.mp3", Body: strings.NewReader("xxx"), Size: 3,
	})
	require.NoError(t, upErr)
	h := NewAudioHandler(
		&stubStoryReader{s: &model.Story{
			ID: 1, ChildID: 7, AudioStatus: model.AudioStatusReady, AudioObjectKey: "audio/7/1-x.mp3",
		}},
		&stubChildReader{c: &model.Child{ID: 7, UserID: 99}},
		st, 15*time.Minute,
	)
	r := mkAudioRouter(h, 99)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/stories/1/audio_url", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var body audioURLResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ready", body.AudioStatus)
	assert.Contains(t, body.URL, "audio/7/1-x.mp3")
}

func TestAudioURL_Failed(t *testing.T) {
	h := NewAudioHandler(
		&stubStoryReader{s: &model.Story{ID: 1, ChildID: 7, AudioStatus: model.AudioStatusFailed}},
		&stubChildReader{c: &model.Child{ID: 7, UserID: 99}},
		storage.NewMock(), 15*time.Minute,
	)
	r := mkAudioRouter(h, 99)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/stories/1/audio_url", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 503, w.Code)
}

func TestAudioURL_NotOwner(t *testing.T) {
	h := NewAudioHandler(
		&stubStoryReader{s: &model.Story{ID: 1, ChildID: 7, AudioStatus: model.AudioStatusReady, AudioObjectKey: "k"}},
		&stubChildReader{c: &model.Child{ID: 7, UserID: 1234}},
		storage.NewMock(), 15*time.Minute,
	)
	r := mkAudioRouter(h, 99)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/stories/1/audio_url", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 403, w.Code)
}

func TestAudioURL_Unauthenticated(t *testing.T) {
	h := NewAudioHandler(
		&stubStoryReader{s: &model.Story{ID: 1, ChildID: 7, AudioStatus: model.AudioStatusPending}},
		&stubChildReader{c: &model.Child{ID: 7, UserID: 99}},
		storage.NewMock(), 15*time.Minute,
	)
	r := mkAudioRouter(h, 0)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/stories/1/audio_url", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 401, w.Code)
}

func TestAudioURL_StoryNotFound(t *testing.T) {
	h := NewAudioHandler(
		&stubStoryReader{err: repository.ErrNotFound},
		&stubChildReader{},
		storage.NewMock(), 15*time.Minute,
	)
	r := mkAudioRouter(h, 99)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/stories/999/audio_url", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 404, w.Code)
}

var _ = errors.New // keep errors import for future tests
