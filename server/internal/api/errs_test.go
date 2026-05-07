package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	apperr "github.com/aibao/server/internal/pkg/errors"
)

func TestRespondError_AppError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	RespondError(c, apperr.New(apperr.CodeNotFound, "child_not_found", "未找到孩子档案"))

	assert.Equal(t, 404, rec.Code)
	assert.Contains(t, rec.Body.String(), "child_not_found")
	assert.Contains(t, rec.Body.String(), "未找到孩子档案")
}

func TestRespondError_PlainError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	RespondError(c, errors.New("boom"))

	assert.Equal(t, 500, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal_error")
}
