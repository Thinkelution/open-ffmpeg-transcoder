package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/hibiken/asynq"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/notify"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/storage"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/transcoder"
)

type Handler struct {
	ctx context.Context
	cfg *config.Config
	db  *database.DB
	ff  *transcoder.FFmpeg
}

func NewHandler(ctx context.Context, cfg *config.Config, db *database.DB, ff *transcoder.FFmpeg) *Handler {
	return &Handler{ctx: ctx, cfg: cfg, db: db, ff: ff}
}

func (h *Handler) HandleTranscode(_ context.Context, task *asynq.Task) error {
	jobID := string(task.Payload())
	log.Printf("[handler] Processing job %s", jobID)

	job, err := h.db.GetJob(h.ctx, jobID)
	if err != nil || job == nil {
		return fmt.Errorf("job %s not found: %w", jobID, err)
	}

	if job.Status == database.JobStatusCancelled {
		log.Printf("[handler] Job %s was cancelled, skipping", jobID)
		return nil
	}

	// Parse configs
	var inputCfg database.StorageConfig
	var outputCfg database.StorageConfig
	var settings database.TranscodeSettings

	json.Unmarshal(job.InputConfig, &inputCfg)
	json.Unmarshal(job.OutputConfig, &outputCfg)
	json.Unmarshal(job.Settings, &settings)

	// Create temp working directory for this job
	jobDir := filepath.Join(h.cfg.TempDir, jobID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		h.failJob(jobID, job, fmt.Sprintf("create temp dir: %v", err))
		return nil
	}
	defer os.RemoveAll(jobDir)

	// Resolve preset if specified
	if settings.PresetID != "" {
		preset, err := h.db.GetPreset(h.ctx, settings.PresetID)
		if err == nil && preset != nil {
			var presetSettings database.TranscodeSettings
			json.Unmarshal(preset.Settings, &presetSettings)
			settings = mergeSettings(settings, presetSettings)
		}
	}

	// Step 1: Download
	h.db.UpdateJobStatus(h.ctx, jobID, database.JobStatusDownloading)

	inputExt := filepath.Ext(inputCfg.URL)
	if inputExt == "" {
		inputExt = ".mp4"
	}
	inputPath := filepath.Join(jobDir, "input"+inputExt)

	downloader, err := storage.NewDownloader(inputCfg, h.cfg)
	if err != nil {
		h.failJob(jobID, job, fmt.Sprintf("create downloader: %v", err))
		return nil
	}

	log.Printf("[handler] Job %s: downloading from %s", jobID, inputCfg.URL)
	if err := downloader.Download(h.ctx, inputPath); err != nil {
		h.failJob(jobID, job, fmt.Sprintf("download: %v", err))
		return nil
	}

	// Probe input
	probeResult, err := h.ff.Probe(h.ctx, inputPath)
	if err == nil {
		infoJSON, _ := json.Marshal(probeResult)
		h.db.UpdateJobInputInfo(h.ctx, jobID, infoJSON)
	}

	durationSec, _ := h.ff.GetDurationSeconds(h.ctx, inputPath)

	// Step 2: Transcode
	h.db.UpdateJobStatus(h.ctx, jobID, database.JobStatusTranscoding)

	outputExt := "." + settings.Format
	if outputExt == "." {
		outputExt = ".mp4"
	}
	outputPath := filepath.Join(jobDir, "output"+outputExt)
	if settings.Format == "hls" {
		outputDir := filepath.Join(jobDir, "output")
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			h.failJob(jobID, job, fmt.Sprintf("create HLS output dir: %v", err))
			return nil
		}
		outputPath = filepath.Join(outputDir, "index.m3u8")
		settings.ExtraFlags = ensureHLSSegmentFilename(settings.ExtraFlags, filepath.Join(outputDir, "segment_%05d.ts"))
	}

	progressCb := func(progress float32, speed string, fps float32) {
		h.db.UpdateJobProgress(h.ctx, jobID, progress, speed, fps)
	}

	log.Printf("[handler] Job %s: transcoding (duration=%.1fs)", jobID, durationSec)
	if err := h.ff.Run(h.ctx, inputPath, outputPath, settings, durationSec, progressCb); err != nil {
		h.failJob(jobID, job, fmt.Sprintf("transcode: %v", err))
		return nil
	}

	// Gather output file info
	statPath := outputPath
	if settings.Format == "hls" {
		statPath = filepath.Dir(outputPath)
	}
	if stat, err := os.Stat(statPath); err == nil {
		outProbe, _ := h.ff.Probe(h.ctx, outputPath)
		outputInfo := map[string]interface{}{
			"size_bytes": stat.Size(),
			"probe":      outProbe,
		}
		outJSON, _ := json.Marshal(outputInfo)
		h.db.UpdateJobOutputInfo(h.ctx, jobID, outJSON)
	}

	// Step 3: Upload
	h.db.UpdateJobStatus(h.ctx, jobID, database.JobStatusUploading)

	uploader, err := storage.NewUploader(outputCfg, h.cfg)
	if err != nil {
		h.failJob(jobID, job, fmt.Sprintf("create uploader: %v", err))
		return nil
	}

	uploadPath := outputPath
	if settings.Format == "hls" {
		uploadPath = filepath.Dir(outputPath)
	}

	log.Printf("[handler] Job %s: uploading to %s", jobID, outputCfg.URL)
	if err := uploader.Upload(h.ctx, uploadPath); err != nil {
		h.failJob(jobID, job, fmt.Sprintf("upload: %v", err))
		return nil
	}

	// Complete
	h.db.UpdateJobStatus(h.ctx, jobID, database.JobStatusCompleted)
	h.db.UpdateJobProgress(h.ctx, jobID, 100, "", 0)

	// Refresh job for webhook
	job, _ = h.db.GetJob(h.ctx, jobID)
	if job != nil {
		notify.NotifyJobComplete(h.ctx, job.WebhookURL, job, h.cfg.WebhookRetry)
	}

	log.Printf("[handler] Job %s: completed successfully", jobID)
	return nil
}

func ensureHLSSegmentFilename(flags []string, pattern string) []string {
	for i, flag := range flags {
		if flag == "-hls_segment_filename" || strings.HasPrefix(flag, "-hls_segment_filename ") {
			return flags
		}
		if strings.HasPrefix(flag, "hls_segment_filename=") || strings.HasPrefix(flag, "-hls_segment_filename=") {
			return flags
		}
		if flag == pattern && i > 0 && flags[i-1] == "-hls_segment_filename" {
			return flags
		}
	}
	return append(flags, "-hls_segment_filename", pattern)
}

func (h *Handler) failJob(jobID string, job *database.Job, errMsg string) {
	log.Printf("[handler] Job %s failed: %s", jobID, errMsg)
	h.db.UpdateJobError(h.ctx, jobID, errMsg)

	// Refresh for webhook
	job, _ = h.db.GetJob(h.ctx, jobID)
	if job != nil {
		notify.NotifyJobComplete(h.ctx, job.WebhookURL, job, h.cfg.WebhookRetry)
	}
}

// mergeSettings merges inline settings over preset defaults.
// Inline settings take precedence over preset when non-zero.
func mergeSettings(inline, preset database.TranscodeSettings) database.TranscodeSettings {
	result := preset

	if inline.Video.Codec != "" {
		result.Video.Codec = inline.Video.Codec
	}
	if inline.Video.Bitrate != "" {
		result.Video.Bitrate = inline.Video.Bitrate
	}
	if inline.Video.Width > 0 {
		result.Video.Width = inline.Video.Width
	}
	if inline.Video.Height > 0 {
		result.Video.Height = inline.Video.Height
	}
	if inline.Video.Framerate > 0 {
		result.Video.Framerate = inline.Video.Framerate
	}
	if inline.Video.Profile != "" {
		result.Video.Profile = inline.Video.Profile
	}
	if inline.Video.PixelFormat != "" {
		result.Video.PixelFormat = inline.Video.PixelFormat
	}
	if inline.Audio.Codec != "" {
		result.Audio.Codec = inline.Audio.Codec
	}
	if inline.Audio.Bitrate != "" {
		result.Audio.Bitrate = inline.Audio.Bitrate
	}
	if inline.Audio.Channels > 0 {
		result.Audio.Channels = inline.Audio.Channels
	}
	if inline.Audio.SampleRate > 0 {
		result.Audio.SampleRate = inline.Audio.SampleRate
	}
	if inline.Format != "" {
		result.Format = inline.Format
	}
	if inline.HardwareAccel != "" {
		result.HardwareAccel = inline.HardwareAccel
	}
	if len(inline.ExtraFlags) > 0 {
		result.ExtraFlags = inline.ExtraFlags
	}
	return result
}
