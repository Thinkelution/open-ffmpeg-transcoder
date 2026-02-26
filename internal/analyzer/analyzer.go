package analyzer

import (
	"context"
	"math"

	"github.com/thinkelution/open-ffmpeg-transcoder/internal/transcoder"
)

type AnalysisResult struct {
	Score   int    `json:"score"`
	Rating  string `json:"rating"`
	CPU     CPUInfo    `json:"cpu"`
	Memory  MemoryInfo `json:"memory"`
	GPU     GPUInfo    `json:"gpu"`
	Disk    DiskInfo   `json:"disk"`
	EstimatedParallel EstimatedParallel `json:"estimated_parallel_transcodes"`
	EstimatedSpeed    EstimatedSpeed    `json:"estimated_speed"`
}

type EstimatedParallel struct {
	CPU1080pH264 int `json:"cpu_1080p_h264"`
	GPU1080pH264 int `json:"gpu_1080p_h264"`
	CPU4kH265    int `json:"cpu_4k_h265"`
	GPU4kH265    int `json:"gpu_4k_h265"`
}

type EstimatedSpeed struct {
	CPU1080pH264 string `json:"1080p_h264_cpu"`
	GPU1080pH264 string `json:"1080p_h264_gpu"`
	CPU4kH265    string `json:"4k_h265_cpu"`
	GPU4kH265    string `json:"4k_h265_gpu"`
}

type Analyzer struct {
	ff *transcoder.FFmpeg
}

func New(ff *transcoder.FFmpeg) *Analyzer {
	return &Analyzer{ff: ff}
}

func (a *Analyzer) Analyze(ctx context.Context) (*AnalysisResult, error) {
	cpu := DetectCPU()
	mem := DetectMemory()
	gpu := DetectGPU()
	disk := BenchmarkDisk(ctx)

	// Compute overall score as weighted average
	weights := map[string]float64{
		"cpu":    0.35,
		"memory": 0.20,
		"gpu":    0.25,
		"disk":   0.20,
	}

	gpuScore := gpu.Score
	if !gpu.Available {
		// Redistribute GPU weight to CPU
		weights["cpu"] = 0.50
		weights["memory"] = 0.25
		weights["disk"] = 0.25
		weights["gpu"] = 0
		gpuScore = 0
	}

	overall := weights["cpu"]*float64(cpu.Score) +
		weights["memory"]*float64(mem.Score) +
		weights["gpu"]*float64(gpuScore) +
		weights["disk"]*float64(disk.Score)

	score := int(math.Round(overall))

	est := estimateParallel(cpu, mem, gpu)
	speed := estimateSpeed(cpu, gpu)

	return &AnalysisResult{
		Score:             score,
		Rating:            scoreRating(score),
		CPU:               cpu,
		Memory:            mem,
		GPU:               gpu,
		Disk:              disk,
		EstimatedParallel: est,
		EstimatedSpeed:    speed,
	}, nil
}

func estimateParallel(cpu CPUInfo, mem MemoryInfo, gpu GPUInfo) EstimatedParallel {
	// Rough heuristics based on real-world observations:
	// 1080p H.264 CPU: ~2 cores per stream, needs ~2GB RAM each
	// 4K H.265 CPU: ~8 cores per stream, needs ~4GB RAM each
	// GPU: depends on NVENC session limits (consumer ~3, pro ~unlimited)

	cpuStreams1080 := cpu.Threads / 2
	memStreams1080 := int(mem.AvailableGB / 2)
	cpu1080 := min(cpuStreams1080, memStreams1080)
	if cpu1080 < 1 {
		cpu1080 = 1
	}

	cpuStreams4k := cpu.Threads / 8
	memStreams4k := int(mem.AvailableGB / 4)
	cpu4k := min(cpuStreams4k, memStreams4k)
	if cpu4k < 1 {
		cpu4k = 1
	}

	gpu1080 := 0
	gpu4k := 0
	if gpu.Available && len(gpu.Devices) > 0 {
		for _, d := range gpu.Devices {
			if d.NVENC {
				sessionsPerGPU := int(d.MemoryGB / 1.5)
				if sessionsPerGPU > 8 {
					sessionsPerGPU = 8
				}
				gpu1080 += sessionsPerGPU
				gpu4k += max(sessionsPerGPU/3, 1)
			}
		}
	}

	return EstimatedParallel{
		CPU1080pH264: cpu1080,
		GPU1080pH264: gpu1080,
		CPU4kH265:    cpu4k,
		GPU4kH265:    gpu4k,
	}
}

func estimateSpeed(cpu CPUInfo, gpu GPUInfo) EstimatedSpeed {
	// Rough speed estimates based on core count and GPU presence
	cpuSpeed1080 := float64(cpu.Threads) * 0.25
	if cpuSpeed1080 > 10 {
		cpuSpeed1080 = 10
	}
	cpuSpeed4k := float64(cpu.Threads) * 0.05
	if cpuSpeed4k > 2 {
		cpuSpeed4k = 2
	}

	gpuSpeed1080 := "N/A"
	gpuSpeed4k := "N/A"
	if gpu.Available && len(gpu.Devices) > 0 {
		gpuSpeed1080 = "6-10x realtime"
		gpuSpeed4k = "1.5-3x realtime"
	}

	return EstimatedSpeed{
		CPU1080pH264: formatSpeed(cpuSpeed1080),
		GPU1080pH264: gpuSpeed1080,
		CPU4kH265:    formatSpeed(cpuSpeed4k),
		GPU4kH265:    gpuSpeed4k,
	}
}

func scoreRating(score int) string {
	switch {
	case score >= 90:
		return "Excellent"
	case score >= 75:
		return "Great"
	case score >= 60:
		return "Good"
	case score >= 40:
		return "Fair"
	default:
		return "Limited"
	}
}

func formatSpeed(x float64) string {
	if x >= 1.0 {
		return fmtFloat(x) + "x realtime"
	}
	return fmtFloat(x) + "x realtime"
}

func fmtFloat(f float64) string {
	if f == float64(int(f)) {
		return intToStr(int(f))
	}
	// Simple formatting without importing strconv at module level
	whole := int(f)
	frac := int((f - float64(whole)) * 10)
	return intToStr(whole) + "." + intToStr(frac)
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
