# NVIDIA GPU Setup Guide

This guide covers setting up NVIDIA GPU acceleration for hardware-accelerated video transcoding with NVENC (encoding) and NVDEC (decoding).

## Prerequisites

1. **NVIDIA GPU** with NVENC support (most GeForce GTX 600+ and Quadro/Tesla cards)
2. **NVIDIA drivers** installed on the host
3. **NVIDIA Container Toolkit** (for Docker GPU passthrough)

## Host Setup

### 1. Install NVIDIA Drivers

**Ubuntu/Debian:**

```bash
sudo apt update
sudo apt install nvidia-driver-535  # or latest version
sudo reboot

# Verify
nvidia-smi
```

**CentOS/RHEL:**

```bash
sudo dnf install nvidia-driver
sudo reboot
nvidia-smi
```

### 2. Install NVIDIA Container Toolkit

```bash
# Add NVIDIA package repository
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | \
  sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg

curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

sudo apt update
sudo apt install -y nvidia-container-toolkit

# Configure Docker runtime
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker

# Test
docker run --rm --gpus all nvidia/cuda:12.3.1-base-ubuntu22.04 nvidia-smi
```

### 3. Verify Container GPU Access

```bash
docker run --rm --gpus all nvidia/cuda:12.3.1-base-ubuntu22.04 nvidia-smi
```

You should see your GPU(s) listed with driver version and CUDA version.

## Deployment

### Docker Compose with GPU

```bash
cd open-ffmpeg-transcoder

# Build and start with GPU support
docker compose -f docker/docker-compose.yml -f docker/docker-compose.gpu.yml up -d
```

The GPU compose override:
- Uses `Dockerfile.gpu` (CUDA-based image with FFmpeg NVENC support)
- Requests all NVIDIA GPUs via `deploy.resources.reservations.devices`
- Increases default `MAX_WORKERS` to 4

### Verify GPU in Transcoder

```bash
# Check system info
curl http://localhost:8090/api/v1/system/info | jq '.gpu'

# Run hardware analysis
curl http://localhost:8090/api/v1/system/analyze | jq '{gpu: .gpu, estimated_parallel_transcodes: .estimated_parallel_transcodes}'

# Benchmark GPU encoding
curl -X POST http://localhost:8090/api/v1/system/benchmark \
  -H "Content-Type: application/json" \
  -d '{"codec": "h264_nvenc", "width": 1920, "height": 1080}'
```

## Using GPU Encoding

### Via Built-in Presets

```bash
# Use NVENC H.264 preset
curl -X POST http://localhost:8090/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "input": {"type": "http", "url": "https://example.com/input.mp4"},
    "output": {"type": "local", "url": "/tmp/transcoder/output/gpu_result.mp4"},
    "settings": {"preset_id": "<nvenc_h264_1080p preset UUID>"}
  }'
```

### Via Direct Settings

```bash
# H.264 NVENC
curl -X POST http://localhost:8090/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "input": {"type": "http", "url": "https://example.com/input.mp4"},
    "output": {"type": "local", "url": "/tmp/transcoder/output/result.mp4"},
    "settings": {
      "video": {
        "codec": "h264_nvenc",
        "bitrate": "5M",
        "width": 1920,
        "height": 1080,
        "profile": "high"
      },
      "audio": {"codec": "aac", "bitrate": "192k"},
      "format": "mp4",
      "hardware_accel": "nvidia",
      "extra_flags": ["-movflags", "+faststart"]
    }
  }'
```

### Available GPU Codecs

| Codec | Encoder | Description |
|-------|---------|-------------|
| H.264 | `h264_nvenc` | NVIDIA NVENC H.264 encoder |
| H.265/HEVC | `hevc_nvenc` | NVIDIA NVENC HEVC encoder |

When `hardware_accel` is set to `nvidia`, the transcoder also uses CUDA-based decoding and scaling, which offloads the entire pipeline to the GPU.

## NVENC Session Limits

NVIDIA consumer GPUs (GeForce) are limited to a small number of concurrent NVENC sessions (typically 3-5). Professional GPUs (Quadro, Tesla, A-series) have no session limits.

If you hit session limits, the transcoder will fall back to CPU encoding for additional jobs.

## Troubleshooting

### GPU not detected

```bash
# Check nvidia-smi works in container
docker compose -f docker/docker-compose.yml -f docker/docker-compose.gpu.yml \
  exec transcoder nvidia-smi

# Check FFmpeg NVENC support
docker compose -f docker/docker-compose.yml -f docker/docker-compose.gpu.yml \
  exec transcoder ffmpeg -encoders 2>/dev/null | grep nvenc
```

### Permission errors

Ensure the NVIDIA Container Toolkit is properly configured:

```bash
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
```

### FFmpeg doesn't have NVENC

The GPU Dockerfile installs FFmpeg from Ubuntu repositories. If NVENC isn't included, you may need to compile FFmpeg from source with `--enable-nvenc` and CUDA SDK headers.

## Cloud GPU Instances

### AWS

- **Instance types**: g4dn (T4), g5 (A10G), p3 (V100), p4 (A100)
- **AMI**: Use Deep Learning AMI (pre-installed NVIDIA drivers)
- **ECS**: Use `gpu` resource requirement in task definition

### GCP

- **Instance types**: n1 + NVIDIA T4/V100/A100
- **Image**: Use Deep Learning VM Image
- **GKE**: Create GPU node pool

### Azure

- **Instance types**: NC-series (T4), ND-series (A100)
- **Image**: Use NVIDIA GPU-optimized VM
- **AKS**: Add GPU node pool with NVIDIA plugin

## Performance Comparison

Typical speed improvements with GPU (NVIDIA T4):

| Task | CPU (libx264) | GPU (h264_nvenc) | Speedup |
|------|--------------|------------------|---------|
| 1080p H.264 | 2-3x realtime | 8-12x realtime | 4-5x |
| 4K H.265 | 0.3-0.5x realtime | 2-4x realtime | 5-8x |
| 720p H.264 | 5-8x realtime | 15-20x realtime | 3-4x |

GPU encoding is significantly faster but may produce slightly larger files at equivalent visual quality. For the best quality-to-speed tradeoff, use GPU encoding with a slightly higher bitrate than you would with CPU.
