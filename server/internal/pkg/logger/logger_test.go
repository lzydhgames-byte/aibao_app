package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aibao/server/internal/pkg/traceid"
)

func TestLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	lg := NewWithWriter(&buf, "debug")
	lg.Info("hello", "k", "v")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "hello", entry["msg"])
	assert.Equal(t, "v", entry["k"])
	assert.NotEmpty(t, entry["time"])
}

func TestFromCtx_InjectsTraceID(t *testing.T) {
	var buf bytes.Buffer
	base := NewWithWriter(&buf, "debug")
	SetDefault(base)

	ctx := traceid.WithID(context.Background(), "tr-xyz")
	FromCtx(ctx).Info("evt")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "tr-xyz", entry["trace_id"])
}

func TestFromCtx_NoTraceID(t *testing.T) {
	var buf bytes.Buffer
	base := NewWithWriter(&buf, "debug")
	SetDefault(base)

	FromCtx(context.Background()).Info("evt")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	_, has := entry["trace_id"]
	assert.False(t, has, "no trace_id when ctx has none")
}

func TestNewFromConfig_FileOutput(t *testing.T) {
	dir := t.TempDir()
	lg, closer, err := NewFromConfig(dir, "info")
	require.NoError(t, err)
	defer func() { _ = closer() }()

	lg.Info("hello")

	files, err := filepath.Glob(filepath.Join(dir, "*.log"))
	require.NoError(t, err)
	assert.NotEmpty(t, files, "log file should be created under log_dir")
}

func TestLevel_FilteredCorrectly(t *testing.T) {
	var buf bytes.Buffer
	lg := NewWithWriter(&buf, "warn")
	lg.Debug("nope")
	lg.Info("nope")
	lg.Warn("yep")
	assert.NotContains(t, buf.String(), "nope")
	assert.Contains(t, buf.String(), "yep")
}
