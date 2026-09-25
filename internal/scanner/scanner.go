package scanner

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/appsettings"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/storage"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/worker"
)

type Scanner struct {
	cfg *config.Config
	db  *database.DB
}

func New(cfg *config.Config, db *database.DB) *Scanner {
	return &Scanner{cfg: cfg, db: db}
}

func (s *Scanner) Start(ctx context.Context) {
	go func() {
		log.Println("[scanner] Started")

		for {
			settings, err := appsettings.Load(ctx, s.db, s.cfg)
			if err != nil {
				log.Printf("[scanner] Could not load settings: %v", err)
			} else if settings.ScannerEnabled {
				s.scanOnce(ctx, settings)
			}

			interval := time.Duration(settings.ScannerIntervalSec) * time.Second
			if interval <= 0 {
				interval = time.Minute
			}
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				log.Println("[scanner] Stopped")
				return
			case <-timer.C:
			}
		}
	}()
}

func (s *Scanner) scanOnce(ctx context.Context, settings appsettings.RuntimeSettings) {
	if settings.ScannerBucket == "" {
		log.Println("[scanner] Scanner is enabled but bucket is empty")
		return
	}

	creds := storage.S3Credentials{
		AccessKeyID:     settings.S3AccessKey,
		SecretAccessKey: settings.S3SecretKey,
		Region:          settings.S3Region,
		Endpoint:        settings.S3Endpoint,
	}

	objects, err := storage.ListObjects(ctx, creds, settings.ScannerBucket, normalizedInputPrefix(settings.ScannerInputPrefix))
	if err != nil {
		log.Printf("[scanner] List failed: %v", err)
		return
	}

	for _, object := range objects {
		if object.Key == nil {
			continue
		}
		key := *object.Key
		if !shouldProcess(key, settings.ScannerOutputPrefix) {
			continue
		}

		exists, err := s.db.HasScannerJobForSource(ctx, settings.ScannerBucket, key)
		if err != nil {
			log.Printf("[scanner] Could not check %s: %v", key, err)
			continue
		}
		if exists {
			continue
		}

		job, err := s.createJob(ctx, settings, creds, key)
		if err != nil {
			log.Printf("[scanner] Could not create job for %s: %v", key, err)
			continue
		}
		if err := worker.EnqueueJob(s.cfg, job.ID, job.Priority); err != nil {
			log.Printf("[scanner] Could not enqueue job %s for %s: %v", job.ID, key, err)
			continue
		}
		log.Printf("[scanner] Queued %s as job %s", key, job.ID)
	}
}

func (s *Scanner) createJob(ctx context.Context, settings appsettings.RuntimeSettings, creds storage.S3Credentials, key string) (*database.Job, error) {
	outputKey := outputPrefixForKey(settings, key)
	metadata, _ := json.Marshal(map[string]string{
		"scanner_bucket":     settings.ScannerBucket,
		"scanner_source_key": key,
		"hls_prefix":         outputKey,
	})
	credsJSON, _ := json.Marshal(creds)
	var renditions []database.HLSRendition
	if err := json.Unmarshal([]byte(settings.HLSLadder), &renditions); err != nil || len(renditions) == 0 {
		renditions = []database.HLSRendition{{
			Name:         fmt.Sprintf("%dp", settings.HLSHeight),
			Width:        settings.HLSWidth,
			Height:       settings.HLSHeight,
			VideoBitrate: settings.HLSVideoBitrate,
			AudioBitrate: settings.HLSAudioBitrate,
		}}
	}

	return s.db.CreateJob(ctx, &database.CreateJobRequest{
		Input: database.StorageConfig{
			Type:        "s3",
			URL:         fmt.Sprintf("s3://%s/%s", settings.ScannerBucket, key),
			Credentials: credsJSON,
		},
		Output: database.StorageConfig{
			Type:        "s3",
			URL:         fmt.Sprintf("s3://%s/%s", settings.ScannerBucket, outputKey),
			Credentials: credsJSON,
		},
		Settings: database.TranscodeSettings{
			Video: database.VideoSettings{
				Codec:       settings.HLSVideoCodec,
				Bitrate:     settings.HLSVideoBitrate,
				Width:       settings.HLSWidth,
				Height:      settings.HLSHeight,
				Framerate:   settings.HLSFramerate,
				Profile:     "main",
				PixelFormat: "yuv420p",
			},
			Audio: database.AudioSettings{
				Codec:      settings.HLSAudioCodec,
				Bitrate:    settings.HLSAudioBitrate,
				Channels:   2,
				SampleRate: 48000,
			},
			Format: "hls",
			ExtraFlags: []string{
				"-hls_time", fmt.Sprintf("%d", settings.HLSSegmentSeconds),
				"-hls_playlist_type", "vod",
				"-hls_flags", "independent_segments",
			},
			HLS: &database.HLSSettings{
				MasterPlaylist: "master.m3u8",
				SegmentSeconds: settings.HLSSegmentSeconds,
				VideoCodec:     settings.HLSVideoCodec,
				AudioCodec:     settings.HLSAudioCodec,
				AudioBitrate:   settings.HLSAudioBitrate,
				Framerate:      settings.HLSFramerate,
				Renditions:     renditions,
			},
		},
		Priority: settings.ScannerPriority,
		Metadata: metadata,
	})
}

func shouldProcess(key, outputPrefix string) bool {
	if strings.HasSuffix(key, "/") {
		return false
	}
	if outputPrefix != "" && strings.HasPrefix(key, normalizedOutputPrefix(outputPrefix)+"/") {
		return false
	}
	switch strings.ToLower(filepath.Ext(key)) {
	case ".mp4", ".mov", ".mkv", ".avi", ".webm", ".m4v", ".ts", ".mts":
		return true
	default:
		return false
	}
}

func normalizedInputPrefix(value string) string {
	prefix := strings.Trim(value, "/")
	if prefix == "." {
		return ""
	}
	if prefix == "" {
		return ""
	}
	return prefix + "/"
}

func normalizedOutputPrefix(value string) string {
	prefix := strings.Trim(value, "/")
	if prefix == "" || prefix == "." {
		return "hls"
	}
	return prefix
}

func outputPrefixForKey(settings appsettings.RuntimeSettings, key string) string {
	template := strings.TrimSpace(settings.ScannerOutputTemplate)
	if template == "" {
		return path.Join(normalizedOutputPrefix(settings.ScannerOutputPrefix), uniqueFolder(key)) + "/"
	}

	template = strings.TrimPrefix(template, "/")
	template = strings.TrimSuffix(template, "/")
	template = strings.TrimSuffix(template, "/*")
	if template == "" {
		return path.Join(normalizedOutputPrefix(settings.ScannerOutputPrefix), uniqueFolder(key)) + "/"
	}

	base := fileBaseName(key)
	hash := sourceHash(key)
	sourceDir := strings.Trim(path.Dir(key), ".")
	sourceDir = strings.Trim(sourceDir, "/")

	replacer := strings.NewReplacer(
		"{file_base_name}", base,
		"{source_hash}", hash,
		"{source_dir}", sourceDir,
		"file_base_name", base,
		"source_hash", hash,
		"source_dir", sourceDir,
	)
	template = replacer.Replace(template)

	template = strings.Trim(template, "/")
	if template == "" {
		template = uniqueFolder(key)
	}
	return template + "/"
}

var unsafeKeyChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func uniqueFolder(key string) string {
	return fmt.Sprintf("%s-%s", fileBaseName(key), sourceHash(key))
}

func fileBaseName(key string) string {
	ext := filepath.Ext(key)
	base := strings.TrimSuffix(path.Base(key), ext)
	base = strings.Trim(unsafeKeyChars.ReplaceAllString(base, "-"), "-._")
	if base == "" {
		base = "video"
	}
	return strings.ToLower(base)
}

func sourceHash(key string) string {
	sum := sha1.Sum([]byte(key))
	return hex.EncodeToString(sum[:])[:12]
}
