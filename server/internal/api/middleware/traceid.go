package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/pkg/traceid"
)

// TraceID middleware ensures every request has a trace id: it honors an
// incoming X-Trace-Id header if present, or generates a new one. The trace id
// is attached to the request context (so downstream handlers can read it via
// traceid.FromContext) and echoed in the response header for client logging.
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		incoming := c.GetHeader(traceid.Header)
		var id string
		if incoming != "" {
			id = incoming
		} else {
			id = traceid.New()
		}
		ctx := traceid.WithID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Header(traceid.Header, id)
		c.Next()
	}
}
