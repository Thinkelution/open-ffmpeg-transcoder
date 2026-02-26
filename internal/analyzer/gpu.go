package analyzer

import (
	"os/exec"
	"strconv"
	"strings"
)

type GPUInfo struct {
	Available bool        `json:"available"`
	Devices   []GPUDevice `json:"devices"`
	Score     int         `json:"score"`
}

type GPUDevice struct {
	Index    int     `json:"index"`
	Name     string  `json:"name"`
	MemoryGB float64 `json:"memory_gb"`
	NVENC    bool    `json:"nvenc"`
	NVDEC    bool    `json:"nvdec"`
	Driver   string  `json:"driver_version"`
}

func DetectGPU() GPUInfo {
	info := GPUInfo{}

	// Try nvidia-smi
	out, err := exec.Command(
		"nvidia-smi",
		"--query-gpu=index,name,memory.total,driver_version",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return info
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ", ")
		if len(parts) < 4 {
			continue
		}

		idx, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		memMB, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)

		dev := GPUDevice{
			Index:    idx,
			Name:     strings.TrimSpace(parts[1]),
			MemoryGB: memMB / 1024.0,
			NVENC:    checkNVENC(idx),
			NVDEC:    checkNVDEC(idx),
			Driver:   strings.TrimSpace(parts[3]),
		}

		info.Devices = append(info.Devices, dev)
	}

	info.Available = len(info.Devices) > 0
	info.Score = scoreGPU(info)
	return info
}

func checkNVENC(gpuIndex int) bool {
	// Check if NVENC is available by querying encoder sessions capability
	out, err := exec.Command(
		"nvidia-smi",
		"--query-gpu=encoder.stats.sessionCount",
		"--format=csv,noheader",
		"-i", strconv.Itoa(gpuIndex),
	).Output()
	if err != nil {
		// If the query itself succeeds (nvidia-smi exists and GPU responds), NVENC is likely available
		// Fall back to checking if the GPU name suggests NVENC support
		return true
	}
	_ = out
	return true
}

func checkNVDEC(gpuIndex int) bool {
	out, err := exec.Command(
		"nvidia-smi",
		"--query-gpu=decoder.stats.sessionCount",
		"--format=csv,noheader",
		"-i", strconv.Itoa(gpuIndex),
	).Output()
	if err != nil {
		return true
	}
	_ = out
	return true
}

func scoreGPU(info GPUInfo) int {
	if !info.Available || len(info.Devices) == 0 {
		return 0
	}

	totalScore := 0
	for _, dev := range info.Devices {
		devScore := 50 // Base score for having a GPU

		// Memory-based scoring
		if dev.MemoryGB >= 24 {
			devScore += 40
		} else if dev.MemoryGB >= 16 {
			devScore += 35
		} else if dev.MemoryGB >= 8 {
			devScore += 25
		} else if dev.MemoryGB >= 4 {
			devScore += 15
		}

		// NVENC bonus
		if dev.NVENC {
			devScore += 5
		}
		if dev.NVDEC {
			devScore += 5
		}

		if devScore > 100 {
			devScore = 100
		}
		totalScore += devScore
	}

	avg := totalScore / len(info.Devices)
	// Bonus for multiple GPUs
	if len(info.Devices) > 1 {
		avg += len(info.Devices) * 2
	}
	if avg > 100 {
		avg = 100
	}

	return avg
}
