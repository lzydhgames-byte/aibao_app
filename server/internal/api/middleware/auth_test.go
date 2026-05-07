package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/pkg/jwtauth"
)

func newTestMgr() *jwtauth.Manager {
	return jwtauth.New("secret-x", time.Hour, time.Hour)
}

func TestJWTAuth_AcceptsValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := newTestMgr()
	tok, err := mgr.IssueAccess(42)
	require.NoError(t, err)

	r := gin.New()
	r.Use(JWTAuth(mgr))

	var seen int64
	r.GET("/x", func(c *gin.Context) {
		uid, _ := userctx.FromContext(c.Request.Context())
		seen = uid
		c.Status(200)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, int64(42), seen)
}

func TestJWTAuth_RejectsMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(JWTAuth(newTestMgr()))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestJWTAuth_RejectsBadToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(JWTAuth(newTestMgr()))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	r.ServeHTTP(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestJWTAuth_RejectsWrongScheme(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(JWTAuth(newTestMgr()))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	tok, _ := newTestMgr().IssueAccess(1)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Token "+tok) // wrong scheme
	r.ServeHTTP(rec, req)
	assert.Equal(t, 401, rec.Code)
}
