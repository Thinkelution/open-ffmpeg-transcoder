# ============================================
# Open FFmpeg Transcoder - CPU Build
# ============================================

# --- Build Stage ---
FROM golang:1.22-bookworm AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o transcoder ./cmd/transcoder

# --- Runtime Stage ---
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /build/transcoder /app/transcoder

RUN mkdir -p /tmp/transcoder

ENV MODE=all \
    PORT=8090 \
    DATABASE_URL=postgres://transcoder:transcoder@postgres:5432/transcoder?sslmode=disable \
    REDIS_ADDR=redis:6379 \
    TEMP_DIR=/tmp/transcoder \
    MAX_WORKERS=2 \
    FFMPEG_PATH=/usr/bin/ffmpeg \
    FFPROBE_PATH=/usr/bin/ffprobe

EXPOSE 8090

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/transcoder", "healthcheck" ]

ENTRYPOINT ["/app/transcoder"]
CMD ["all"]
