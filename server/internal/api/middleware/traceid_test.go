package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/aibao/server/internal/pkg/traceid"
)

func TestTraceID_GeneratesWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceID())

	var seen string
	r.GET("/x", func(c *gin.Context) {
		id, _ := traceid.FromContext(c.Request.Context())
		seen = id
		c.Status(200)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(rec, req)

	assert.NotEmpty(t, seen)
	assert.Equal(t, seen, rec.Header().Get("X-Trace-Id"))
}

func TestTraceID_HonorsIncoming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceID())

	var seen string
	r.GET("/x", func(c *gin.Context) {
		id, _ := traceid.FromContext(c.Request.Context())
		seen = id
		c.Status(200)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Trace-Id", "tr-incoming")
	r.ServeHTTP(rec, req)

	assert.Equal(t, "tr-incoming", seen)
	assert.Equal(t, "tr-incoming", rec.Header().Get("X-Trace-Id"))
}
