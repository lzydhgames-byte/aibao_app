package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/pkg/logger"
)

// Logger logs structured "request start" and "request done" events for every
// request. The done event includes status code and duration_ms.
// The same trace_id appears on both lines (and on any downstream business
// logs) so a single request's full timeline can be reconstructed.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		lg := logger.FromCtx(c.Request.Context())
		lg.Info("http.request.start",
			"method", c.Request.Method,
			"path", c.FullPath(),
		)
		c.Next()
		lg.Info("http.request.done",
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}
