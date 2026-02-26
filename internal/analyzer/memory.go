package analyzer

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type MemoryInfo struct {
	TotalGB     float64 `json:"total_gb"`
	AvailableGB float64 `json:"available_gb"`
	Score       int     `json:"score"`
}

func DetectMemory() MemoryInfo {
	info := MemoryInfo{}

	switch runtime.GOOS {
	case "linux":
		info.TotalGB, info.AvailableGB = readLinuxMemory()
	case "darwin":
		info.TotalGB, info.AvailableGB = readDarwinMemory()
	}

	info.Score = scoreMemory(info)
	return info
}

func readLinuxMemory() (total, available float64) {
	out, err := exec.Command("sh", "-c", "grep -E '^(MemTotal|MemAvailable)' /proc/meminfo").Output()
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		val, _ := strconv.ParseFloat(parts[1], 64)
		valGB := val / 1024 / 1024
		if strings.HasPrefix(line, "MemTotal") {
			total = valGB
		} else if strings.HasPrefix(line, "MemAvailable") {
			available = valGB
		}
	}
	return
}

func readDarwinMemory() (total, available float64) {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, 0
	}
	bytes, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	total = bytes / 1024 / 1024 / 1024

	// macOS doesn't expose "available" easily; estimate as 70% of total
	available = total * 0.7
	return
}

func scoreMemory(info MemoryInfo) int {
	// 4GB = ~30, 8GB = ~50, 16GB = ~65, 32GB = ~80, 64GB+ = ~95
	gb := info.TotalGB
	score := 20 + (gb/64.0)*75
	if score > 95 {
		score = 95
	}
	if score < 10 {
		score = 10
	}
	return int(score)
}
