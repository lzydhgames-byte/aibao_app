package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/aibao/server/internal/pkg/traceid"
)

var (
	defaultMu sync.RWMutex
	def       *slog.Logger = slog.Default()
)

// NewWithWriter creates a JSON slog.Logger that writes to w at the given level.
// Used in tests with a *bytes.Buffer for assertions, or in main with a multi-writer.
func NewWithWriter(w io.Writer, level string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: parseLevel(level),
	})
	return slog.New(h)
}

// NewFromConfig creates a logger writing to <dir>/app.log with rotation
// (lumberjack: 100MB max size per file, 14-day retention, gzipped backups)
// and also stderr for dev visibility. Returns the logger and a closer to flush
// on shutdown.
func NewFromConfig(logDir, level string) (*slog.Logger, func() error, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("mkdir log dir: %w", err)
	}

	rot := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "app.log"),
		MaxSize:    100, // MB
		MaxAge:     14,  // days
		MaxBackups: 14,
		Compress:   true,
	}

	mw := io.MultiWriter(rot, os.Stderr)
	lg := NewWithWriter(mw, level)

	return lg, rot.Close, nil
}

// SetDefault replaces the package-level default logger. Safe for concurrent use.
func SetDefault(l *slog.Logger) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	def = l
}

// Default returns the current package-level default logger.
func Default() *slog.Logger {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return def
}

// FromCtx returns the default logger pre-populated with trace_id (if present in ctx).
func FromCtx(ctx context.Context) *slog.Logger {
	lg := Default()
	if id, ok := traceid.FromContext(ctx); ok {
		return lg.With("trace_id", id)
	}
	return lg
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
