package analyzer

import (
	"os/exec"
	"runtime"
	"strings"
)

type CPUInfo struct {
	Model   string `json:"model"`
	Cores   int    `json:"cores"`
	Threads int    `json:"threads"`
	Arch    string `json:"arch"`
	Score   int    `json:"score"`
}

func DetectCPU() CPUInfo {
	info := CPUInfo{
		Cores:   runtime.NumCPU(),
		Threads: runtime.NumCPU(),
		Arch:    runtime.GOARCH,
	}

	switch runtime.GOOS {
	case "linux":
		info.Model = readLinuxCPUModel()
		info.Threads = readLinuxCPUThreads(info.Cores)
	case "darwin":
		info.Model = readDarwinCPUModel()
	default:
		info.Model = "Unknown"
	}

	info.Score = scoreCPU(info)
	return info
}

func readLinuxCPUModel() string {
	out, err := exec.Command("sh", "-c", "grep -m1 'model name' /proc/cpuinfo | cut -d: -f2").Output()
	if err != nil {
		return "Unknown"
	}
	return strings.TrimSpace(string(out))
}

func readLinuxCPUThreads(fallback int) int {
	out, err := exec.Command("nproc").Output()
	if err != nil {
		return fallback
	}
	s := strings.TrimSpace(string(out))
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	if n > 0 {
		return n
	}
	return fallback
}

func readDarwinCPUModel() string {
	out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
	if err != nil {
		return "Unknown"
	}
	return strings.TrimSpace(string(out))
}

func scoreCPU(info CPUInfo) int {
	// Score based on thread count (primary factor for transcoding throughput)
	// 4 threads = ~30, 8 = ~50, 16 = ~70, 32 = ~85, 64+ = ~95
	threads := float64(info.Threads)
	score := 20 + (threads/64.0)*75
	if score > 95 {
		score = 95
	}

	model := strings.ToLower(info.Model)
	if strings.Contains(model, "epyc") || strings.Contains(model, "xeon") || strings.Contains(model, "threadripper") {
		score += 5
	}
	if strings.Contains(model, "apple m") {
		score += 3
	}

	if score > 100 {
		score = 100
	}
	return int(score)
}
