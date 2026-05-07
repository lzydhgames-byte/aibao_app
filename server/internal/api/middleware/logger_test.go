package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/pkg/logger"
)

func TestLogger_LogsStartAndEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger.SetDefault(logger.NewWithWriter(&buf, "debug"))

	r := gin.New()
	r.Use(TraceID(), Logger())
	r.GET("/x", func(c *gin.Context) { c.Status(204) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(rec, req)

	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	require.GreaterOrEqual(t, len(lines), 2)

	var start, end map[string]any
	require.NoError(t, json.Unmarshal(lines[0], &start))
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &end))

	assert.Equal(t, "http.request.start", start["msg"])
	assert.Equal(t, "http.request.done", end["msg"])
	assert.Equal(t, start["trace_id"], end["trace_id"])
	assert.Equal(t, float64(204), end["status"])
	assert.NotNil(t, end["duration_ms"])
}
