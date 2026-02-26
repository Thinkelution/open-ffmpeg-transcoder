package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/api"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/worker"
)

func main() {
	cfg := config.Load()

	mode := cfg.Mode
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	if err := os.MkdirAll(cfg.TempDir, 0755); err != nil {
		log.Fatalf("Failed to create temp directory: %v", err)
	}

	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var w *worker.Worker
	var srv *http.Server

	switch mode {
	case "serve":
		srv = startAPI(cfg, db)
	case "worker":
		w = startWorker(ctx, cfg, db)
	case "all":
		w = startWorker(ctx, cfg, db)
		srv = startAPI(cfg, db)
	default:
		fmt.Fprintf(os.Stderr, "Usage: %s [serve|worker|all]\n", os.Args[0])
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	if srv != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}

	if w != nil {
		w.Stop()
	}

	cancel()
	log.Println("Shutdown complete.")
}

func startAPI(cfg *config.Config, db *database.DB) *http.Server {
	router := api.NewRouter(cfg, db)
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("API server starting on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("API server error: %v", err)
		}
	}()

	return srv
}

func startWorker(ctx context.Context, cfg *config.Config, db *database.DB) *worker.Worker {
	w, err := worker.New(ctx, cfg, db)
	if err != nil {
		log.Fatalf("Failed to create worker: %v", err)
	}

	go func() {
		log.Printf("Worker starting with %d max concurrent jobs", cfg.MaxWorkers)
		if err := w.Start(); err != nil {
			log.Fatalf("Worker error: %v", err)
		}
	}()

	return w
}
