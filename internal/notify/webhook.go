package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
)

type WebhookPayload struct {
	Event     string        `json:"event"`
	Job       *database.Job `json:"job"`
	Timestamp time.Time     `json:"timestamp"`
}

// SendWebhook sends a POST request to the webhook URL with the job data.
// Retries up to maxRetries times with exponential backoff.
func SendWebhook(ctx context.Context, webhookURL string, job *database.Job, event string, maxRetries int) {
	if webhookURL == "" {
		return
	}

	payload := WebhookPayload{
		Event:     event,
		Job:       job,
		Timestamp: time.Now().UTC(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[webhook] Failed to marshal payload for job %s: %v", job.ID, err)
		return
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
		if err != nil {
			log.Printf("[webhook] Failed to create request for job %s: %v", job.ID, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "1transcoder/1.0")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[webhook] Attempt %d failed for job %s: %v", attempt+1, job.ID, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("[webhook] Successfully notified %s for job %s (status %d)", webhookURL, job.ID, resp.StatusCode)
			return
		}

		log.Printf("[webhook] Attempt %d returned status %d for job %s", attempt+1, resp.StatusCode, job.ID)
	}

	log.Printf("[webhook] All %d attempts failed for job %s to %s", maxRetries+1, job.ID, webhookURL)
}

// NotifyJobComplete sends a webhook for job completion.
func NotifyJobComplete(ctx context.Context, webhookURL string, job *database.Job, maxRetries int) {
	event := "job.completed"
	if job.Status == database.JobStatusFailed {
		event = "job.failed"
	} else if job.Status == database.JobStatusCancelled {
		event = "job.cancelled"
	}
	go SendWebhook(ctx, webhookURL, job, event, maxRetries)
}

// FormatDuration formats milliseconds into a human-readable string.
func FormatDuration(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	if d < time.Second {
		return fmt.Sprintf("%dms", ms)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}
