package database

import (
	"encoding/json"
	"time"
)

type JobStatus string

const (
	JobStatusPending     JobStatus = "pending"
	JobStatusDownloading JobStatus = "downloading"
	JobStatusTranscoding JobStatus = "transcoding"
	JobStatusUploading   JobStatus = "uploading"
	JobStatusCompleted   JobStatus = "completed"
	JobStatusFailed      JobStatus = "failed"
	JobStatusCancelled   JobStatus = "cancelled"
)

type Job struct {
	ID           string          `json:"id"`
	Status       JobStatus       `json:"status"`
	Progress     float32         `json:"progress"`
	Speed        string          `json:"speed"`
	FPS          float32         `json:"fps"`
	ETA          string          `json:"eta"`
	InputConfig  json.RawMessage `json:"input"`
	OutputConfig json.RawMessage `json:"output"`
	Settings     json.RawMessage `json:"settings"`
	Priority     int             `json:"priority"`
	WebhookURL   string          `json:"webhook_url,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	InputInfo    json.RawMessage `json:"input_info,omitempty"`
	OutputInfo   json.RawMessage `json:"output_info,omitempty"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	DurationMs   int64           `json:"duration_ms"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type Preset struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Settings    json.RawMessage `json:"settings"`
	IsSystem    bool            `json:"is_system"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// API request/response types

type StorageConfig struct {
	Type        string          `json:"type"`
	URL         string          `json:"url"`
	Credentials json.RawMessage `json:"credentials,omitempty"`
}

type VideoSettings struct {
	Codec       string `json:"codec,omitempty"`
	Bitrate     string `json:"bitrate,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Framerate   int    `json:"framerate,omitempty"`
	Profile     string `json:"profile,omitempty"`
	PixelFormat string `json:"pixel_format,omitempty"`
}

type AudioSettings struct {
	Codec      string `json:"codec,omitempty"`
	Bitrate    string `json:"bitrate,omitempty"`
	Channels   int    `json:"channels,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
}

type TranscodeSettings struct {
	PresetID      string        `json:"preset_id,omitempty"`
	Video         VideoSettings `json:"video,omitempty"`
	Audio         AudioSettings `json:"audio,omitempty"`
	Format        string        `json:"format,omitempty"`
	HardwareAccel string        `json:"hardware_accel,omitempty"`
	ExtraFlags    []string      `json:"extra_flags,omitempty"`
}

type CreateJobRequest struct {
	Input      StorageConfig     `json:"input" binding:"required"`
	Output     StorageConfig     `json:"output" binding:"required"`
	Settings   TranscodeSettings `json:"settings" binding:"required"`
	Priority   int               `json:"priority"`
	WebhookURL string            `json:"webhook_url"`
	Metadata   json.RawMessage   `json:"metadata"`
}

type CreatePresetRequest struct {
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description"`
	Settings    json.RawMessage `json:"settings" binding:"required"`
}

type UpdatePresetRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Settings    json.RawMessage `json:"settings"`
}

type ProbeRequest struct {
	Input StorageConfig `json:"input" binding:"required"`
}

type JobListParams struct {
	Status string
	Limit  int
	Offset int
}

type AppSettings struct {
	ScannerEnabled        bool   `json:"scanner_enabled"`
	ScannerIntervalSec    int    `json:"scanner_interval_seconds"`
	ScannerBucket         string `json:"scanner_bucket"`
	ScannerInputPrefix    string `json:"scanner_input_prefix"`
	ScannerOutputPrefix   string `json:"scanner_output_prefix"`
	ScannerOutputTemplate string `json:"scanner_output_template"`
	ScannerPriority       int    `json:"scanner_priority"`
	S3Region              string `json:"s3_region"`
	S3AccessKey           string `json:"s3_access_key"`
	S3SecretConfigured    bool   `json:"s3_secret_configured"`
	S3Endpoint            string `json:"s3_endpoint"`
	HLSVideoCodec         string `json:"hls_video_codec"`
	HLSVideoBitrate       string `json:"hls_video_bitrate"`
	HLSWidth              int    `json:"hls_width"`
	HLSHeight             int    `json:"hls_height"`
	HLSFramerate          int    `json:"hls_framerate"`
	HLSAudioCodec         string `json:"hls_audio_codec"`
	HLSAudioBitrate       string `json:"hls_audio_bitrate"`
	HLSSegmentSeconds     int    `json:"hls_segment_seconds"`
}

type UpdateAppSettingsRequest struct {
	ScannerEnabled        bool   `json:"scanner_enabled"`
	ScannerIntervalSec    int    `json:"scanner_interval_seconds"`
	ScannerBucket         string `json:"scanner_bucket"`
	ScannerInputPrefix    string `json:"scanner_input_prefix"`
	ScannerOutputPrefix   string `json:"scanner_output_prefix"`
	ScannerOutputTemplate string `json:"scanner_output_template"`
	ScannerPriority       int    `json:"scanner_priority"`
	S3Region              string `json:"s3_region"`
	S3AccessKey           string `json:"s3_access_key"`
	S3SecretKey           string `json:"s3_secret_key"`
	S3Endpoint            string `json:"s3_endpoint"`
	HLSVideoCodec         string `json:"hls_video_codec"`
	HLSVideoBitrate       string `json:"hls_video_bitrate"`
	HLSWidth              int    `json:"hls_width"`
	HLSHeight             int    `json:"hls_height"`
	HLSFramerate          int    `json:"hls_framerate"`
	HLSAudioCodec         string `json:"hls_audio_codec"`
	HLSAudioBitrate       string `json:"hls_audio_bitrate"`
	HLSSegmentSeconds     int    `json:"hls_segment_seconds"`
}
