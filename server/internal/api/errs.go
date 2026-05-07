package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperr "github.com/aibao/server/internal/pkg/errors"
	"github.com/aibao/server/internal/pkg/logger"
)

// RespondError translates err into a JSON response. AppError errors are mapped
// to their declared HTTP status; everything else becomes 500 internal_error.
func RespondError(c *gin.Context, err error) {
	if ae, ok := apperr.AsAppError(err); ok {
		c.AbortWithStatusJSON(ae.HTTPStatus(), gin.H{
			"reason":   ae.Reason,
			"user_msg": ae.UserMsg,
		})
		return
	}
	logger.FromCtx(c.Request.Context()).Error("api.unexpected_error", "err", err.Error())
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
		"reason":   "internal_error",
		"user_msg": "服务暂时不可用，请稍后再试",
	})
}
