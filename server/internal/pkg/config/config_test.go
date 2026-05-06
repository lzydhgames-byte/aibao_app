package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeValidConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
server:
  port: 8080
  log_dir: /tmp/aibao
  log_level: info
postgres:
  host: 127.0.0.1
  port: 5432
  database: aibao
  user: aibao
redis:
  addr: 127.0.0.1:6379
`), 0o600))
	return path
}

func TestLoad_FromFile(t *testing.T) {
	path := writeValidConfig(t)

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "/tmp/aibao", cfg.Server.LogDir)
	assert.Equal(t, "info", cfg.Server.LogLevel)
	assert.Equal(t, "127.0.0.1", cfg.Postgres.Host)
	assert.Equal(t, "127.0.0.1:6379", cfg.Redis.Addr)
}

func TestLoad_EnvOverride(t *testing.T) {
	path := writeValidConfig(t)

	t.Setenv("AIBAO_POSTGRES_PASSWORD", "secret")
	t.Setenv("AIBAO_SERVER_PORT", "9090")

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "secret", cfg.Postgres.Password)
	assert.Equal(t, 9090, cfg.Server.Port, "env should override file")
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/no/such/file.yaml")
	assert.Error(t, err)
}

func TestLoad_MissingRequired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`server:
  port: 8080
`), 0o600))
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "postgres.host")
}

func TestLoad_LogLevelDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
server:
  port: 8080
  log_dir: /tmp/aibao
postgres:
  host: 127.0.0.1
  port: 5432
  database: aibao
  user: aibao
redis:
  addr: 127.0.0.1:6379
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "info", cfg.Server.LogLevel, "LogLevel should default to info when empty")
}

func TestLoad_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("this is: : not valid: yaml:\n  -]\n"), 0o600))

	_, err := Load(path)
	assert.Error(t, err)
}
