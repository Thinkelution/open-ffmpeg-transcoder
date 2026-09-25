package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/analyzer"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/appsettings"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/transcoder"
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
