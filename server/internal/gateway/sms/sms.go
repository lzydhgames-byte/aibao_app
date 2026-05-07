// Package sms abstracts SMS providers behind a Sender interface. The MVP only
// ships a mock implementation that logs the message and uses a fixed code; a
// future Tencent Cloud SMS implementation will plug in via the same interface.
package sms

import "context"

// Sender sends a verification code to a phone number.
type Sender interface {
	SendCode(ctx context.Context, phone, code string) error
}
