package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
)

type DashboardHandler struct {
	cfg  *config.Config
	db   *database.DB
	auth interface {
		Login(*gin.Context, string, string) bool
		Logout(*gin.Context)
	}
}

func NewDashboardHandler(cfg *config.Config, db *database.DB, auth interface {
	Login(*gin.Context, string, string) bool
	Logout(*gin.Context)
}) *DashboardHandler {
	return &DashboardHandler{cfg: cfg, db: db, auth: auth}
}

func (h *DashboardHandler) LoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{
		"Title": "Sign in",
	})
}

func (h *DashboardHandler) LoginPost(c *gin.Context) {
	user := c.PostForm("username")
	password := c.PostForm("password")
	if h.auth.Login(c, user, password) {
		c.Redirect(http.StatusFound, "/")
		return
	}
	c.HTML(http.StatusUnauthorized, "login.html", gin.H{
		"Title": "Sign in",
		"Error": "Invalid username or password",
	})
}

func (h *DashboardHandler) Logout(c *gin.Context) {
	h.auth.Logout(c)
	c.Redirect(http.StatusFound, "/login")
}

func (h *DashboardHandler) Index(c *gin.Context) {
	counts, _ := h.db.GetJobCounts(c.Request.Context())
	jobs, _, _ := h.db.ListJobs(c.Request.Context(), database.JobListParams{Limit: 10})

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"Title":      "Dashboard",
		"Counts":     counts,
		"RecentJobs": jobs,
		"Port":       h.cfg.Port,
	})
}

func (h *DashboardHandler) JobsPage(c *gin.Context) {
	status := c.Query("status")
	jobs, total, _ := h.db.ListJobs(c.Request.Context(), database.JobListParams{
		Status: status,
		Limit:  100,
	})

	c.HTML(http.StatusOK, "jobs.html", gin.H{
		"Title":  "Jobs",
		"Jobs":   jobs,
		"Total":  total,
		"Status": status,
	})
}

func (h *DashboardHandler) SystemPage(c *gin.Context) {
	c.HTML(http.StatusOK, "system.html", gin.H{
		"Title": "System",
	})
}

func (h *DashboardHandler) SettingsPage(c *gin.Context) {
	c.HTML(http.StatusOK, "settings.html", gin.H{
		"Title": "Settings",
	})
}
