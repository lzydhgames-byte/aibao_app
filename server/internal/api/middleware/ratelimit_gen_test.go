package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/aibao/server/internal/api/userctx"
)

type fakeCounter struct {
	val map[string]int
}

func newFakeCounter() *fakeCounter { return &fakeCounter{val: map[string]int{}} }

func (f *fakeCounter) IncrWithTTL(_ context.Context, key string, _ time.Duration) (int64, error) {
	f.val[key]++
	return int64(f.val[key]), nil
}

func TestGenerateRateLimit_AllowsUnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	c := newFakeCounter()
	r.Use(injectUser(7), GenerateRateLimit(c, 5, time.Minute))
	r.POST("/x", func(c *gin.Context) { c.Status(200) })
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		r.ServeHTTP(rec, req)
		assert.Equal(t, 200, rec.Code, "i=%d", i)
	}
}

func TestGenerateRateLimit_RejectsOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	c := newFakeCounter()
	c.val["rate:gen:7"] = 5
	r.Use(injectUser(7), GenerateRateLimit(c, 5, time.Minute))
	r.POST("/x", func(c *gin.Context) { c.Status(200) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestGenerateRateLimit_NoUserCtxAllows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	c := newFakeCounter()
	r.Use(GenerateRateLimit(c, 5, time.Minute))
	r.POST("/x", func(c *gin.Context) { c.Status(200) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, 200, rec.Code)
}

func injectUser(uid int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(userctx.WithUserID(c.Request.Context(), uid))
		c.Next()
	}
}
