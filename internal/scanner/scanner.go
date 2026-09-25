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
	if !s.cfg.ScannerEnabled {
		return
	}
	if s.cfg.ScannerBucket == "" {
		log.Println("[scanner] SCANNER_ENABLED=true but SCANNER_BUCKET is empty; scanner disabled")
		return
	}

	interval := time.Duration(s.cfg.ScannerIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}

	go func() {
		log.Printf("[scanner] Watching s3://%s/%s every %s", s.cfg.ScannerBucket, s.cfg.ScannerInputPrefix, interval)
		s.scanOnce(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("[scanner] Stopped")
				return
			case <-ticker.C:
				s.scanOnce(ctx)
			}
		}
	}()
}

func (s *Scanner) scanOnce(ctx context.Context) {
	creds := storage.S3Credentials{
		AccessKeyID:     s.cfg.S3AccessKey,
		SecretAccessKey: s.cfg.S3SecretKey,
		Region:          s.cfg.S3Region,
		Endpoint:        s.cfg.S3Endpoint,
	}

	objects, err := storage.ListObjects(ctx, creds, s.cfg.ScannerBucket, s.normalizedInputPrefix())
	if err != nil {
		log.Printf("[scanner] List failed: %v", err)
		return
	}

	for _, object := range objects {
		if object.Key == nil {
			continue
		}
		key := *object.Key
		if !s.shouldProcess(key) {
			continue
		}

		exists, err := s.db.HasScannerJobForSource(ctx, s.cfg.ScannerBucket, key)
		if err != nil {
			log.Printf("[scanner] Could not check %s: %v", key, err)
			continue
		}
		if exists {
			continue
		}

		job, err := s.createJob(ctx, key)
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

func (s *Scanner) createJob(ctx context.Context, key string) (*database.Job, error) {
	outputKey := path.Join(s.normalizedOutputPrefix(), uniqueFolder(key)) + "/"
	metadata, _ := json.Marshal(map[string]string{
		"scanner_bucket":     s.cfg.ScannerBucket,
		"scanner_source_key": key,
		"hls_prefix":         outputKey,
	})

	return s.db.CreateJob(ctx, &database.CreateJobRequest{
		Input: database.StorageConfig{
			Type: "s3",
			URL:  fmt.Sprintf("s3://%s/%s", s.cfg.ScannerBucket, key),
		},
		Output: database.StorageConfig{
			Type: "s3",
			URL:  fmt.Sprintf("s3://%s/%s", s.cfg.ScannerBucket, outputKey),
		},
		Settings: database.TranscodeSettings{
			Video: database.VideoSettings{
				Codec:       s.cfg.HLSVideoCodec,
				Bitrate:     s.cfg.HLSVideoBitrate,
				Width:       s.cfg.HLSWidth,
				Height:      s.cfg.HLSHeight,
				Framerate:   s.cfg.HLSFramerate,
				Profile:     "main",
				PixelFormat: "yuv420p",
			},
			Audio: database.AudioSettings{
				Codec:      s.cfg.HLSAudioCodec,
				Bitrate:    s.cfg.HLSAudioBitrate,
				Channels:   2,
				SampleRate: 48000,
			},
			Format: "hls",
			ExtraFlags: []string{
				"-hls_time", fmt.Sprintf("%d", s.cfg.HLSSegmentSeconds),
				"-hls_playlist_type", "vod",
				"-hls_flags", "independent_segments",
			},
		},
		Priority: s.cfg.ScannerPriority,
		Metadata: metadata,
	})
}

func (s *Scanner) shouldProcess(key string) bool {
	if strings.HasSuffix(key, "/") {
		return false
	}
	if strings.HasPrefix(key, s.normalizedOutputPrefix()+"/") {
		return false
	}
	switch strings.ToLower(filepath.Ext(key)) {
	case ".mp4", ".mov", ".mkv", ".avi", ".webm", ".m4v", ".ts", ".mts":
		return true
	default:
		return false
	}
}

func (s *Scanner) normalizedInputPrefix() string {
	prefix := strings.Trim(s.cfg.ScannerInputPrefix, "/")
	if prefix == "." {
		return ""
	}
	if prefix == "" {
		return ""
	}
	return prefix + "/"
}

func (s *Scanner) normalizedOutputPrefix() string {
	prefix := strings.Trim(s.cfg.ScannerOutputPrefix, "/")
	if prefix == "" || prefix == "." {
		return "hls"
	}
	return prefix
}

var unsafeKeyChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func uniqueFolder(key string) string {
	ext := filepath.Ext(key)
	base := strings.TrimSuffix(path.Base(key), ext)
	base = strings.Trim(unsafeKeyChars.ReplaceAllString(base, "-"), "-._")
	if base == "" {
		base = "video"
	}
	sum := sha1.Sum([]byte(key))
	return fmt.Sprintf("%s-%s", strings.ToLower(base), hex.EncodeToString(sum[:])[:12])
}
