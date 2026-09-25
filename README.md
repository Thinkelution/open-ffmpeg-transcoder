# 1transcoder

A production-grade, API-driven video transcoding service built with Go and FFmpeg. Supports multiple input sources (HTTP, S3, FTP, local), output destinations, NVIDIA GPU acceleration, real-time progress tracking, and a built-in web dashboard. Deployable anywhere with Docker.

## Features

- **Any input, any output** -- Pull from HTTP URLs, S3, FTP, or local files; push results to S3, FTP, or local storage
- **Full API control** -- Every transcoding parameter configurable via REST API
- **Job queue with priorities** -- Redis-backed job queue (asynq) with priority levels, retries, and concurrency control
- **Real-time progress** -- Live progress percentage, encoding speed, and FPS tracking
- **Preset system** -- Built-in presets (H.264/H.265, 480p-4K, GPU) plus custom user presets
- **NVIDIA GPU acceleration** -- NVENC/NVDEC hardware encoding with automatic detection
- **Hardware analyzer** -- Scores your hardware and estimates parallel transcode capacity
- **Web dashboard** -- Built-in monitoring UI with job management and system info
- **Webhook notifications** -- POST callbacks on job completion or failure
- **Wasabi/S3 bucket scanner** -- Watch a bucket prefix, transcode new video objects to HLS once, and upload results under `hls/<unique-video-key>/`
- **Docker-ready** -- One command deployment with Docker Compose (CPU and GPU variants)
- **Single binary** -- API server, worker, and dashboard all in one Go binary

## Quick Start

### Docker (Recommended)

```bash
# Clone the repository
git clone https://github.com/thinkelution/open-ffmpeg-transcoder.git
cd open-ffmpeg-transcoder

# Start everything (API + Worker + PostgreSQL + Redis)
docker compose -f docker/docker-compose.yml up -d

# With NVIDIA GPU support
docker compose -f docker/docker-compose.yml -f docker/docker-compose.gpu.yml up -d
```

The service will be available at `http://localhost:8090`.

### Local Development

Prerequisites: Go 1.22+, PostgreSQL, Redis, FFmpeg

```bash
# Install dependencies
go mod download

# Start PostgreSQL and Redis (via Docker)
make dev-deps

# Copy and edit environment config
cp .env.example .env

# Build and run
make run
```

### Submit Your First Job

```bash
# Transcode a video from URL to local file
curl -X POST http://localhost:8090/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "type": "http",
      "url": "https://sample-videos.com/video321/mp4/720/big_buck_bunny_720p_1mb.mp4"
    },
    "output": {
      "type": "local",
      "url": "/tmp/transcoder/output/result.mp4"
    },
    "settings": {
      "video": {
        "codec": "libx264",
        "bitrate": "2M",
        "width": 1280,
        "height": 720
      },
      "audio": {
        "codec": "aac",
        "bitrate": "128k"
      },
      "format": "mp4",
      "extra_flags": ["-movflags", "+faststart"]
    }
  }'

# Check job status
curl http://localhost:8090/api/v1/jobs/<job-id>
```

## API Overview

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/jobs` | POST | Create a transcoding job |
| `/api/v1/jobs` | GET | List jobs (filter by status, paginate) |
| `/api/v1/jobs/:id` | GET | Get job details and progress |
| `/api/v1/jobs/:id` | DELETE | Cancel or delete a job |
| `/api/v1/jobs/:id/retry` | POST | Retry a failed job |
| `/api/v1/presets` | GET/POST | List or create encoding presets |
| `/api/v1/presets/:id` | GET/PUT/DELETE | Manage individual presets |
| `/api/v1/system/health` | GET | Health check with job counts |
| `/api/v1/system/info` | GET | FFmpeg version, codecs, GPU status |
| `/api/v1/system/analyze` | GET | Hardware analysis with scoring |
| `/api/v1/system/benchmark` | POST | Run encoding benchmark |
| `/api/v1/media/probe` | POST | Probe media file metadata |

See [docs/API.md](docs/API.md) for complete API documentation.

## Architecture

The service runs as a single Go binary with three modes:

- **`all`** (default) -- Runs API server and worker in one process
- **`serve`** -- API server only (for horizontal scaling)
- **`worker`** -- Worker only (for dedicated transcode nodes)

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Clients   │───▶│  API Server │───▶│    Redis     │
│  Dashboard  │    │   (Gin)     │    │  Job Queue   │
└─────────────┘    └──────┬──────┘    └──────┬───────┘
                          │                   │
                   ┌──────▼──────┐    ┌──────▼───────┐
                   │ PostgreSQL  │    │   Workers    │
                   │  Job State  │◀───│  (Asynq)    │
                   └─────────────┘    └──────┬───────┘
                                             │
                                      ┌──────▼───────┐
                                      │   FFmpeg     │
                                      │  + NVIDIA    │
                                      └──────────────┘
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed architecture documentation.

## Configuration

All settings are configured via environment variables. See [.env.example](.env.example) for the full list.

| Variable | Default | Description |
|----------|---------|-------------|
| `MODE` | `all` | Run mode: `serve`, `worker`, or `all` |
| `PORT` | `8090` | API server port |
| `DATABASE_URL` | `postgres://...` | PostgreSQL connection string |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `API_KEY` | (empty) | API key for authentication (disabled if empty) |
| `DASHBOARD_USER` | `admin` | Dashboard login username |
| `DASHBOARD_PASSWORD` | `API_KEY` | Dashboard login password |
| `SESSION_SECRET` | `API_KEY` | Secret used to sign dashboard sessions |
| `MAX_WORKERS` | `2` | Maximum concurrent transcoding jobs |
| `FFMPEG_PATH` | `ffmpeg` | Path to FFmpeg binary |
| `TEMP_DIR` | `/tmp/transcoder` | Temporary directory for intermediate files |
| `S3_REGION` | `us-east-1` | Default S3 region |
| `S3_ACCESS_KEY` | (empty) | Default S3 access key |
| `S3_SECRET_KEY` | (empty) | Default S3 secret key |

### Wasabi/S3 HLS Scanner

Set `SCANNER_ENABLED=true` to watch a Wasabi or S3 bucket for new source videos. The scanner lists `SCANNER_BUCKET` under `SCANNER_INPUT_PREFIX`, skips anything already queued or completed, creates a normal transcode job, and uploads the HLS output to:

```text
s3://<bucket>/<SCANNER_OUTPUT_PREFIX>/<source-name>-<source-key-hash>/index.m3u8
```

For Wasabi, use your bucket's region and endpoint, for example:

```bash
S3_REGION=ap-southeast-1
S3_ENDPOINT=https://s3.ap-southeast-1.wasabisys.com
SCANNER_ENABLED=true
SCANNER_BUCKET=your-bucket
SCANNER_INPUT_PREFIX=incoming
SCANNER_OUTPUT_PREFIX=hls
```

Scanner HLS defaults can be tuned with `HLS_VIDEO_CODEC`, `HLS_FRAMERATE`, `HLS_AUDIO_CODEC`, `HLS_AUDIO_BITRATE`, `HLS_SEGMENT_SECONDS`, and `HLS_LADDER`.

`HLS_LADDER` is a JSON array of renditions. The default ladder is 1080p, 720p, and 480p. Scanner jobs write `master.m3u8` plus one folder per rendition, for example `720p/index.m3u8` with its segments.

These values can also be changed in the dashboard under **Settings** after signing in. The Wasabi secret is write-only from the browser: leave it blank to keep the current value.

Use `SCANNER_OUTPUT_TEMPLATE` when you need a specific destination layout. For example, `file_base_name/hls/*` stores a source file named `trailer.mp4` at `trailer/hls/index.m3u8` with segments in the same folder. Supported template tokens are `file_base_name`, `source_hash`, and `source_dir`.

## NVIDIA GPU Setup

For GPU-accelerated transcoding with NVENC/NVDEC:

1. Install NVIDIA drivers on the host
2. Install [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html)
3. Use the GPU Docker Compose:

```bash
docker compose -f docker/docker-compose.yml -f docker/docker-compose.gpu.yml up -d
```

4. Verify GPU is detected:

```bash
curl http://localhost:8090/api/v1/system/info | jq '.gpu'
curl http://localhost:8090/api/v1/system/analyze | jq '.gpu'
```

5. Use GPU presets or specify NVENC codecs:

```json
{
  "settings": {
    "video": { "codec": "h264_nvenc", "bitrate": "5M" },
    "hardware_accel": "nvidia"
  }
}
```

See [docs/GPU_SETUP.md](docs/GPU_SETUP.md) for detailed GPU configuration.

## Built-in Presets

| Preset | Description |
|--------|-------------|
| `h264_1080p` | H.264 1080p @ 5Mbps |
| `h264_720p` | H.264 720p @ 2.5Mbps |
| `h264_480p` | H.264 480p @ 1Mbps |
| `h265_4k` | H.265/HEVC 4K @ 15Mbps |
| `nvenc_h264_1080p` | NVIDIA NVENC H.264 1080p |
| `nvenc_h265_4k` | NVIDIA NVENC H.265 4K |
| `audio_mp3` | MP3 audio extraction @ 320kbps |
| `audio_aac` | AAC audio extraction @ 256kbps |

## Project Structure

```
open-ffmpeg-transcoder/
├── cmd/transcoder/       # Application entry point
├── internal/
│   ├── api/              # HTTP handlers, middleware, router
│   ├── analyzer/         # Hardware detection and scoring
│   ├── config/           # Environment configuration
│   ├── database/         # PostgreSQL models and queries
│   ├── notify/           # Webhook notifications
│   ├── storage/          # S3, HTTP, FTP, local backends
│   ├── transcoder/       # FFmpeg wrapper and progress parsing
│   └── worker/           # Asynq job processing
├── migrations/           # SQL migration files
├── web/                  # Embedded dashboard (HTML, CSS, JS)
├── docker/               # Dockerfiles and Compose configs
├── docs/                 # Extended documentation
└── Makefile              # Build and development commands
```

## Cloud Deployment

The service is designed for cloud deployment:

- **AWS**: ECS/Fargate with RDS (PostgreSQL) and ElastiCache (Redis). Use GPU instances (p3/g4) for NVENC.
- **GCP**: Cloud Run or GKE with Cloud SQL and Memorystore. Use GPU node pools.
- **Azure**: ACI or AKS with Azure Database for PostgreSQL and Azure Cache for Redis.
- **Any VPS**: Docker Compose works on any Linux server with Docker installed.

For production, run API and worker separately for independent scaling:

```bash
# API server (scale horizontally)
MODE=serve PORT=8090 ./transcoder

# Worker nodes (scale based on GPU/CPU count)
MODE=worker MAX_WORKERS=4 ./transcoder
```

## License

MIT License. See [LICENSE](LICENSE) for details.
