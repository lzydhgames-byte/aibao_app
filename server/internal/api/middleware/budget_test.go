package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/aibao/server/internal/gateway/llm"
)

type fakeBudgetCheck struct {
	allow bool
	err   error
}

func (f *fakeBudgetCheck) PreCheck(_ context.Context) error {
	if f.err != nil {
		return f.err
	}
	if !f.allow {
		return llm.ErrBudgetExceeded
	}
	return nil
}

func TestBudget_AllowsWhenUnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BudgetGuard(&fakeBudgetCheck{allow: true}))
	r.POST("/x", func(c *gin.Context) { c.Status(200) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, 200, rec.Code)
}

func TestBudget_RejectsBudgetExceeded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BudgetGuard(&fakeBudgetCheck{allow: false}))
	r.POST("/x", func(c *gin.Context) { c.Status(200) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "budget_exceeded")
}

func TestBudget_AllowsOnRedisError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BudgetGuard(&fakeBudgetCheck{err: errors.New("redis down")}))
	r.POST("/x", func(c *gin.Context) { c.Status(200) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.ServeHTTP(rec, req)
	assert.Equal(t, 200, rec.Code)
}
