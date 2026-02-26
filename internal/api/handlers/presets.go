package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
)

type PresetHandler struct {
	db *database.DB
}

func NewPresetHandler(db *database.DB) *PresetHandler {
	return &PresetHandler{db: db}
}

func (h *PresetHandler) Create(c *gin.Context) {
	var req database.CreatePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	preset, err := h.db.CreatePreset(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, preset)
}

func (h *PresetHandler) List(c *gin.Context) {
	presets, err := h.db.ListPresets(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, presets)
}

func (h *PresetHandler) Get(c *gin.Context) {
	id := c.Param("id")
	preset, err := h.db.GetPreset(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if preset == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "preset not found"})
		return
	}
	c.JSON(http.StatusOK, preset)
}

func (h *PresetHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req database.UpdatePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	preset, err := h.db.UpdatePreset(c.Request.Context(), id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if preset == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "preset not found or is a system preset"})
		return
	}

	c.JSON(http.StatusOK, preset)
}

func (h *PresetHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.DeletePreset(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "preset deleted"})
}
