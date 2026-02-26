# API Reference

Base URL: `http://localhost:8090/api/v1`

## Authentication

If `API_KEY` is set, all API requests require authentication via one of:

- Header: `X-API-Key: your-api-key`
- Header: `Authorization: Bearer your-api-key`

## Jobs

### Create Job

```
POST /api/v1/jobs
```

Creates a new transcoding job and enqueues it for processing.

**Request Body:**

```json
{
  "input": {
    "type": "http",
    "url": "https://example.com/video.mp4",
    "credentials": {}
  },
  "output": {
    "type": "s3",
    "url": "s3://my-bucket/output/video.mp4",
    "credentials": {
      "access_key_id": "AKIA...",
      "secret_access_key": "...",
      "region": "us-east-1"
    }
  },
  "settings": {
    "preset_id": "",
    "video": {
      "codec": "libx264",
      "bitrate": "5M",
      "width": 1920,
      "height": 1080,
      "framerate": 30,
      "profile": "high",
      "pixel_format": "yuv420p"
    },
    "audio": {
      "codec": "aac",
      "bitrate": "192k",
      "channels": 2,
      "sample_rate": 48000
    },
    "format": "mp4",
    "hardware_accel": "auto",
    "extra_flags": ["-movflags", "+faststart"]
  },
  "priority": 5,
  "webhook_url": "https://example.com/callback",
  "metadata": {"user_id": "123"}
}
```

**Input Types:**

| Type | URL Format | Credentials |
|------|-----------|-------------|
| `http` / `https` | `https://example.com/file.mp4` | Not required |
| `s3` | `s3://bucket/key` | `access_key_id`, `secret_access_key`, `region`, `endpoint` |
| `ftp` | `ftp://host/path/file.mp4` | `host`, `port`, `username`, `password` |
| `local` | `/path/to/file.mp4` | Not required |

**Output Types:**

| Type | URL Format | Credentials |
|------|-----------|-------------|
| `s3` | `s3://bucket/key` | Same as input |
| `ftp` | `ftp://host/path/file.mp4` | Same as input |
| `local` | `/path/to/output.mp4` | Not required |

**Video Codecs:**

| Codec | Type | Description |
|-------|------|-------------|
| `libx264` | CPU | H.264/AVC software encoder |
| `libx265` | CPU | H.265/HEVC software encoder |
| `h264_nvenc` | GPU | NVIDIA H.264 hardware encoder |
| `hevc_nvenc` | GPU | NVIDIA H.265 hardware encoder |
| `libvpx-vp9` | CPU | VP9 software encoder |
| `libaom-av1` | CPU | AV1 software encoder |

**Priority:** 1 (lowest) to 10 (highest). Jobs with priority >= 8 go to the `critical` queue, <= 2 to `low`.

**Response:** `201 Created`

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "progress": 0,
  "created_at": "2025-01-15T10:00:00Z"
}
```

### List Jobs

```
GET /api/v1/jobs?status=transcoding&limit=50&offset=0
```

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `status` | string | (all) | Filter: `pending`, `downloading`, `transcoding`, `uploading`, `completed`, `failed`, `cancelled` |
| `limit` | int | 50 | Max results (1-200) |
| `offset` | int | 0 | Pagination offset |

**Response:** `200 OK`

```json
{
  "jobs": [...],
  "total": 42,
  "limit": 50,
  "offset": 0
}
```

### Get Job

```
GET /api/v1/jobs/:id
```

Returns full job details including progress, timing, and media info.

**Response:** `200 OK`

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "transcoding",
  "progress": 45.2,
  "speed": "2.5x",
  "fps": 75.3,
  "eta": "",
  "input": {...},
  "output": {...},
  "settings": {...},
  "priority": 5,
  "input_info": { "format": {...}, "streams": [...] },
  "output_info": null,
  "started_at": "2025-01-15T10:00:05Z",
  "completed_at": null,
  "duration_ms": 0,
  "created_at": "2025-01-15T10:00:00Z"
}
```

### Delete Job

```
DELETE /api/v1/jobs/:id
```

Cancels a running job or deletes a completed/failed job.

### Retry Job

```
POST /api/v1/jobs/:id/retry
```

Re-enqueues a failed or cancelled job. Resets progress to 0.

---

## Presets

### Create Preset

```
POST /api/v1/presets
```

```json
{
  "name": "my_custom_preset",
  "description": "Custom 720p preset for web delivery",
  "settings": {
    "video": {
      "codec": "libx264",
      "bitrate": "2M",
      "width": 1280,
      "height": 720,
      "profile": "main"
    },
    "audio": {
      "codec": "aac",
      "bitrate": "128k",
      "channels": 2
    },
    "format": "mp4",
    "extra_flags": ["-movflags", "+faststart"]
  }
}
```

### List Presets

```
GET /api/v1/presets
```

Returns all presets (system presets first, then user-created).

### Get Preset

```
GET /api/v1/presets/:id
```

### Update Preset

```
PUT /api/v1/presets/:id
```

Only user-created presets can be updated. System presets are immutable.

### Delete Preset

```
DELETE /api/v1/presets/:id
```

Only user-created presets can be deleted.

---

## System

### Health Check

```
GET /api/v1/system/health
```

```json
{
  "status": "ok",
  "version": "1.0.0",
  "jobs": {
    "pending": 2,
    "transcoding": 1,
    "completed": 45,
    "failed": 3
  }
}
```

### System Info

```
GET /api/v1/system/info
```

```json
{
  "ffmpeg_version": "ffmpeg version 6.1.1 ...",
  "encoders": ["libx264", "libx265", "aac", "h264_nvenc", ...],
  "decoders": ["h264", "hevc", "aac", ...],
  "formats": ["mp4", "mkv", "webm", ...],
  "gpu": {
    "nvidia_available": true
  },
  "config": {
    "max_workers": 4,
    "temp_dir": "/tmp/transcoder"
  }
}
```

### Hardware Analysis

```
GET /api/v1/system/analyze
```

Returns a comprehensive hardware analysis with scoring and capacity estimates.

```json
{
  "score": 78,
  "rating": "Great",
  "cpu": {
    "model": "AMD EPYC 7R32",
    "cores": 8,
    "threads": 16,
    "arch": "amd64",
    "score": 72
  },
  "memory": {
    "total_gb": 32,
    "available_gb": 28,
    "score": 80
  },
  "gpu": {
    "available": true,
    "devices": [
      {
        "index": 0,
        "name": "NVIDIA T4",
        "memory_gb": 16,
        "nvenc": true,
        "nvdec": true,
        "driver_version": "535.104.05"
      }
    ],
    "score": 90
  },
  "disk": {
    "read_mbps": 500,
    "write_mbps": 450,
    "score": 80
  },
  "estimated_parallel_transcodes": {
    "cpu_1080p_h264": 8,
    "gpu_1080p_h264": 10,
    "cpu_4k_h265": 2,
    "gpu_4k_h265": 3
  },
  "estimated_speed": {
    "1080p_h264_cpu": "4x realtime",
    "1080p_h264_gpu": "6-10x realtime",
    "4k_h265_cpu": "0.8x realtime",
    "4k_h265_gpu": "1.5-3x realtime"
  }
}
```

### Run Benchmark

```
POST /api/v1/system/benchmark
```

```json
{
  "codec": "libx264",
  "width": 1920,
  "height": 1080
}
```

Runs a 10-second test pattern encode and measures real-world performance.

**Response:**

```json
{
  "codec": "libx264",
  "resolution": "1920x1080",
  "speed_factor": 3.45,
  "elapsed_ms": 2899,
  "status": "completed"
}
```

---

## Media

### Probe Media

```
POST /api/v1/media/probe
```

```json
{
  "input": {
    "type": "http",
    "url": "https://example.com/video.mp4"
  }
}
```

Returns ffprobe output (format info and stream details).

---

## Webhook Payload

When a job completes, fails, or is cancelled, a POST request is sent to the `webhook_url`:

```json
{
  "event": "job.completed",
  "job": {
    "id": "...",
    "status": "completed",
    "progress": 100,
    "duration_ms": 15234,
    "input_info": {...},
    "output_info": {...}
  },
  "timestamp": "2025-01-15T10:05:00Z"
}
```

Events: `job.completed`, `job.failed`, `job.cancelled`

The webhook is retried with exponential backoff (1s, 2s, 4s...) up to the configured maximum retries.

---

## Error Responses

All errors follow the format:

```json
{
  "error": "description of what went wrong"
}
```

| Status Code | Description |
|-------------|-------------|
| 400 | Bad request / validation error |
| 401 | Missing or invalid API key |
| 404 | Resource not found |
| 429 | Rate limit exceeded |
| 500 | Internal server error |
