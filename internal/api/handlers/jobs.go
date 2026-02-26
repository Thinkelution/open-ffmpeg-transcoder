package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/worker"
)

type JobHandler struct {
	cfg *config.Config
	db  *database.DB
}

func NewJobHandler(cfg *config.Config, db *database.DB) *JobHandler {
	return &JobHandler{cfg: cfg, db: db}
}

func (h *JobHandler) Create(c *gin.Context) {
	var req database.CreateJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	job, err := h.db.CreateJob(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := worker.EnqueueJob(h.cfg, job.ID, req.Priority); err != nil {
		h.db.UpdateJobError(c.Request.Context(), job.ID, "failed to enqueue: "+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue job: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, job)
}

func (h *JobHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	status := c.Query("status")

	if limit > 200 {
		limit = 200
	}

	jobs, total, err := h.db.ListJobs(c.Request.Context(), database.JobListParams{
		Status: status,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jobs":   jobs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *JobHandler) Get(c *gin.Context) {
	id := c.Param("id")
	job, err := h.db.GetJob(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *JobHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	job, err := h.db.GetJob(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	// If still running, mark as cancelled first
	if job.Status == database.JobStatusPending || job.Status == database.JobStatusDownloading ||
		job.Status == database.JobStatusTranscoding || job.Status == database.JobStatusUploading {
		h.db.UpdateJobStatus(c.Request.Context(), id, database.JobStatusCancelled)
	}

	if err := h.db.DeleteJob(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "job deleted"})
}

func (h *JobHandler) Retry(c *gin.Context) {
	id := c.Param("id")

	job, err := h.db.GetJob(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	if job.Status != database.JobStatusFailed && job.Status != database.JobStatusCancelled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "can only retry failed or cancelled jobs"})
		return
	}

	h.db.UpdateJobStatus(c.Request.Context(), id, database.JobStatusPending)
	h.db.UpdateJobProgress(c.Request.Context(), id, 0, "", 0)

	if err := worker.EnqueueJob(h.cfg, id, job.Priority); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue: " + err.Error()})
		return
	}

	job, _ = h.db.GetJob(c.Request.Context(), id)
	c.JSON(http.StatusOK, job)
}
