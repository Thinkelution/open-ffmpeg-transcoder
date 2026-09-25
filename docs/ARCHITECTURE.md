# Architecture

## System Overview

1transcoder is a single Go binary that can run in three modes:

| Mode | Description | Use Case |
|------|-------------|----------|
| `all` | API server + worker in one process | Development, small deployments |
| `serve` | API server only | Production: scale API horizontally |
| `worker` | Worker only | Production: dedicated transcode nodes |

```
                          ┌──────────────────────────────────┐
                          │          Load Balancer           │
                          └──────────┬───────────────────────┘
                                     │
              ┌──────────────────────┼──────────────────────┐
              │                      │                      │
     ┌────────▼─────────┐  ┌────────▼─────────┐  ┌────────▼─────────┐
     │  API Server #1   │  │  API Server #2   │  │  API Server #3   │
     │  (mode=serve)    │  │  (mode=serve)    │  │  (mode=serve)    │
     └────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘
              │                      │                      │
              └──────────────────────┼──────────────────────┘
                                     │
              ┌──────────────────────┼──────────────────────┐
              │                      │                      │
     ┌────────▼─────────┐  ┌────────▼─────────┐           │
     │   PostgreSQL     │  │     Redis        │           │
     │   (Job State)    │  │   (Job Queue)    │           │
     └──────────────────┘  └────────┬─────────┘           │
                                     │                      │
              ┌──────────────────────┼──────────────────────┘
              │                      │
     ┌────────▼─────────┐  ┌────────▼─────────┐
     │  Worker Node #1  │  │  Worker Node #2  │
     │  (mode=worker)   │  │  (mode=worker)   │
     │  CPU transcoding │  │  GPU + CPU       │
     └──────────────────┘  └──────────────────┘
```

## Component Details

### API Server (`internal/api/`)

The API server uses [Gin](https://github.com/gin-gonic/gin) as the HTTP framework. It handles:

- **Job management**: CRUD operations for transcoding jobs
- **Preset management**: Encoding preset creation and management
- **System endpoints**: Health checks, FFmpeg info, hardware analysis
- **Media probing**: File metadata extraction via ffprobe
- **Web dashboard**: Serves embedded HTML templates with live data
- **Authentication**: Optional API key auth via `X-API-Key` header or `Bearer` token
- **Rate limiting**: Per-IP request throttling (300 req/min default)

### Worker (`internal/worker/`)

Workers process transcoding jobs from the Redis queue using [asynq](https://github.com/hibiken/asynq). The processing pipeline:

1. **Dequeue** -- Pull job from Redis priority queue
2. **Download** -- Fetch input file from configured source (S3/HTTP/FTP/local)
3. **Probe** -- Extract media metadata with ffprobe
4. **Transcode** -- Run FFmpeg with configured settings, report progress
5. **Upload** -- Push output to configured destination
6. **Notify** -- Send webhook callback if configured
7. **Cleanup** -- Remove temporary files

Worker concurrency is controlled by `MAX_WORKERS`. Each worker processes one job at a time. Priority queues (`critical`, `default`, `low`) ensure important jobs are handled first.

### FFmpeg Wrapper (`internal/transcoder/`)

The FFmpeg wrapper manages:

- **Command building** -- Constructs FFmpeg arguments from API settings (codec, bitrate, resolution, etc.)
- **GPU detection** -- Detects NVIDIA NVENC/NVDEC availability
- **Progress parsing** -- Parses FFmpeg's `-progress pipe:1` output for real-time progress, speed, and FPS
- **Media probing** -- Wraps ffprobe for media metadata extraction
- **Benchmarking** -- Generates test patterns and measures encoding speed

FFmpeg is invoked as a subprocess (not via cgo), making it version-independent and simpler to maintain.

### Storage (`internal/storage/`)

Pluggable storage backends implement two interfaces:

```go
type Downloader interface {
    Download(ctx context.Context, localPath string) error
}

type Uploader interface {
    Upload(ctx context.Context, localPath string) error
}
```

| Backend | Download | Upload | Notes |
|---------|----------|--------|-------|
| HTTP/HTTPS | Yes | No | Any publicly accessible URL |
| S3 | Yes | Yes | AWS S3, MinIO, any S3-compatible |
| FTP | Yes | Yes | Standard FTP with auth |
| Local | Yes | Yes | Local filesystem paths |

S3 credentials can be set globally (environment variables) or per-job (in the request payload).

### Database (`internal/database/`)

PostgreSQL stores:

- **Jobs** -- Status, progress, input/output config, settings, timing, error messages
- **Presets** -- Named encoding configurations (system + user-created)
- **Schema migrations** -- Auto-applied on startup

Migrations are embedded in the binary and run automatically.

### Hardware Analyzer (`internal/analyzer/`)

The analyzer evaluates the deployment hardware:

- **CPU** -- Model detection, core/thread count, architecture-based scoring
- **Memory** -- Total/available RAM detection
- **GPU** -- NVIDIA GPU detection via `nvidia-smi`, NVENC/NVDEC capability
- **Disk I/O** -- Sequential read/write benchmark (64MB test)
- **Scoring** -- Weighted composite score (CPU 35%, Memory 20%, GPU 25%, Disk 20%)
- **Estimates** -- Parallel transcode capacity and encoding speed predictions

### Webhook Notifications (`internal/notify/`)

When a job completes (or fails), a POST request is sent to the configured `webhook_url`:

```json
{
  "event": "job.completed",
  "job": { /* full job object */ },
  "timestamp": "2025-01-15T10:30:00Z"
}
```

Retries with exponential backoff (configurable max retries).

## Data Flow

### Job Lifecycle

```
Created ──▶ Pending ──▶ Downloading ──▶ Transcoding ──▶ Uploading ──▶ Completed
                │              │               │              │
                └──────────────┴───────────────┴──────────────┘
                                       │
                                  Failed / Cancelled
```

### Progress Tracking

1. FFmpeg outputs progress to stdout via `-progress pipe:1`
2. Worker parses `out_time`, `speed`, and `fps` fields
3. Progress percentage calculated from `out_time / total_duration`
4. Database updated every second with current progress
5. Dashboard polls API every 3 seconds for live updates

## Security

- **API Key Authentication** -- Optional, set `API_KEY` environment variable
- **Rate Limiting** -- 300 requests/minute per IP
- **CORS** -- Configurable (defaults to allow all for development)
- **Input validation** -- All API inputs validated via Gin's binding
- **Temporary file cleanup** -- Job working directories removed after completion
- **No credential storage** -- S3/FTP credentials are used transiently, never persisted

## Scaling Strategy

| Component | Scaling Method | Notes |
|-----------|---------------|-------|
| API Server | Horizontal (multiple instances) | Stateless, shared database |
| Worker | Horizontal (add nodes) | Each node processes `MAX_WORKERS` concurrent jobs |
| PostgreSQL | Vertical / Read replicas | Single writer, read scaling for job listings |
| Redis | Single instance / Cluster | Asynq supports Redis Cluster for high throughput |
