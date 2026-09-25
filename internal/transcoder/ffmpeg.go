package transcoder

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/database"
)

type FFmpeg struct {
	BinaryPath  string
	ProbePath   string
	HWAccelType string // "nvidia", "auto", "none"
}

type ProgressCallback func(progress float32, speed string, fps float32)

func New(ffmpegPath, ffprobePath string) *FFmpeg {
	return &FFmpeg{
		BinaryPath:  ffmpegPath,
		ProbePath:   ffprobePath,
		HWAccelType: "auto",
	}
}

// BuildArgs constructs the full FFmpeg argument list from transcoding settings.
func (f *FFmpeg) BuildArgs(input, output string, settings database.TranscodeSettings, durationSec float64) []string {
	if settings.Format == "hls" && settings.HLS != nil && len(settings.HLS.Renditions) > 0 {
		return f.BuildHLSLadderArgs(input, output, settings, durationSec)
	}

	args := []string{"-y"}

	// Hardware acceleration input flags
	hwaccel := resolveHWAccel(settings.HardwareAccel)
	if hwaccel == "nvidia" {
		args = append(args, "-hwaccel", "cuda", "-hwaccel_output_format", "cuda")
	}

	args = append(args, "-i", input)

	// Video settings
	v := settings.Video
	if v.Codec != "" {
		args = append(args, "-c:v", v.Codec)
	}
	if v.Bitrate != "" {
		args = append(args, "-b:v", v.Bitrate)
	}
	if v.Width > 0 && v.Height > 0 {
		if hwaccel == "nvidia" && isNVENCCodec(v.Codec) {
			args = append(args, "-vf", fmt.Sprintf("scale_cuda=%d:%d", v.Width, v.Height))
		} else {
			args = append(args, "-vf", fmt.Sprintf("scale=%d:%d", v.Width, v.Height))
		}
	}
	if v.Framerate > 0 {
		args = append(args, "-r", strconv.Itoa(v.Framerate))
	}
	if v.Profile != "" {
		args = append(args, "-profile:v", v.Profile)
	}
	if v.PixelFormat != "" {
		args = append(args, "-pix_fmt", v.PixelFormat)
	}

	// Audio settings
	a := settings.Audio
	if a.Codec != "" {
		args = append(args, "-c:a", a.Codec)
	}
	if a.Bitrate != "" {
		args = append(args, "-b:a", a.Bitrate)
	}
	if a.Channels > 0 {
		args = append(args, "-ac", strconv.Itoa(a.Channels))
	}
	if a.SampleRate > 0 {
		args = append(args, "-ar", strconv.Itoa(a.SampleRate))
	}

	// Format
	if settings.Format != "" {
		args = append(args, "-f", settings.Format)
	}

	// Extra flags
	for _, flag := range settings.ExtraFlags {
		args = append(args, flag)
	}

	// Progress output via pipe
	args = append(args, "-progress", "pipe:1", "-stats_period", "1")

	args = append(args, output)
	return args
}

func (f *FFmpeg) BuildHLSLadderArgs(input, output string, settings database.TranscodeSettings, durationSec float64) []string {
	hls := settings.HLS
	outputDir := filepath.Dir(output)
	master := hls.MasterPlaylist
	if master == "" {
		master = filepath.Base(output)
	}
	if master == "." || master == string(filepath.Separator) {
		master = "master.m3u8"
	}

	videoCodec := hls.VideoCodec
	if videoCodec == "" {
		videoCodec = settings.Video.Codec
	}
	if videoCodec == "" {
		videoCodec = "libx264"
	}
	audioCodec := hls.AudioCodec
	if audioCodec == "" {
		audioCodec = settings.Audio.Codec
	}
	if audioCodec == "" {
		audioCodec = "aac"
	}
	defaultAudioBitrate := hls.AudioBitrate
	if defaultAudioBitrate == "" {
		defaultAudioBitrate = settings.Audio.Bitrate
	}
	if defaultAudioBitrate == "" {
		defaultAudioBitrate = "128k"
	}
	segmentSeconds := hls.SegmentSeconds
	if segmentSeconds <= 0 {
		segmentSeconds = 6
	}

	args := []string{"-y", "-i", input}
	var filterParts []string
	var varStreams []string
	validRenditions := make([]database.HLSRendition, 0, len(hls.Renditions))
	for _, rendition := range hls.Renditions {
		width := rendition.Width
		height := rendition.Height
		if width <= 0 || height <= 0 {
			continue
		}
		validRenditions = append(validRenditions, rendition)
	}

	for i, rendition := range validRenditions {
		name := cleanRenditionName(rendition, i)
		width := rendition.Width
		height := rendition.Height
		filterParts = append(filterParts, fmt.Sprintf("[0:v]scale=w=%d:h=%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1[v%d]", width, height, width, height, i))
		varStreams = append(varStreams, fmt.Sprintf("v:%d,a:%d,name:%s", i, i, name))
	}

	if len(filterParts) == 0 {
		return f.BuildArgs(input, output, database.TranscodeSettings{
			Video:      settings.Video,
			Audio:      settings.Audio,
			Format:     settings.Format,
			ExtraFlags: settings.ExtraFlags,
		}, durationSec)
	}

	args = append(args, "-filter_complex", strings.Join(filterParts, ";"))
	for i, rendition := range validRenditions {
		name := cleanRenditionName(rendition, i)
		videoBitrate := rendition.VideoBitrate
		if videoBitrate == "" {
			videoBitrate = settings.Video.Bitrate
		}
		if videoBitrate == "" {
			videoBitrate = "2500k"
		}
		audioBitrate := rendition.AudioBitrate
		if audioBitrate == "" {
			audioBitrate = defaultAudioBitrate
		}
		args = append(args,
			"-map", fmt.Sprintf("[v%d]", i),
			"-map", "0:a:0?",
			fmt.Sprintf("-c:v:%d", i), videoCodec,
			fmt.Sprintf("-b:v:%d", i), videoBitrate,
			fmt.Sprintf("-maxrate:v:%d", i), videoBitrate,
			fmt.Sprintf("-bufsize:v:%d", i), doubleBitrate(videoBitrate),
			fmt.Sprintf("-c:a:%d", i), audioCodec,
			fmt.Sprintf("-b:a:%d", i), audioBitrate,
			fmt.Sprintf("-ac:a:%d", i), "2",
		)
		if hls.Framerate > 0 {
			args = append(args, fmt.Sprintf("-r:v:%d", i), strconv.Itoa(hls.Framerate))
		} else if settings.Video.Framerate > 0 {
			args = append(args, fmt.Sprintf("-r:v:%d", i), strconv.Itoa(settings.Video.Framerate))
		}
		if settings.Video.Profile != "" {
			args = append(args, fmt.Sprintf("-profile:v:%d", i), settings.Video.Profile)
		}
		if settings.Video.PixelFormat != "" {
			args = append(args, fmt.Sprintf("-pix_fmt:v:%d", i), settings.Video.PixelFormat)
		}
		if name == "" {
			name = fmt.Sprintf("v%d", i)
		}
	}

	args = append(args,
		"-f", "hls",
		"-hls_time", strconv.Itoa(segmentSeconds),
		"-hls_playlist_type", "vod",
		"-hls_flags", "independent_segments",
		"-hls_segment_filename", filepath.Join(outputDir, "%v", "segment_%05d.ts"),
		"-master_pl_name", master,
		"-var_stream_map", strings.Join(varStreams, " "),
		"-progress", "pipe:1",
		"-stats_period", "1",
		filepath.Join(outputDir, "%v", "index.m3u8"),
	)
	return args
}

// Run executes the FFmpeg command and reports progress via the callback.
func (f *FFmpeg) Run(ctx context.Context, input, output string, settings database.TranscodeSettings, durationSec float64, cb ProgressCallback) error {
	args := f.BuildArgs(input, output, settings, durationSec)

	log.Printf("[ffmpeg] Running: %s %s", f.BinaryPath, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, f.BinaryPath, args...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	go parseProgress(stdout, durationSec, cb)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("transcoding cancelled")
		}
		return fmt.Errorf("ffmpeg exited with error: %w", err)
	}

	return nil
}

// CheckNVIDIA returns true if nvidia GPU encoding is available.
func (f *FFmpeg) CheckNVIDIA() bool {
	if !hasNVIDIADevice() {
		return false
	}
	cmd := exec.Command(f.BinaryPath, "-hide_banner", "-encoders")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "h264_nvenc")
}

func hasNVIDIADevice() bool {
	if _, err := os.Stat("/dev/nvidiactl"); err == nil {
		return true
	}
	if matches, err := filepath.Glob("/dev/nvidia[0-9]*"); err == nil && len(matches) > 0 {
		return true
	}
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		cmd := exec.Command("nvidia-smi", "-L")
		out, err := cmd.Output()
		return err == nil && strings.Contains(strings.ToLower(string(out)), "gpu")
	}
	return false
}

// GetVersion returns the FFmpeg version string.
func (f *FFmpeg) GetVersion() string {
	cmd := exec.Command(f.BinaryPath, "-version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	lines := strings.SplitN(string(out), "\n", 2)
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return "unknown"
}

// GetEncoders returns the list of available encoders.
func (f *FFmpeg) GetEncoders() []string {
	cmd := exec.Command(f.BinaryPath, "-hide_banner", "-encoders")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var encoders []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 7 && line[0] == 'V' || (len(line) > 7 && line[0] == 'A') {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				encoders = append(encoders, parts[1])
			}
		}
	}
	return encoders
}

// GetDecoders returns the list of available decoders.
func (f *FFmpeg) GetDecoders() []string {
	cmd := exec.Command(f.BinaryPath, "-hide_banner", "-decoders")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var decoders []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 7 && line[0] == 'V' || (len(line) > 7 && line[0] == 'A') {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				decoders = append(decoders, parts[1])
			}
		}
	}
	return decoders
}

// GetFormats returns supported muxer/demuxer formats.
func (f *FFmpeg) GetFormats() []string {
	cmd := exec.Command(f.BinaryPath, "-hide_banner", "-formats")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var formats []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 4 && (line[0] == 'D' || line[0] == 'E' || line[1] == 'E') {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				formats = append(formats, parts[1])
			}
		}
	}
	return formats
}

func resolveHWAccel(setting string) string {
	switch setting {
	case "nvidia":
		return "nvidia"
	case "none", "":
		return "none"
	case "auto":
		// In auto mode, the worker checks for NVIDIA at startup and sets this
		return "none"
	}
	return "none"
}

func isNVENCCodec(codec string) bool {
	return strings.Contains(codec, "nvenc")
}

func cleanRenditionName(r database.HLSRendition, index int) string {
	name := strings.TrimSpace(r.Name)
	if name == "" && r.Height > 0 {
		name = fmt.Sprintf("%dp", r.Height)
	}
	if name == "" {
		name = fmt.Sprintf("v%d", index)
	}
	var b strings.Builder
	for _, ch := range strings.ToLower(name) {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			b.WriteRune(ch)
		}
	}
	if b.Len() == 0 {
		return fmt.Sprintf("v%d", index)
	}
	return b.String()
}

func doubleBitrate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	unit := ""
	num := value
	last := value[len(value)-1]
	if last < '0' || last > '9' {
		unit = string(last)
		num = strings.TrimSpace(value[:len(value)-1])
	}
	n, err := strconv.Atoi(num)
	if err != nil {
		return value
	}
	return fmt.Sprintf("%d%s", n*2, unit)
}

func parseProgress(r io.Reader, totalDuration float64, cb ProgressCallback) {
	scanner := bufio.NewScanner(r)
	var currentTime float64
	var speed string
	var fps float32

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "out_time_us":
			if us, err := strconv.ParseFloat(val, 64); err == nil {
				currentTime = us / 1_000_000
			}
		case "out_time_ms":
			if ms, err := strconv.ParseFloat(val, 64); err == nil {
				currentTime = ms / 1_000_000
			}
		case "out_time":
			currentTime = parseDuration(val)
		case "speed":
			speed = val
		case "fps":
			if f, err := strconv.ParseFloat(val, 32); err == nil {
				fps = float32(f)
			}
		case "progress":
			if totalDuration > 0 && currentTime > 0 {
				pct := float32((currentTime / totalDuration) * 100)
				if pct > 100 {
					pct = 100
				}
				if cb != nil {
					cb(pct, speed, fps)
				}
			}
			if val == "end" {
				if cb != nil {
					cb(100, speed, fps)
				}
			}
		}
	}
}

func parseDuration(s string) float64 {
	// Format: HH:MM:SS.microseconds
	s = strings.TrimSpace(s)
	if s == "" || s == "N/A" {
		return 0
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.ParseFloat(parts[0], 64)
	m, _ := strconv.ParseFloat(parts[1], 64)
	sec, _ := strconv.ParseFloat(parts[2], 64)
	return h*3600 + m*60 + sec
}

// RunBenchmark runs a quick encode benchmark and returns the speed multiplier.
func (f *FFmpeg) RunBenchmark(ctx context.Context, codec string, width, height int) (float64, time.Duration, error) {
	args := []string{
		"-y",
		"-f", "lavfi",
		"-i", fmt.Sprintf("testsrc=duration=10:size=%dx%d:rate=30", width, height),
		"-f", "lavfi",
		"-i", "sine=frequency=1000:duration=10",
		"-c:v", codec,
		"-b:v", "5M",
		"-c:a", "aac",
		"-b:a", "128k",
		"-progress", "pipe:1",
		"-f", "null",
		"-",
	}

	cmd := exec.CommandContext(ctx, f.BinaryPath, args...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, 0, err
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return 0, 0, err
	}

	var lastSpeed string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "speed=") {
			lastSpeed = strings.TrimPrefix(line, "speed=")
		}
	}

	if err := cmd.Wait(); err != nil {
		return 0, 0, err
	}

	elapsed := time.Since(start)

	speedMult := 0.0
	lastSpeed = strings.TrimSpace(lastSpeed)
	lastSpeed = strings.TrimSuffix(lastSpeed, "x")
	if v, err := strconv.ParseFloat(lastSpeed, 64); err == nil {
		speedMult = v
	} else if elapsed.Seconds() > 0 {
		speedMult = 10.0 / elapsed.Seconds()
	}

	return speedMult, elapsed, nil
}
