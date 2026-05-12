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
	LLM      LLMConfig      `mapstructure:"llm"`
	Worker   WorkerConfig   `mapstructure:"worker"`
	TTS      TTSConfig      `mapstructure:"tts"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Audio    AudioConfig    `mapstructure:"audio"`
}

// AudioConfig holds ffmpeg + BGM mixing parameters.
type AudioConfig struct {
	FFmpegPath        string  `mapstructure:"ffmpeg_path"`         // default "ffmpeg" (PATH lookup)
	BGMCacheDir       string  `mapstructure:"bgm_cache_dir"`       // default "./cache/bgm"
	BGMVolumeDB       float64 `mapstructure:"bgm_volume_db"`       // default -18.0
	MixTimeoutSeconds int     `mapstructure:"mix_timeout_seconds"` // default 30
	OutputSampleRate  int     `mapstructure:"output_sample_rate"`  // default 32000
	OutputBitrateKbps int     `mapstructure:"output_bitrate_kbps"` // default 128
}

// TTSConfig holds TTS provider parameters.
type TTSConfig struct {
	Provider       string  `mapstructure:"provider"`        // "minimax" / "mock"
	BaseURL        string  `mapstructure:"base_url"`        // https://api.minimax.chat
	GroupID        string  `mapstructure:"group_id"`        // env AIBAO_TTS_MINIMAX_GROUP_ID
	APIKey         string  `mapstructure:"api_key"`         // env AIBAO_TTS_MINIMAX_API_KEY
	Model          string  `mapstructure:"model"`           // "speech-01-turbo"
	VoiceID        string  `mapstructure:"voice_id"`        // "female-tianmei"
	Format         string  `mapstructure:"format"`          // "mp3"
	SampleRate     int     `mapstructure:"sample_rate"`     // 32000
	Bitrate        int     `mapstructure:"bitrate"`         // 128000
	Speed          float64 `mapstructure:"speed"`           // 1.0
	TimeoutSeconds int     `mapstructure:"timeout_seconds"` // 60
}

// StorageConfig holds object storage provider parameters.
type StorageConfig struct {
	Provider         string `mapstructure:"provider"`               // "cos" / "mock"
	Bucket           string `mapstructure:"bucket"`                 // env AIBAO_STORAGE_COS_BUCKET
	Region           string `mapstructure:"region"`                 // ap-shanghai
	AppID            string `mapstructure:"app_id"`                 // env AIBAO_STORAGE_COS_APPID
	SecretID         string `mapstructure:"secret_id"`              // env AIBAO_STORAGE_COS_SECRET_ID
	SecretKey        string `mapstructure:"secret_key"`             // env AIBAO_STORAGE_COS_SECRET_KEY
	PresignedTTLSec  int    `mapstructure:"presigned_ttl_seconds"`  // 900 (15min)
	UploadTimeoutSec int    `mapstructure:"upload_timeout_seconds"` // 30
}

// LLMConfig holds LLM provider parameters.
type LLMConfig struct {
	Provider           string  `mapstructure:"provider"`              // "doubao" / "mock"
	StoryModel         string  `mapstructure:"story_model"`           // "doubao-1.5-pro-32k"
	IntentModel        string  `mapstructure:"intent_model"`          // "doubao-lite"
	APIKey             string  `mapstructure:"api_key"`               // env AIBAO_LLM_DOUBAO_API_KEY
	BaseURL            string  `mapstructure:"base_url"`              // doubao OpenAI-compatible endpoint
	TimeoutSeconds     int     `mapstructure:"timeout_seconds"`       // 30
	MaxRetries         int     `mapstructure:"max_retries"`           // 1
	StoryTemperature   float64 `mapstructure:"story_temperature"`     // 0.8
	IntentTemperature  float64 `mapstructure:"intent_temperature"`    // 0
	DailyBudgetYuan    float64 `mapstructure:"daily_budget_yuan"`     // 100.0
	PriceInputPerMTok  float64 `mapstructure:"price_input_per_mtok"`  // 0.8
	PriceOutputPerMTok float64 `mapstructure:"price_output_per_mtok"` // 2.0
	GenerateRPM        int     `mapstructure:"generate_rpm"`          // 5 / user / minute
}

// WorkerConfig holds outbox worker parameters.
type WorkerConfig struct {
	Enabled            bool `mapstructure:"enabled"`               // true
	PollIntervalSec    int  `mapstructure:"poll_interval_seconds"` // 5
	BatchSize          int  `mapstructure:"batch_size"`            // 10
	MaxAttempts        int  `mapstructure:"max_attempts"`          // 5
	BackoffBaseSeconds int  `mapstructure:"backoff_base_seconds"`  // 2
	BackoffMaxSeconds  int  `mapstructure:"backoff_max_seconds"`   // 600
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
		"llm.provider", "llm.story_model", "llm.intent_model", "llm.api_key",
		"llm.base_url", "llm.timeout_seconds", "llm.max_retries",
		"llm.story_temperature", "llm.intent_temperature",
		"llm.daily_budget_yuan", "llm.price_input_per_mtok", "llm.price_output_per_mtok",
		"llm.generate_rpm",
		"worker.enabled", "worker.poll_interval_seconds", "worker.batch_size",
		"worker.max_attempts", "worker.backoff_base_seconds", "worker.backoff_max_seconds",
		"tts.provider", "tts.base_url", "tts.group_id", "tts.api_key",
		"tts.model", "tts.voice_id", "tts.format", "tts.sample_rate",
		"tts.bitrate", "tts.speed", "tts.timeout_seconds",
		"storage.provider", "storage.bucket", "storage.region", "storage.app_id",
		"storage.secret_id", "storage.secret_key",
		"storage.presigned_ttl_seconds", "storage.upload_timeout_seconds",
		"audio.ffmpeg_path", "audio.bgm_cache_dir", "audio.bgm_volume_db",
		"audio.mix_timeout_seconds", "audio.output_sample_rate", "audio.output_bitrate_kbps",
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
	if c.LLM.Provider == "" {
		c.LLM.Provider = "doubao"
	}
	if c.LLM.Provider == "doubao" && c.LLM.APIKey == "" {
		return fmt.Errorf("config %s: llm.api_key is required (set AIBAO_LLM_API_KEY)", path)
	}
	if c.LLM.StoryModel == "" {
		c.LLM.StoryModel = "doubao-1.5-pro-32k"
	}
	if c.LLM.IntentModel == "" {
		c.LLM.IntentModel = "doubao-lite"
	}
	if c.LLM.BaseURL == "" {
		c.LLM.BaseURL = "https://ark.cn-beijing.volces.com/api/v3"
	}
	if c.LLM.TimeoutSeconds == 0 {
		c.LLM.TimeoutSeconds = 30
	}
	if c.LLM.MaxRetries == 0 {
		c.LLM.MaxRetries = 1
	}
	if c.LLM.StoryTemperature == 0 {
		c.LLM.StoryTemperature = 0.8
	}
	if c.LLM.DailyBudgetYuan == 0 {
		c.LLM.DailyBudgetYuan = 100.0
	}
	if c.LLM.PriceInputPerMTok == 0 {
		c.LLM.PriceInputPerMTok = 0.8
	}
	if c.LLM.PriceOutputPerMTok == 0 {
		c.LLM.PriceOutputPerMTok = 2.0
	}
	if c.LLM.GenerateRPM == 0 {
		c.LLM.GenerateRPM = 5
	}
	if c.Worker.PollIntervalSec == 0 {
		c.Worker.PollIntervalSec = 5
	}
	if c.Worker.BatchSize == 0 {
		c.Worker.BatchSize = 10
	}
	if c.Worker.MaxAttempts == 0 {
		c.Worker.MaxAttempts = 5
	}
	if c.Worker.BackoffBaseSeconds == 0 {
		c.Worker.BackoffBaseSeconds = 2
	}
	if c.Worker.BackoffMaxSeconds == 0 {
		c.Worker.BackoffMaxSeconds = 600
	}
	if c.TTS.Provider == "" {
		c.TTS.Provider = "minimax"
	}
	if c.TTS.Provider == "minimax" {
		if c.TTS.GroupID == "" {
			return fmt.Errorf("config %s: tts.group_id is required (set AIBAO_TTS_MINIMAX_GROUP_ID)", path)
		}
		if c.TTS.APIKey == "" {
			return fmt.Errorf("config %s: tts.api_key is required (set AIBAO_TTS_MINIMAX_API_KEY)", path)
		}
	}
	if c.TTS.BaseURL == "" {
		c.TTS.BaseURL = "https://api.minimax.chat"
	}
	if c.TTS.Model == "" {
		c.TTS.Model = "speech-01-turbo"
	}
	if c.TTS.VoiceID == "" {
		c.TTS.VoiceID = "female-tianmei" // TBD-confirm in Minimax console
	}
	if c.TTS.Format == "" {
		c.TTS.Format = "mp3"
	}
	if c.TTS.SampleRate == 0 {
		c.TTS.SampleRate = 32000
	}
	if c.TTS.Bitrate == 0 {
		c.TTS.Bitrate = 128000
	}
	if c.TTS.Speed == 0 {
		c.TTS.Speed = 1.0
	}
	if c.TTS.TimeoutSeconds == 0 {
		c.TTS.TimeoutSeconds = 60
	}

	if c.Storage.Provider == "" {
		c.Storage.Provider = "cos"
	}
	if c.Storage.Provider == "cos" {
		if c.Storage.Bucket == "" {
			return fmt.Errorf("config %s: storage.bucket is required (set AIBAO_STORAGE_COS_BUCKET)", path)
		}
		if c.Storage.Region == "" {
			return fmt.Errorf("config %s: storage.region is required", path)
		}
		if c.Storage.SecretID == "" {
			return fmt.Errorf("config %s: storage.secret_id is required (set AIBAO_STORAGE_COS_SECRET_ID)", path)
		}
		if c.Storage.SecretKey == "" {
			return fmt.Errorf("config %s: storage.secret_key is required (set AIBAO_STORAGE_COS_SECRET_KEY)", path)
		}
	}
	if c.Storage.PresignedTTLSec == 0 {
		c.Storage.PresignedTTLSec = 900
	}
	if c.Storage.UploadTimeoutSec == 0 {
		c.Storage.UploadTimeoutSec = 30
	}

	if c.Audio.FFmpegPath == "" {
		c.Audio.FFmpegPath = "ffmpeg"
	}
	if c.Audio.BGMCacheDir == "" {
		c.Audio.BGMCacheDir = "./cache/bgm"
	}
	if c.Audio.BGMVolumeDB == 0 {
		c.Audio.BGMVolumeDB = -18.0
	}
	if c.Audio.MixTimeoutSeconds == 0 {
		c.Audio.MixTimeoutSeconds = 30
	}
	if c.Audio.OutputSampleRate == 0 {
		c.Audio.OutputSampleRate = 32000
	}
	if c.Audio.OutputBitrateKbps == 0 {
		c.Audio.OutputBitrateKbps = 128
	}
	return nil
}
