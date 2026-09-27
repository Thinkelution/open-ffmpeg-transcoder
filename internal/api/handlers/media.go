package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/storage"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/transcoder"
)

type MediaHandler struct {
	cfg *config.Config
	db  *database.DB
	ff  *transcoder.FFmpeg
}

func NewMediaHandler(cfg *config.Config, db *database.DB) *MediaHandler {
	return &MediaHandler{
		cfg: cfg,
		db:  db,
		ff:  transcoder.New(cfg.FFmpegPath, cfg.FFprobePath),
	}
}

func (h *MediaHandler) Probe(c *gin.Context) {
	var req database.ProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// For HTTP/HTTPS URLs, ffprobe can handle them directly
	if req.Input.Type == "http" || req.Input.Type == "https" {
		result, err := h.ff.Probe(c.Request.Context(), req.Input.URL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
		return
	}

	// For other types, download first then probe
	tmpFile := filepath.Join(h.cfg.TempDir, "probe-"+uuid.New().String())
	defer os.Remove(tmpFile)

	downloader, err := storage.NewDownloader(req.Input, h.cfg)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := downloader.Download(c.Request.Context(), tmpFile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "download failed: " + err.Error()})
		return
	}

	result, err := h.ff.Probe(c.Request.Context(), tmpFile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *MediaHandler) Uploads(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "24"))
	if limit <= 0 || limit > 100 {
		limit = 24
	}

	jobs, _, err := h.db.ListJobs(c.Request.Context(), database.JobListParams{Limit: limit})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]gin.H, 0, len(jobs))
	for _, job := range jobs {
		var inputCfg database.StorageConfig
		var outputCfg database.StorageConfig
		var settings database.TranscodeSettings
		metadata := map[string]string{}
		outputInfo := map[string]interface{}{}
		json.Unmarshal(job.InputConfig, &inputCfg)
		json.Unmarshal(job.OutputConfig, &outputCfg)
		json.Unmarshal(job.Settings, &settings)
		json.Unmarshal(job.Metadata, &metadata)
		json.Unmarshal(job.OutputInfo, &outputInfo)

		if settings.Format != "hls" && metadata["scanner_source_key"] == "" && metadata["scanner_output_prefix"] == "" {
			continue
		}

		outputBucket, outputPrefix := parseS3URL(outputCfg.URL)
		sourceKey := metadata["scanner_source_key"]
		if sourceKey == "" {
			sourceKey = strings.TrimPrefix(inputCfg.URL, "s3://")
		}
		name := path.Base(sourceKey)
		if name == "." || name == "/" || name == "" {
			name = job.ID
		}

		thumbnailReady := hasThumbnail(outputInfo)
		if job.Status == database.JobStatusCompleted && outputCfg.Type == "s3" && outputPrefix != "" {
			thumbnailReady = true
		}

		items = append(items, gin.H{
			"job_id":          job.ID,
			"name":            name,
			"source_key":      sourceKey,
			"status":          job.Status,
			"progress":        job.Progress,
			"speed":           job.Speed,
			"created_at":      job.CreatedAt,
			"updated_at":      job.UpdatedAt,
			"output_bucket":   outputBucket,
			"output_prefix":   outputPrefix,
			"master_playlist": childStorageURL(outputCfg.URL, "master.m3u8"),
			"thumbnail_ready": thumbnailReady,
		})
	}

	c.JSON(http.StatusOK, gin.H{"uploads": items})
}

func (h *MediaHandler) Thumbnail(c *gin.Context) {
	jobID := c.Param("id")
	job, err := h.db.GetJob(c.Request.Context(), jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	var outputCfg database.StorageConfig
	if err := json.Unmarshal(job.OutputConfig, &outputCfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job output config is invalid"})
		return
	}
	if outputCfg.Type != "s3" {
		c.JSON(http.StatusNotFound, gin.H{"error": "thumbnail is not stored in S3"})
		return
	}

	thumbCfg := outputCfg
	thumbCfg.URL = childStorageURL(outputCfg.URL, "thumbnail.jpg")
	downloader, err := storage.NewDownloader(thumbCfg, h.cfg)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tmpFile := filepath.Join(h.cfg.TempDir, "thumb-"+jobID+".jpg")
	defer os.Remove(tmpFile)
	if err := downloader.Download(c.Request.Context(), tmpFile); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "thumbnail not available"})
		return
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "image/jpeg", data)
}

func hasThumbnail(outputInfo map[string]interface{}) bool {
	thumbnail, ok := outputInfo["thumbnail"]
	if !ok || thumbnail == nil {
		return false
	}
	if data, ok := thumbnail.(map[string]interface{}); ok {
		_, hasURL := data["url"]
		_, hasName := data["name"]
		return hasURL || hasName
	}
	return true
}

func parseS3URL(raw string) (bucket, key string) {
	if !strings.HasPrefix(raw, "s3://") {
		return "", ""
	}
	trimmed := strings.TrimPrefix(raw, "s3://")
	parts := strings.SplitN(trimmed, "/", 2)
	bucket = parts[0]
	if len(parts) > 1 {
		key = strings.Trim(parts[1], "/")
	}
	return bucket, key
}

func childStorageURL(baseURL, child string) string {
	baseURL = strings.TrimSpace(baseURL)
	child = strings.Trim(child, "/")
	if baseURL == "" || child == "" {
		return ""
	}
	if strings.HasPrefix(baseURL, "s3://") {
		trimmed := strings.TrimSuffix(strings.TrimPrefix(baseURL, "s3://"), "/")
		return "s3://" + path.Join(trimmed, child)
	}
	return strings.TrimRight(baseURL, "/") + "/" + child
}
