package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Mode         string // "serve", "worker", "all"
	Port         string
	DatabaseURL  string
	RedisAddr    string
	RedisPass    string
	APIKey       string
	TempDir      string
	MaxWorkers   int
	FFmpegPath   string
	FFprobePath  string
	LogLevel     string
	WebhookRetry int

	// S3 defaults (can be overridden per-job)
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3Endpoint  string
}

func Load() *Config {
	return &Config{
		Mode:         getEnv("MODE", "all"),
		Port:         getEnv("PORT", "8090"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://transcoder:transcoder@localhost:5432/transcoder?sslmode=disable"),
		RedisAddr:    getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPass:    getEnv("REDIS_PASSWORD", ""),
		APIKey:       getEnv("API_KEY", ""),
		TempDir:      getEnv("TEMP_DIR", "/tmp/transcoder"),
		MaxWorkers:   getEnvInt("MAX_WORKERS", 2),
		FFmpegPath:   getEnv("FFMPEG_PATH", "ffmpeg"),
		FFprobePath:  getEnv("FFPROBE_PATH", "ffprobe"),
		LogLevel:     strings.ToLower(getEnv("LOG_LEVEL", "info")),
		WebhookRetry: getEnvInt("WEBHOOK_RETRY", 3),
		S3Region:     getEnv("S3_REGION", "us-east-1"),
		S3AccessKey:  getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey:  getEnv("S3_SECRET_KEY", ""),
		S3Endpoint:   getEnv("S3_ENDPOINT", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
