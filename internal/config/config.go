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

	// Bucket scanner
	ScannerEnabled      bool
	ScannerIntervalSec  int
	ScannerBucket       string
	ScannerInputPrefix  string
	ScannerOutputPrefix string
	ScannerPriority     int
	HLSVideoCodec       string
	HLSVideoBitrate     string
	HLSWidth            int
	HLSHeight           int
	HLSFramerate        int
	HLSAudioCodec       string
	HLSAudioBitrate     string
	HLSSegmentSeconds   int
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

		ScannerEnabled:      getEnvBool("SCANNER_ENABLED", false),
		ScannerIntervalSec:  getEnvInt("SCANNER_INTERVAL_SECONDS", 60),
		ScannerBucket:       getEnv("SCANNER_BUCKET", ""),
		ScannerInputPrefix:  strings.Trim(strings.TrimPrefix(getEnv("SCANNER_INPUT_PREFIX", ""), "/"), " "),
		ScannerOutputPrefix: strings.Trim(strings.Trim(getEnv("SCANNER_OUTPUT_PREFIX", "hls"), "/"), " "),
		ScannerPriority:     getEnvInt("SCANNER_PRIORITY", 5),
		HLSVideoCodec:       getEnv("HLS_VIDEO_CODEC", "libx264"),
		HLSVideoBitrate:     getEnv("HLS_VIDEO_BITRATE", "2500k"),
		HLSWidth:            getEnvInt("HLS_WIDTH", 1280),
		HLSHeight:           getEnvInt("HLS_HEIGHT", 720),
		HLSFramerate:        getEnvInt("HLS_FRAMERATE", 30),
		HLSAudioCodec:       getEnv("HLS_AUDIO_CODEC", "aac"),
		HLSAudioBitrate:     getEnv("HLS_AUDIO_BITRATE", "128k"),
		HLSSegmentSeconds:   getEnvInt("HLS_SEGMENT_SECONDS", 6),
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

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}
