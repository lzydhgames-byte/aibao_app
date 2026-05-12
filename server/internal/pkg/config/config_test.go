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
auth:
  jwt_secret: dev-secret
  access_ttl_minutes: 1440
  refresh_ttl_minutes: 10080
sms:
  provider: mock
  code_ttl_seconds: 300
  resend_cooldown_sec: 60
crypto:
  phone_aes_key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  safehash_salt: dev-salt
llm:
  provider: mock
  story_model: doubao-1.5-pro-32k
  intent_model: doubao-lite
  api_key: dev-key
  base_url: https://example.com
worker:
  enabled: true
tts:
  provider: mock
  group_id: dev-gid
  api_key: dev-tts-key
storage:
  provider: mock
  bucket: dev-bucket
  region: ap-shanghai
  secret_id: dev-sid
  secret_key: dev-skey
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
auth:
  jwt_secret: x
crypto:
  phone_aes_key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  safehash_salt: salt
llm:
  api_key: dev-key
tts:
  group_id: g
  api_key: k
storage:
  bucket: b
  region: ap-shanghai
  secret_id: s
  secret_key: k
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "info", cfg.Server.LogLevel, "LogLevel should default to info when empty")
}

func TestLoad_AuthAndSMSDefaults(t *testing.T) {
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
auth:
  jwt_secret: x
crypto:
  phone_aes_key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  safehash_salt: salt
llm:
  api_key: dev-key
tts:
  group_id: g
  api_key: k
storage:
  bucket: b
  region: ap-shanghai
  secret_id: s
  secret_key: k
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 1440, cfg.Auth.AccessTTLMinutes)
	assert.Equal(t, 10080, cfg.Auth.RefreshTTLMinutes)
	assert.Equal(t, "mock", cfg.SMS.Provider)
	assert.Equal(t, 300, cfg.SMS.CodeTTLSeconds)
	assert.Equal(t, 60, cfg.SMS.ResendCooldownSec)
}

func TestLoad_MissingJWTSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
server:
  port: 8080
postgres:
  host: 127.0.0.1
  port: 5432
  database: aibao
  user: aibao
redis:
  addr: 127.0.0.1:6379
crypto:
  phone_aes_key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  safehash_salt: salt
llm:
  api_key: dev-key
tts:
  group_id: g
  api_key: k
storage:
  bucket: b
  region: ap-shanghai
  secret_id: s
  secret_key: k
`), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth.jwt_secret")
}

func TestLoad_LLMDefaults(t *testing.T) {
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
auth:
  jwt_secret: x
crypto:
  phone_aes_key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  safehash_salt: salt
llm:
  api_key: dev-key
tts:
  group_id: g
  api_key: k
storage:
  bucket: b
  region: ap-shanghai
  secret_id: s
  secret_key: k
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "doubao", cfg.LLM.Provider)
	assert.Equal(t, "doubao-1.5-pro-32k", cfg.LLM.StoryModel)
	assert.Equal(t, "doubao-lite", cfg.LLM.IntentModel)
	assert.InDelta(t, 0.8, cfg.LLM.StoryTemperature, 0.001)
	assert.InDelta(t, 100.0, cfg.LLM.DailyBudgetYuan, 0.001)
	assert.Equal(t, 5, cfg.LLM.GenerateRPM)
	assert.Equal(t, 5, cfg.Worker.PollIntervalSec)
	assert.Equal(t, 10, cfg.Worker.BatchSize)
}

func TestLoad_TTSStorageDefaults(t *testing.T) {
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
auth:
  jwt_secret: x
crypto:
  phone_aes_key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  safehash_salt: salt
llm:
  api_key: dev-key
tts:
  group_id: g
  api_key: k
storage:
  bucket: b
  region: ap-shanghai
  secret_id: s
  secret_key: k
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "minimax", cfg.TTS.Provider)
	assert.Equal(t, "speech-01-turbo", cfg.TTS.Model)
	assert.Equal(t, "female-tianmei", cfg.TTS.VoiceID)
	assert.Equal(t, "mp3", cfg.TTS.Format)
	assert.Equal(t, 32000, cfg.TTS.SampleRate)
	assert.InDelta(t, 1.0, cfg.TTS.Speed, 0.001)
	assert.Equal(t, "cos", cfg.Storage.Provider)
	assert.Equal(t, 900, cfg.Storage.PresignedTTLSec)
}

func TestLoad_AudioDefaults(t *testing.T) {
	path := writeValidConfig(t)
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "ffmpeg", cfg.Audio.FFmpegPath)
	assert.Equal(t, "./cache/bgm", cfg.Audio.BGMCacheDir)
	assert.InDelta(t, -18.0, cfg.Audio.BGMVolumeDB, 0.001)
	assert.Equal(t, 30, cfg.Audio.MixTimeoutSeconds)
	assert.Equal(t, 32000, cfg.Audio.OutputSampleRate)
	assert.Equal(t, 128, cfg.Audio.OutputBitrateKbps)
}

func TestLoad_TTSMissingKey(t *testing.T) {
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
auth:
  jwt_secret: x
crypto:
  phone_aes_key: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  safehash_salt: salt
llm:
  api_key: dev-key
storage:
  bucket: b
  region: ap-shanghai
  secret_id: s
  secret_key: k
`), 0o600))
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tts.group_id")
}

func TestLoad_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("this is: : not valid: yaml:\n  -]\n"), 0o600))

	_, err := Load(path)
	assert.Error(t, err)
}
