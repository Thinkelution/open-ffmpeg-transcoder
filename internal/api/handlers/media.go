package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
		playable := false
		if job.Status == database.JobStatusCompleted && outputCfg.Type == "s3" && outputPrefix != "" {
			playable = true
		}

		shareURL := ""
		thumbnailURL := ""
		if playable {
			shareURL = h.publicPlayerURL(c, job.ID)
		}
		if thumbnailReady && outputCfg.Type == "s3" {
			thumbCfg := outputCfg
			thumbCfg.URL = childStorageURL(outputCfg.URL, "thumbnail.jpg")
			if signedURL, err := storage.PresignGetObject(c.Request.Context(), thumbCfg, h.cfg, 6*time.Hour); err == nil {
				thumbnailURL = signedURL
			}
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
			"playback_url":    fmt.Sprintf("/api/v1/media/uploads/%s/hls/master.m3u8", job.ID),
			"share_url":       shareURL,
			"thumbnail_ready": thumbnailReady,
			"thumbnail_url":   thumbnailURL,
			"playable":        playable,
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
	h.streamStorageObject(c, thumbCfg, "image/jpeg", false)
}

func (h *MediaHandler) Share(c *gin.Context) {
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
	if job.Status != database.JobStatusCompleted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job is not completed yet"})
		return
	}
	presignExpires := 24 * time.Hour
	c.JSON(http.StatusOK, gin.H{
		"expires_at": time.Now().Add(presignExpires).UTC().Format(time.RFC3339),
		"player_url": h.publicPlayerURL(c, jobID),
		"hls_url":    h.externalBaseURL(c) + "/play/" + jobID + "/hls/master.m3u8",
	})
}

func (h *MediaHandler) PublicPlayer(c *gin.Context) {
	jobID := c.Param("id")
	job, err := h.db.GetJob(c.Request.Context(), jobID)
	if err != nil || job == nil {
		c.String(http.StatusNotFound, "Video not found")
		return
	}
	var outputCfg database.StorageConfig
	json.Unmarshal(job.OutputConfig, &outputCfg)
	_, outputPrefix := parseS3URL(outputCfg.URL)
	name := html.EscapeString(job.ID)
	metadata := map[string]string{}
	json.Unmarshal(job.Metadata, &metadata)
	if source := path.Base(metadata["scanner_source_key"]); source != "." && source != "/" && source != "" {
		name = html.EscapeString(source)
	}
	hlsURL := html.EscapeString("/play/" + jobID + "/hls/master.m3u8")
	caption := html.EscapeString(outputPrefix)
	page := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s - 1transcoder</title>
<script src="https://cdn.jsdelivr.net/npm/hls.js@1.5.17/dist/hls.min.js"></script>
<style>
body{margin:0;background:#020617;color:#e5e7eb;font-family:system-ui,-apple-system,Segoe UI,sans-serif;display:grid;min-height:100vh;place-items:center;padding:24px}.wrap{width:min(1100px,100%%)}video{width:100%%;aspect-ratio:16/9;background:#000;border-radius:18px;border:1px solid #1f2937}h1{font-size:20px;margin:16px 0 4px}.muted{color:#94a3b8;font-size:13px;margin:0}</style>
</head>
<body><main class="wrap"><video id="player" controls autoplay playsinline></video><h1>%s</h1><p class="muted">%s</p></main><script>
const src=%q; const video=document.getElementById('player');
if(video.canPlayType('application/vnd.apple.mpegurl')){video.src=src;} else if(window.Hls&&window.Hls.isSupported()){const hls=new Hls();hls.loadSource(src);hls.attachMedia(video);} else {document.body.insertAdjacentHTML('beforeend','<p>This browser cannot play HLS.</p>');}
</script></body></html>`, name, name, caption, hlsURL)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

func (h *MediaHandler) PublicHLS(c *gin.Context) {
	jobID := c.Param("id")
	assetPath := strings.TrimPrefix(c.Param("asset"), "/")
	if assetPath == "" {
		assetPath = "master.m3u8"
	}
	if !safeHLSAssetPath(assetPath) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid HLS asset path"})
		return
	}
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
	if err := json.Unmarshal(job.OutputConfig, &outputCfg); err != nil || outputCfg.Type != "s3" {
		c.JSON(http.StatusNotFound, gin.H{"error": "HLS output is not available"})
		return
	}
	assetCfg := outputCfg
	assetCfg.URL = childStorageURL(outputCfg.URL, assetPath)
	contentType := contentTypeForHLSAsset(assetPath)
	if strings.HasSuffix(strings.ToLower(assetPath), ".m3u8") {
		h.streamStorageObjectWithRewriter(c, assetCfg, contentType, func(body string) string {
			return h.rewritePresignedHLSPlaylist(c, body, outputCfg, assetPath, 24*time.Hour)
		})
		return
	}
	if signedURL, err := storage.PresignGetObject(c.Request.Context(), assetCfg, h.cfg, 24*time.Hour); err == nil {
		c.Redirect(http.StatusFound, signedURL)
		return
	}
	h.streamStorageObject(c, assetCfg, contentType, false)
}

func (h *MediaHandler) HLS(c *gin.Context) {
	jobID := c.Param("id")
	assetPath := strings.TrimPrefix(c.Param("asset"), "/")
	if assetPath == "" {
		assetPath = "master.m3u8"
	}
	if !safeHLSAssetPath(assetPath) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid HLS asset path"})
		return
	}

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
		c.JSON(http.StatusNotFound, gin.H{"error": "HLS output is not stored in S3"})
		return
	}

	assetCfg := outputCfg
	assetCfg.URL = childStorageURL(outputCfg.URL, assetPath)
	contentType := contentTypeForHLSAsset(assetPath)
	if strings.HasSuffix(strings.ToLower(assetPath), ".m3u8") {
		h.streamStorageObjectWithRewriter(c, assetCfg, contentType, func(body string) string {
			return h.rewritePresignedHLSPlaylist(c, body, outputCfg, assetPath, 24*time.Hour)
		})
		return
	}
	if signedURL, err := storage.PresignGetObject(c.Request.Context(), assetCfg, h.cfg, 24*time.Hour); err == nil {
		c.Redirect(http.StatusFound, signedURL)
		return
	}
	h.streamStorageObject(c, assetCfg, contentType, false)
}

func (h *MediaHandler) streamStorageObject(c *gin.Context, cfg database.StorageConfig, contentType string, rewritePlaylist bool) {
	downloader, err := storage.NewDownloader(cfg, h.cfg)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tmpFile := filepath.Join(h.cfg.TempDir, "media-"+uuid.New().String()+filepath.Ext(cfg.URL))
	defer os.Remove(tmpFile)
	if err := downloader.Download(c.Request.Context(), tmpFile); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "media asset not available"})
		return
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rewritePlaylist {
		data = []byte(rewriteHLSPlaylist(string(data), c.Request.URL.Path))
	}
	c.Header("Cache-Control", "private, max-age=30")
	c.Data(http.StatusOK, contentType, data)
}

func (h *MediaHandler) streamStorageObjectWithRewriter(c *gin.Context, cfg database.StorageConfig, contentType string, rewrite func(string) string) {
	downloader, err := storage.NewDownloader(cfg, h.cfg)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tmpFile := filepath.Join(h.cfg.TempDir, "media-"+uuid.New().String()+filepath.Ext(cfg.URL))
	defer os.Remove(tmpFile)
	if err := downloader.Download(c.Request.Context(), tmpFile); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "media asset not available"})
		return
	}
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rewrite != nil {
		data = []byte(rewrite(string(data)))
	}
	c.Header("Cache-Control", "private, max-age=30")
	c.Data(http.StatusOK, contentType, data)
}

func rewriteHLSPlaylist(body, requestPath string) string {
	basePath := path.Dir(requestPath)
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "/") {
			continue
		}
		lines[i] = path.Join(basePath, trimmed)
	}
	return strings.Join(lines, "\n")
}

func safeHLSAssetPath(assetPath string) bool {
	if strings.Contains(assetPath, "\\") || strings.HasPrefix(assetPath, "/") {
		return false
	}
	clean := path.Clean(assetPath)
	return clean == assetPath && clean != "." && !strings.HasPrefix(clean, "../") && clean != ".."
}

func contentTypeForHLSAsset(assetPath string) string {
	switch strings.ToLower(path.Ext(assetPath)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}

func (h *MediaHandler) rewritePresignedHLSPlaylist(c *gin.Context, body string, outputCfg database.StorageConfig, assetPath string, expires time.Duration) string {
	baseAsset := path.Dir(assetPath)
	if baseAsset == "." {
		baseAsset = ""
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "/") {
			continue
		}
		asset := path.Join(baseAsset, trimmed)
		if strings.HasSuffix(strings.ToLower(asset), ".m3u8") {
			lines[i] = path.Join(path.Dir(c.Request.URL.Path), trimmed)
			continue
		}
		if signedURL, err := h.presignedHLSURLFromConfig(c, outputCfg, asset, expires); err == nil {
			lines[i] = signedURL
		}
	}
	return strings.Join(lines, "\n")
}

func (h *MediaHandler) presignedHLSURL(c *gin.Context, job *database.Job, asset string, expires time.Duration) (string, error) {
	var outputCfg database.StorageConfig
	if err := json.Unmarshal(job.OutputConfig, &outputCfg); err != nil {
		return "", fmt.Errorf("job output config is invalid")
	}
	return h.presignedHLSURLFromConfig(c, outputCfg, asset, expires)
}

func (h *MediaHandler) presignedHLSURLFromConfig(c *gin.Context, outputCfg database.StorageConfig, asset string, expires time.Duration) (string, error) {
	if outputCfg.Type != "s3" {
		return "", fmt.Errorf("HLS output is not stored in S3")
	}
	assetCfg := outputCfg
	assetCfg.URL = childStorageURL(outputCfg.URL, strings.Trim(asset, "/"))
	return storage.PresignGetObject(c.Request.Context(), assetCfg, h.cfg, expires)
}

func (h *MediaHandler) publicPlayerURL(c *gin.Context, jobID string) string {
	return h.externalBaseURL(c) + "/play/" + jobID
}

func (h *MediaHandler) externalBaseURL(c *gin.Context) string {
	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	return scheme + "://" + host
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
