package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/metrics"
)

// Metrics records http_requests_total and http_request_duration_seconds for
// every request, labeled by route path and response status. Routes that don't
// match a registered pattern are bucketed as "unknown" (avoids cardinality
// explosions from random paths).
func Metrics(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		m.HTTPRequests.WithLabelValues(path, status).Inc()
		m.HTTPDuration.WithLabelValues(path, status).Observe(time.Since(start).Seconds())
	}
}
