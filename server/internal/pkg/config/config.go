package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config is the root application configuration loaded from yaml + env.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
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

// Load reads config from file and overlays env vars (prefix AIBAO_).
// Env naming: AIBAO_SERVER_PORT, AIBAO_POSTGRES_PASSWORD, etc.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("AIBAO")
	v.AutomaticEnv()
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
	} {
		_ = v.BindEnv(key)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func validate(c *Config) error {
	if c.Server.Port == 0 {
		return fmt.Errorf("server.port is required")
	}
	if c.Postgres.Host == "" {
		return fmt.Errorf("postgres.host is required")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr is required")
	}
	if c.Server.LogLevel == "" {
		c.Server.LogLevel = "info"
	}
	return nil
}
