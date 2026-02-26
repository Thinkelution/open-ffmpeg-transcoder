package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
)

type DashboardHandler struct {
	cfg *config.Config
	db  *database.DB
}

func NewDashboardHandler(cfg *config.Config, db *database.DB) *DashboardHandler {
	return &DashboardHandler{cfg: cfg, db: db}
}

func (h *DashboardHandler) Index(c *gin.Context) {
	counts, _ := h.db.GetJobCounts(c.Request.Context())
	jobs, _, _ := h.db.ListJobs(c.Request.Context(), database.JobListParams{Limit: 10})

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"Title":     "Dashboard",
		"Counts":    counts,
		"RecentJobs": jobs,
		"Port":      h.cfg.Port,
	})
}

func (h *DashboardHandler) JobsPage(c *gin.Context) {
	status := c.Query("status")
	jobs, total, _ := h.db.ListJobs(c.Request.Context(), database.JobListParams{
		Status: status,
		Limit:  100,
	})

	c.HTML(http.StatusOK, "jobs.html", gin.H{
		"Title":      "Jobs",
		"Jobs":       jobs,
		"Total":      total,
		"Status":     status,
	})
}

func (h *DashboardHandler) SystemPage(c *gin.Context) {
	c.HTML(http.StatusOK, "system.html", gin.H{
		"Title": "System",
	})
}
