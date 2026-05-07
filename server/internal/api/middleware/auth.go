package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/api/userctx"
	"github.com/aibao/server/internal/pkg/jwtauth"
)

// JWTAuth requires a valid Bearer access token. On success, the user id is
// attached to the request context via userctx.WithUserID.
func JWTAuth(mgr *jwtauth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"reason":   "unauthorized",
				"user_msg": "请先登录",
			})
			return
		}
		tok := strings.TrimPrefix(auth, prefix)
		claims, err := mgr.ParseAccess(tok)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"reason":   "unauthorized",
				"user_msg": "登录已过期，请重新登录",
			})
			return
		}
		ctx := userctx.WithUserID(c.Request.Context(), claims.UserID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
