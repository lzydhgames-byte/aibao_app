package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/model"
)

type meFakeUsers struct {
	byID map[int64]*model.User
}

func (f *meFakeUsers) FindByID(_ context.Context, id int64) (*model.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, errors.New("not found")
}

func TestMe_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := &meFakeUsers{byID: map[int64]*model.User{
		7: {ID: 7, Nickname: "妈妈", SubscriptionTier: "free"},
	}}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(userctx.WithUserID(c.Request.Context(), 7))
		c.Next()
	})
	v1 := r.Group("/api/v1")
	NewMeHandler(users).RegisterRoutes(v1)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "妈妈")
}

func TestMe_NoUserCtx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	NewMeHandler(&meFakeUsers{byID: map[int64]*model.User{}}).RegisterRoutes(v1)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
