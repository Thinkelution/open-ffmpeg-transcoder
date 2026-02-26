package handlers

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/storage"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/transcoder"
)

type MediaHandler struct {
	cfg *config.Config
	ff  *transcoder.FFmpeg
}

func NewMediaHandler(cfg *config.Config) *MediaHandler {
	return &MediaHandler{
		cfg: cfg,
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
