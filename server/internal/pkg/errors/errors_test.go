package errors

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_FieldsSet(t *testing.T) {
	e := New(CodeNotFound, "child_not_found", "未找到孩子档案")
	assert.Equal(t, CodeNotFound, e.Code)
	assert.Equal(t, "child_not_found", e.Reason)
	assert.Equal(t, "未找到孩子档案", e.UserMsg)
	assert.Equal(t, http.StatusNotFound, e.HTTPStatus())
}

func TestWrap_PreservesCause(t *testing.T) {
	cause := errors.New("db connection refused")
	e := Wrap(cause, CodeInternal, "db_error", "服务暂时不可用")
	assert.True(t, errors.Is(e, cause))
}

func TestAsAppError(t *testing.T) {
	e := New(CodeInvalidArgument, "bad_input", "参数错误")
	got, ok := AsAppError(e)
	assert.True(t, ok)
	assert.Equal(t, CodeInvalidArgument, got.Code)

	plain := errors.New("plain")
	_, ok = AsAppError(plain)
	assert.False(t, ok)
}

func TestHTTPStatus_AllCodes(t *testing.T) {
	cases := map[Code]int{
		CodeInvalidArgument:  http.StatusBadRequest,
		CodeUnauthenticated:  http.StatusUnauthorized,
		CodePermissionDenied: http.StatusForbidden,
		CodeNotFound:         http.StatusNotFound,
		CodeRateLimited:      http.StatusTooManyRequests,
		CodeBudgetExceeded:   http.StatusServiceUnavailable,
		CodeInternal:         http.StatusInternalServerError,
	}
	for c, want := range cases {
		assert.Equal(t, want, New(c, "x", "y").HTTPStatus(), "code=%v", c)
	}
}
