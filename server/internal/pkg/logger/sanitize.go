// Package logger provides structured logging primitives plus sanitize helpers
// for stripping sensitive content from log fields.
package logger

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaskPhone masks middle 4 digits of a Chinese mobile phone number.
// Returns "138****8000" for "13800138000" and "+8613800138000",
// "***" for too-short input, and "" for empty input.
func MaskPhone(phone string) string {
	p := strings.TrimPrefix(phone, "+86")
	if p == "" {
		return ""
	}
	if len(p) < 7 {
		return "***"
	}
	return p[:3] + "****" + p[len(p)-4:]
}

// RedactPromptText keeps only the rune length, never the content.
// Used to log "we received user input of length N" without leaking the prompt.
func RedactPromptText(s string) string {
	return fmt.Sprintf("len=%d", utf8.RuneCountInString(s))
}
