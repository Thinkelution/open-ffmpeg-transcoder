package worker

import (
	"context"
	"log"

	"github.com/hibiken/asynq"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/config"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
	"github.com/thinkelution/open-ffmpeg-transcoder/internal/transcoder"
)

const (
	TypeTranscode = "transcode"
)

type Worker struct {
	server *asynq.Server
	mux    *asynq.ServeMux
}

func New(ctx context.Context, cfg *config.Config, db *database.DB) (*Worker, error) {
	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
	}

	server := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.MaxWorkers,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		},
		Logger: &asynqLogger{},
	})

	ff := transcoder.New(cfg.FFmpegPath, cfg.FFprobePath)
	handler := NewHandler(ctx, cfg, db, ff)

	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeTranscode, handler.HandleTranscode)

	return &Worker{
		server: server,
		mux:    mux,
	}, nil
}

func (w *Worker) Start() error {
	return w.server.Run(w.mux)
}

func (w *Worker) Stop() {
	w.server.Stop()
	w.server.Shutdown()
}

// EnqueueJob enqueues a transcoding job into Redis via asynq.
func EnqueueJob(cfg *config.Config, jobID string, priority int) error {
	client := asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
	})
	defer client.Close()

	task := asynq.NewTask(TypeTranscode, []byte(jobID))

	queue := "default"
	if priority >= 8 {
		queue = "critical"
	} else if priority <= 2 {
		queue = "low"
	}

	_, err := client.Enqueue(task,
		asynq.Queue(queue),
		asynq.MaxRetry(2),
	)
	if err != nil {
		return err
	}

	log.Printf("[worker] Enqueued job %s on queue %s", jobID, queue)
	return nil
}

type asynqLogger struct{}

func (l *asynqLogger) Debug(args ...interface{})                    { log.Println(args...) }
func (l *asynqLogger) Info(args ...interface{})                     { log.Println(args...) }
func (l *asynqLogger) Warn(args ...interface{})                     { log.Println(args...) }
func (l *asynqLogger) Error(args ...interface{})                    { log.Println(args...) }
func (l *asynqLogger) Fatal(args ...interface{})                    { log.Fatal(args...) }
