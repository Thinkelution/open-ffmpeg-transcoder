package appsettings

import (
	"context"
	"strconv"
	"strings"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
)

type RuntimeSettings struct {
	database.AppSettings
	S3SecretKey string
}

func Load(ctx context.Context, db *database.DB, cfg *config.Config) (RuntimeSettings, error) {
	values, err := db.GetSettingsMap(ctx)
	if err != nil {
		return RuntimeSettings{}, err
	}

	secret := database.StringSetting(values, "s3_secret_key", cfg.S3SecretKey)
	settings := RuntimeSettings{
		AppSettings: database.AppSettings{
			ScannerEnabled:        database.BoolSetting(values, "scanner_enabled", cfg.ScannerEnabled),
			ScannerIntervalSec:    database.IntSetting(values, "scanner_interval_seconds", cfg.ScannerIntervalSec),
			ScannerBucket:         database.StringSetting(values, "scanner_bucket", cfg.ScannerBucket),
			ScannerInputPrefix:    cleanPrefix(database.StringSetting(values, "scanner_input_prefix", cfg.ScannerInputPrefix)),
			ScannerOutputPrefix:   cleanPrefix(database.StringSetting(values, "scanner_output_prefix", cfg.ScannerOutputPrefix)),
			ScannerOutputTemplate: strings.TrimSpace(database.StringSetting(values, "scanner_output_template", cfg.ScannerOutputTemplate)),
			ScannerPriority:       database.IntSetting(values, "scanner_priority", cfg.ScannerPriority),
			S3Region:              database.StringSetting(values, "s3_region", cfg.S3Region),
			S3AccessKey:           database.StringSetting(values, "s3_access_key", cfg.S3AccessKey),
			S3SecretConfigured:    secret != "",
			S3Endpoint:            database.StringSetting(values, "s3_endpoint", cfg.S3Endpoint),
			HLSVideoCodec:         database.StringSetting(values, "hls_video_codec", cfg.HLSVideoCodec),
			HLSVideoBitrate:       database.StringSetting(values, "hls_video_bitrate", cfg.HLSVideoBitrate),
			HLSWidth:              database.IntSetting(values, "hls_width", cfg.HLSWidth),
			HLSHeight:             database.IntSetting(values, "hls_height", cfg.HLSHeight),
			HLSFramerate:          database.IntSetting(values, "hls_framerate", cfg.HLSFramerate),
			HLSAudioCodec:         database.StringSetting(values, "hls_audio_codec", cfg.HLSAudioCodec),
			HLSAudioBitrate:       database.StringSetting(values, "hls_audio_bitrate", cfg.HLSAudioBitrate),
			HLSSegmentSeconds:     database.IntSetting(values, "hls_segment_seconds", cfg.HLSSegmentSeconds),
		},
		S3SecretKey: secret,
	}

	if settings.ScannerIntervalSec <= 0 {
		settings.ScannerIntervalSec = 60
	}
	if settings.ScannerOutputPrefix == "" {
		settings.ScannerOutputPrefix = "hls"
	}
	if settings.ScannerPriority <= 0 {
		settings.ScannerPriority = 5
	}
	if settings.HLSWidth <= 0 {
		settings.HLSWidth = 1280
	}
	if settings.HLSHeight <= 0 {
		settings.HLSHeight = 720
	}
	if settings.HLSFramerate <= 0 {
		settings.HLSFramerate = 30
	}
	if settings.HLSSegmentSeconds <= 0 {
		settings.HLSSegmentSeconds = 6
	}

	return settings, nil
}

func Save(ctx context.Context, db *database.DB, req database.UpdateAppSettingsRequest) error {
	values := map[string]string{
		"scanner_enabled":          strconv.FormatBool(req.ScannerEnabled),
		"scanner_interval_seconds": strconv.Itoa(defaultInt(req.ScannerIntervalSec, 60)),
		"scanner_bucket":           strings.TrimSpace(req.ScannerBucket),
		"scanner_input_prefix":     cleanPrefix(req.ScannerInputPrefix),
		"scanner_output_prefix":    cleanPrefix(req.ScannerOutputPrefix),
		"scanner_output_template":  strings.TrimSpace(req.ScannerOutputTemplate),
		"scanner_priority":         strconv.Itoa(defaultInt(req.ScannerPriority, 5)),
		"s3_region":                strings.TrimSpace(req.S3Region),
		"s3_access_key":            strings.TrimSpace(req.S3AccessKey),
		"s3_endpoint":              strings.TrimSpace(req.S3Endpoint),
		"hls_video_codec":          strings.TrimSpace(req.HLSVideoCodec),
		"hls_video_bitrate":        strings.TrimSpace(req.HLSVideoBitrate),
		"hls_width":                strconv.Itoa(defaultInt(req.HLSWidth, 1280)),
		"hls_height":               strconv.Itoa(defaultInt(req.HLSHeight, 720)),
		"hls_framerate":            strconv.Itoa(defaultInt(req.HLSFramerate, 30)),
		"hls_audio_codec":          strings.TrimSpace(req.HLSAudioCodec),
		"hls_audio_bitrate":        strings.TrimSpace(req.HLSAudioBitrate),
		"hls_segment_seconds":      strconv.Itoa(defaultInt(req.HLSSegmentSeconds, 6)),
	}
	if req.S3SecretKey != "" {
		values["s3_secret_key"] = req.S3SecretKey
	}
	return db.UpdateSettingsMap(ctx, values)
}

func cleanPrefix(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "/")
	if value == "." {
		return ""
	}
	return value
}

func defaultInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
