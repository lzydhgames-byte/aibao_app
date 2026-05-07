package sms

import (
	"context"

	"github.com/aibao/server/internal/pkg/logger"
)

// MockSender is the dev/test SMS sender. It logs the (phone, code) pair and
// always reports success. Use FixedCode() to learn the constant code that the
// auth service should expect when SMS.Provider == "mock".
type MockSender struct{}

// NewMock constructs a MockSender.
func NewMock() *MockSender { return &MockSender{} }

// FixedCode is the verification code that the mock provider always uses.
const fixedCode = "123456"

// FixedCode returns the constant verification code emitted by NewMock.
func (m *MockSender) FixedCode() string { return fixedCode }

// SendCode logs the message and returns nil. Phone is masked in the log.
func (m *MockSender) SendCode(ctx context.Context, phone, code string) error {
	logger.FromCtx(ctx).Info("sms.mock.send",
		"phone", logger.MaskPhone(phone),
		"code", code,
	)
	return nil
}
