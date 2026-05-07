package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/pkg/logger"
)

func TestMockSender_SendsFixedCodeAndLogs(t *testing.T) {
	var buf bytes.Buffer
	logger.SetDefault(logger.NewWithWriter(&buf, "debug"))

	m := NewMock()
	require.Equal(t, "123456", m.FixedCode())

	err := m.SendCode(context.Background(), "13800138000", "123456")
	require.NoError(t, err)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimRight(buf.Bytes(), "\n"), &entry))
	assert.Equal(t, "sms.mock.send", entry["msg"])
	// phone must be masked
	assert.Equal(t, "138****8000", entry["phone"])
	// code present so the dev can read it
	assert.Equal(t, "123456", entry["code"])
}

func TestMockSender_ImplementsSenderInterface(t *testing.T) {
	var s Sender = NewMock()
	assert.NotNil(t, s)
}
