package transcoder

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type ProbeResult struct {
	Format  ProbeFormat   `json:"format"`
	Streams []ProbeStream `json:"streams"`
}

type ProbeFormat struct {
	Filename       string `json:"filename"`
	FormatName     string `json:"format_name"`
	FormatLongName string `json:"format_long_name"`
	Duration       string `json:"duration"`
	Size           string `json:"size"`
	BitRate        string `json:"bit_rate"`
	ProbeScore     int    `json:"probe_score"`
}

type ProbeStream struct {
	Index          int    `json:"index"`
	CodecName      string `json:"codec_name"`
	CodecLongName  string `json:"codec_long_name"`
	CodecType      string `json:"codec_type"`
	Width          int    `json:"width,omitempty"`
	Height         int    `json:"height,omitempty"`
	DisplayAspect  string `json:"display_aspect_ratio,omitempty"`
	PixFmt         string `json:"pix_fmt,omitempty"`
	FrameRate      string `json:"r_frame_rate,omitempty"`
	AvgFrameRate   string `json:"avg_frame_rate,omitempty"`
	Duration       string `json:"duration,omitempty"`
	BitRate        string `json:"bit_rate,omitempty"`
	SampleRate     string `json:"sample_rate,omitempty"`
	Channels       int    `json:"channels,omitempty"`
	ChannelLayout  string `json:"channel_layout,omitempty"`
}

func (f *FFmpeg) Probe(ctx context.Context, input string) (*ProbeResult, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		input,
	}

	cmd := exec.CommandContext(ctx, f.ProbePath, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe error: %w", err)
	}

	var result ProbeResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	return &result, nil
}

// GetDurationSeconds returns the duration of the media in seconds.
func (f *FFmpeg) GetDurationSeconds(ctx context.Context, input string) (float64, error) {
	result, err := f.Probe(ctx, input)
	if err != nil {
		return 0, err
	}

	if result.Format.Duration != "" {
		var dur float64
		fmt.Sscanf(result.Format.Duration, "%f", &dur)
		if dur > 0 {
			return dur, nil
		}
	}

	// Fallback: check stream durations
	for _, s := range result.Streams {
		if s.Duration != "" {
			var dur float64
			fmt.Sscanf(s.Duration, "%f", &dur)
			if dur > 0 {
				return dur, nil
			}
		}
	}

	return 0, nil
}
