package handlers

import (
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/analyzer"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/appsettings"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/scanner"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/storage"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/transcoder"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/worker"
)

type SystemHandler struct {
	cfg *config.Config
	db  *database.DB
	ff  *transcoder.FFmpeg
}

func NewSystemHandler(cfg *config.Config, db *database.DB) *SystemHandler {
	return &SystemHandler{
		cfg: cfg,
		db:  db,
		ff:  transcoder.New(cfg.FFmpegPath, cfg.FFprobePath),
	}
}

func (h *SystemHandler) Health(c *gin.Context) {
	counts, _ := h.db.GetJobCounts(c.Request.Context())

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": "1.0.0",
		"jobs":    counts,
	})
}

func (h *SystemHandler) Info(c *gin.Context) {
	version := h.ff.GetVersion()
	encoders := h.ff.GetEncoders()
	decoders := h.ff.GetDecoders()
	formats := h.ff.GetFormats()
	nvencAvailable := h.ff.CheckNVIDIA()

	c.JSON(http.StatusOK, gin.H{
		"ffmpeg_version": version,
		"encoders":       encoders,
		"decoders":       decoders,
		"formats":        formats,
		"gpu": gin.H{
			"nvidia_available": nvencAvailable,
		},
		"config": gin.H{
			"max_workers": h.cfg.MaxWorkers,
			"temp_dir":    h.cfg.TempDir,
		},
	})
}

func (h *SystemHandler) Analyze(c *gin.Context) {
	a := analyzer.New(h.ff)
	result, err := a.Analyze(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *SystemHandler) Benchmark(c *gin.Context) {
	var req struct {
		Codec  string `json:"codec"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	c.ShouldBindJSON(&req)

	result, err := analyzer.RunBenchmark(c.Request.Context(), h.ff, req.Codec, req.Width, req.Height)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "result": result})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *SystemHandler) GetSettings(c *gin.Context) {
	settings, err := appsettings.Load(c.Request.Context(), h.db, h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, settings.AppSettings)
}

func (h *SystemHandler) UpdateSettings(c *gin.Context) {
	var req database.UpdateAppSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := appsettings.Save(c.Request.Context(), h.db, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	settings, err := appsettings.Load(c.Request.Context(), h.db, h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, settings.AppSettings)
}

func (h *SystemHandler) UploadSource(c *gin.Context) {
	settings, err := appsettings.Load(c.Request.Context(), h.db, h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(settings.ScannerBucket) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scan bucket is not configured"})
		return
	}
	if strings.TrimSpace(settings.S3AccessKey) == "" || strings.TrimSpace(settings.S3SecretKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Wasabi credentials are not configured"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer src.Close()

	key := uploadKey(settings.ScannerInputPrefix, file.Filename)
	creds := storage.S3Credentials{
		AccessKeyID:     settings.S3AccessKey,
		SecretAccessKey: settings.S3SecretKey,
		Region:          settings.S3Region,
		Endpoint:        settings.S3Endpoint,
	}
	if err := storage.UploadObject(c.Request.Context(), creds, settings.ScannerBucket, key, src); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": uploadErrorMessage(settings.ScannerBucket, key, err)})
		return
	}

	job, err := scanner.CreateJobForKey(c.Request.Context(), h.db, settings, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "uploaded file, but could not create job: " + err.Error()})
		return
	}
	if err := worker.EnqueueJob(h.cfg, job.ID, job.Priority); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "uploaded file, but could not enqueue job: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bucket": settings.ScannerBucket,
		"key":    key,
		"job_id": job.ID,
	})
}

func uploadErrorMessage(bucket, key string, err error) string {
	message := err.Error()
	target := fmt.Sprintf("s3://%s/%s", bucket, key)
	if strings.Contains(message, "AccessDenied") || strings.Contains(message, "StatusCode: 403") {
		return fmt.Sprintf("Wasabi denied upload to %s. Update the access key or bucket policy to allow s3:PutObject for this bucket/prefix, then try again.", target)
	}
	return fmt.Sprintf("Upload to %s failed: %s", target, message)
}

var unsafeUploadChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func uploadKey(prefix, filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filepath.Base(filename), ext)
	base = strings.Trim(unsafeUploadChars.ReplaceAllString(base, "-"), "-._")
	if base == "" {
		base = "video"
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	cleanPrefix := strings.Trim(prefix, "/")
	name := fmt.Sprintf("%s-%s%s", base, stamp, strings.ToLower(ext))
	if cleanPrefix == "" {
		return name
	}
	return path.Join(cleanPrefix, name)
}
