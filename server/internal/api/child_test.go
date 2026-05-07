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
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/repository"
	"github.com/aibao/server/internal/service/child"
)

type childFakeRepo struct {
	byUser map[int64]*model.Child
	byID   map[int64]*model.Child
	next   int64
}

func newChildFakeRepo() *childFakeRepo {
	return &childFakeRepo{byUser: map[int64]*model.Child{}, byID: map[int64]*model.Child{}, next: 1}
}

func (r *childFakeRepo) Create(_ context.Context, c *model.Child) error {
	if _, ok := r.byUser[c.UserID]; ok {
		return repository.ErrAlreadyExists
	}
	c.ID = r.next
	r.next++
	r.byID[c.ID] = c
	r.byUser[c.UserID] = c
	return nil
}
func (r *childFakeRepo) FindByUserID(_ context.Context, uid int64) (*model.Child, error) {
	if c, ok := r.byUser[uid]; ok {
		return c, nil
	}
	return nil, repository.ErrNotFound
}
func (r *childFakeRepo) FindByID(_ context.Context, id int64) (*model.Child, error) {
	if c, ok := r.byID[id]; ok {
		return c, nil
	}
	return nil, repository.ErrNotFound
}
func (r *childFakeRepo) Update(_ context.Context, c *model.Child) error {
	r.byID[c.ID] = c
	r.byUser[c.UserID] = c
	return nil
}

func setupChild(t *testing.T, asUser int64) (*gin.Engine, *childFakeRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newChildFakeRepo()
	svc := child.New(repo)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(userctx.WithUserID(c.Request.Context(), asUser))
		c.Next()
	})
	v1 := r.Group("/api/v1")
	NewChildHandler(svc).RegisterRoutes(v1)
	return r, repo
}

func doJSON(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	var req *http.Request
	if rd != nil {
		req = httptest.NewRequest(method, path, rd)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestChild_Create_OK(t *testing.T) {
	r, _ := setupChild(t, 7)
	rec := doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "小宇", "gender": "boy", "birthday": "2020-08-15",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, rec.Body.String(), "小宇")
	assert.Contains(t, rec.Body.String(), `"birthday":"2020-08-15"`)
}

func TestChild_Create_Conflict(t *testing.T) {
	r, _ := setupChild(t, 7)
	require.Equal(t, http.StatusCreated, doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "a", "gender": "boy", "birthday": "2020-08-15",
	}).Code)
	rec := doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "b", "gender": "girl", "birthday": "2020-08-15",
	})
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "child_already_exists")
}

func TestChild_List_EmptyAndNonEmpty(t *testing.T) {
	r, _ := setupChild(t, 7)
	rec := doJSON(r, http.MethodGet, "/api/v1/children", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var out struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Empty(t, out.Items)

	require.Equal(t, http.StatusCreated, doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "n", "gender": "boy", "birthday": "2020-08-15",
	}).Code)
	rec = doJSON(r, http.MethodGet, "/api/v1/children", nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Len(t, out.Items, 1)
}

func TestChild_Update_OK(t *testing.T) {
	r, _ := setupChild(t, 7)
	doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "n", "gender": "boy", "birthday": "2020-08-15",
	})
	rec := doJSON(r, http.MethodPatch, "/api/v1/children/1", map[string]string{"nickname": "n2"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "n2")
}

func TestChild_Update_Forbidden(t *testing.T) {
	r, _ := setupChild(t, 7)
	doJSON(r, http.MethodPost, "/api/v1/children", map[string]string{
		"nickname": "n", "gender": "boy", "birthday": "2020-08-15",
	})

	r2, _ := setupChild(t, 99)
	rec := doJSON(r2, http.MethodPatch, "/api/v1/children/1", map[string]string{"nickname": "x"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
