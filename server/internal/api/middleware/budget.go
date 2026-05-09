package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/aibao/server/internal/gateway/llm"
	"github.com/aibao/server/internal/pkg/logger"
)

// BudgetChecker is the minimal surface BudgetGuard needs.
type BudgetChecker interface {
	PreCheck(ctx context.Context) error
}

// BudgetGuard refuses requests with 503 when daily LLM budget is exhausted.
// On Redis errors it allows through (don't compound an outage).
func BudgetGuard(b BudgetChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := b.PreCheck(c.Request.Context())
		if err == nil {
			c.Next()
			return
		}
		if errors.Is(err, llm.ErrBudgetExceeded) {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"reason":   "budget_exceeded",
				"user_msg": "今日额度已用完，请明天再来",
			})
			return
		}
		logger.FromCtx(c.Request.Context()).Warn("budget.guard.error", "err", err.Error())
		c.Next()
	}
}
