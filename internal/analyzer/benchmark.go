package analyzer

import (
	"context"
	"fmt"
	"time"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/transcoder"
)

type BenchmarkResult struct {
	Codec       string  `json:"codec"`
	Resolution  string  `json:"resolution"`
	SpeedFactor float64 `json:"speed_factor"`
	ElapsedMs   int64   `json:"elapsed_ms"`
	Status      string  `json:"status"`
}

// RunBenchmark runs a quick transcode test with the specified codec and resolution.
func RunBenchmark(ctx context.Context, ff *transcoder.FFmpeg, codec string, width, height int) (*BenchmarkResult, error) {
	if codec == "" {
		codec = "libx264"
	}
	if width == 0 {
		width = 1920
	}
	if height == 0 {
		height = 1080
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	speed, elapsed, err := ff.RunBenchmark(ctx, codec, width, height)
	if err != nil {
		return &BenchmarkResult{
			Codec:      codec,
			Resolution: fmt.Sprintf("%dx%d", width, height),
			Status:     fmt.Sprintf("failed: %v", err),
		}, err
	}

	return &BenchmarkResult{
		Codec:       codec,
		Resolution:  fmt.Sprintf("%dx%d", width, height),
		SpeedFactor: speed,
		ElapsedMs:   elapsed.Milliseconds(),
		Status:      "completed",
	}, nil
}
