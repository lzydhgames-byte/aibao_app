// Package api wires HTTP handlers and the router. Handlers in this package
// translate HTTP requests into service-layer calls; they do not contain
// business logic themselves.
package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Checker is the minimal interface the /ready endpoint uses to probe
// dependencies (PG, Redis). Repository.Ping/PingRedis are wrapped to satisfy
// this interface in main.go.
type Checker interface {
	Check(ctx context.Context) error
}

// RegisterHealth attaches /health and /ready endpoints to r. /health is a
// shallow liveness probe (process is up); /ready performs the supplied
// dependency checks and returns 503 if any fail.
func RegisterHealth(r *gin.Engine, pg, redis Checker) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
		ctx := c.Request.Context()
		problems := gin.H{}
		if pg != nil {
			if err := pg.Check(ctx); err != nil {
				problems["pg"] = err.Error()
			}
		}
		if redis != nil {
			if err := redis.Check(ctx); err != nil {
				problems["redis"] = err.Error()
			}
		}
		if len(problems) > 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "problems": problems})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
}
