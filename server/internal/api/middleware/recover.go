// Package middleware contains Gin HTTP middlewares used by the server router.
package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/pkg/logger"
)

// Recover catches any panic in downstream handlers, logs it with stack trace,
// and responds with a generic 500. This prevents a single buggy handler from
// taking down the entire process.
func Recover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.FromCtx(c.Request.Context()).Error(
					"http.panic",
					"panic", rec,
					"stack", string(debug.Stack()),
					"path", c.FullPath(),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"reason":   "internal_error",
					"user_msg": "服务暂时不可用，请稍后再试",
				})
			}
		}()
		c.Next()
	}
}
