package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// envPrefix is the environment variable prefix for config overrides.
// Example: server.port → AIBAO_SERVER_PORT, postgres.password → AIBAO_POSTGRES_PASSWORD.
const envPrefix = "AIBAO"

// Config is the root application configuration loaded from yaml + env.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Auth     AuthConfig     `mapstructure:"auth"`
	SMS      SMSConfig      `mapstructure:"sms"`
	Crypto   CryptoConfig   `mapstructure:"crypto"`
}

// AuthConfig holds JWT signing parameters.
type AuthConfig struct {
	JWTSecret         string `mapstructure:"jwt_secret"`          // env AIBAO_AUTH_JWT_SECRET
	AccessTTLMinutes  int    `mapstructure:"access_ttl_minutes"`  // 24h = 1440
	RefreshTTLMinutes int    `mapstructure:"refresh_ttl_minutes"` // 7d  = 10080
}

// SMSConfig holds SMS provider parameters. In MVP we only support "mock".
type SMSConfig struct {
	Provider          string `mapstructure:"provider"`            // "mock" / "tencent" (future)
	CodeTTLSeconds    int    `mapstructure:"code_ttl_seconds"`    // verification code lifetime
	ResendCooldownSec int    `mapstructure:"resend_cooldown_sec"` // per-phone send rate limit
}

// CryptoConfig holds at-rest encryption parameters.
type CryptoConfig struct {
	// PhoneAESKey is a 32-byte (hex-encoded 64-char) key for AES-256-GCM
	// encryption of phone numbers. From env AIBAO_CRYPTO_PHONE_AES_KEY.
	PhoneAESKey string `mapstructure:"phone_aes_key"`
	// SafehashSalt is the salt used for safehash.HashString of phone, child id,
	// etc. From env AIBAO_CRYPTO_SAFEHASH_SALT.
	SafehashSalt string `mapstructure:"safehash_salt"`
}

// ServerConfig holds HTTP server and logging settings.
type ServerConfig struct {
	Port     int    `mapstructure:"port"`
	LogDir   string `mapstructure:"log_dir"`
	LogLevel string `mapstructure:"log_level"`
}

// PostgresConfig holds Postgres connection settings.
type PostgresConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"` // from env AIBAO_POSTGRES_PASSWORD
	SSLMode  string `mapstructure:"sslmode"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"` // from env AIBAO_REDIS_PASSWORD
	DB       int    `mapstructure:"db"`
}

// Load reads config from file and overlays env vars (prefix envPrefix + "_").
// Env naming: AIBAO_SERVER_PORT, AIBAO_POSTGRES_PASSWORD, etc.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix(envPrefix)
	// AutomaticEnv alone does NOT make Unmarshal see env-only keys (e.g. postgres.password
	// when the field isn't present in the file). The BindEnv loop below registers each key
	// explicitly so Unmarshal will read it from env.
	v.AutomaticEnv()
	// Replace dots with underscores: viper key "postgres.password" → env "POSTGRES_PASSWORD".
	// Combined with the prefix, the final env var becomes AIBAO_POSTGRES_PASSWORD.
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	// Explicitly bind env vars for fields not present in the config file,
	// so AutomaticEnv picks them up during Unmarshal.
	for _, key := range []string{
		"server.port", "server.log_dir", "server.log_level",
		"postgres.host", "postgres.port", "postgres.database",
		"postgres.user", "postgres.password", "postgres.sslmode",
		"redis.addr", "redis.password", "redis.db",
		"auth.jwt_secret", "auth.access_ttl_minutes", "auth.refresh_ttl_minutes",
		"sms.provider", "sms.code_ttl_seconds", "sms.resend_cooldown_sec",
		"crypto.phone_aes_key", "crypto.safehash_salt",
	} {
		_ = v.BindEnv(key)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := applyDefaultsAndValidate(&cfg, path); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaultsAndValidate(c *Config, path string) error {
	if c.Server.Port == 0 {
		return fmt.Errorf("config %s: server.port is required", path)
	}
	if c.Postgres.Host == "" {
		return fmt.Errorf("config %s: postgres.host is required", path)
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("config %s: redis.addr is required", path)
	}
	if c.Server.LogLevel == "" {
		c.Server.LogLevel = "info"
	}
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("config %s: auth.jwt_secret is required (set AIBAO_AUTH_JWT_SECRET)", path)
	}
	if c.Auth.AccessTTLMinutes == 0 {
		c.Auth.AccessTTLMinutes = 24 * 60
	}
	if c.Auth.RefreshTTLMinutes == 0 {
		c.Auth.RefreshTTLMinutes = 7 * 24 * 60
	}
	if c.SMS.Provider == "" {
		c.SMS.Provider = "mock"
	}
	if c.SMS.CodeTTLSeconds == 0 {
		c.SMS.CodeTTLSeconds = 300
	}
	if c.SMS.ResendCooldownSec == 0 {
		c.SMS.ResendCooldownSec = 60
	}
	if c.Crypto.PhoneAESKey == "" {
		return fmt.Errorf("config %s: crypto.phone_aes_key is required (set AIBAO_CRYPTO_PHONE_AES_KEY, 64 hex chars)", path)
	}
	if c.Crypto.SafehashSalt == "" {
		return fmt.Errorf("config %s: crypto.safehash_salt is required (set AIBAO_CRYPTO_SAFEHASH_SALT)", path)
	}
	return nil
}
