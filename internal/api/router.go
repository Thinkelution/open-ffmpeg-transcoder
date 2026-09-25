package api

import (
	"html/template"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/api/handlers"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/api/middleware"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
	"github.com/thinkelution/open-ffmpeg-transcoder/web"
)

func NewRouter(cfg *config.Config, db *database.DB) *gin.Engine {
	if cfg.LogLevel != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	// Load embedded HTML templates
	tmpl := template.Must(template.New("").ParseFS(web.TemplatesFS, "templates/*.html"))
	r.SetHTMLTemplate(tmpl)

	// Serve embedded static files
	staticFS, _ := fs.Sub(web.StaticFS, "static")
	r.StaticFS("/static", http.FS(staticFS))

	auth := middleware.NewSessionAuth(cfg)

	// Dashboard routes
	dash := handlers.NewDashboardHandler(cfg, db, auth)
	r.GET("/login", dash.LoginPage)
	r.POST("/login", dash.LoginPost)
	r.GET("/logout", dash.Logout)

	dashboard := r.Group("/")
	dashboard.Use(auth.RequireDashboard())
	dashboard.GET("/", dash.Index)
	dashboard.GET("/jobs", dash.JobsPage)
	dashboard.GET("/system", dash.SystemPage)
	dashboard.GET("/settings", dash.SettingsPage)

	// API routes
	v1 := r.Group("/api/v1")
	v1.Use(auth.RequireAPI())
	v1.Use(middleware.RateLimit())

	jobH := handlers.NewJobHandler(cfg, db)
	v1.POST("/jobs", jobH.Create)
	v1.GET("/jobs", jobH.List)
	v1.GET("/jobs/:id", jobH.Get)
	v1.DELETE("/jobs/:id", jobH.Delete)
	v1.POST("/jobs/:id/retry", jobH.Retry)

	presetH := handlers.NewPresetHandler(db)
	v1.POST("/presets", presetH.Create)
	v1.GET("/presets", presetH.List)
	v1.GET("/presets/:id", presetH.Get)
	v1.PUT("/presets/:id", presetH.Update)
	v1.DELETE("/presets/:id", presetH.Delete)

	sysH := handlers.NewSystemHandler(cfg, db)
	v1.GET("/system/health", sysH.Health)
	v1.GET("/system/info", sysH.Info)
	v1.GET("/system/analyze", sysH.Analyze)
	v1.POST("/system/benchmark", sysH.Benchmark)
	v1.GET("/settings", sysH.GetSettings)
	v1.PUT("/settings", sysH.UpdateSettings)

	mediaH := handlers.NewMediaHandler(cfg)
	v1.POST("/media/probe", mediaH.Probe)

	return r
}
