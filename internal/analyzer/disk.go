package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

type DiskInfo struct {
	ReadMBps  float64 `json:"read_mbps"`
	WriteMBps float64 `json:"write_mbps"`
	Score     int     `json:"score"`
}

// BenchmarkDisk performs a simple sequential write/read benchmark.
func BenchmarkDisk(ctx context.Context) DiskInfo {
	info := DiskInfo{}
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, "transcoder_disk_bench")
	defer os.Remove(testFile)

	// Write benchmark: 64MB of data
	const benchSize = 64 * 1024 * 1024
	data := make([]byte, benchSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Write test
	start := time.Now()
	if err := os.WriteFile(testFile, data, 0644); err != nil {
		info.Score = 30
		return info
	}
	writeElapsed := time.Since(start)
	if writeElapsed > 0 {
		info.WriteMBps = float64(benchSize) / writeElapsed.Seconds() / 1024 / 1024
	}

	// Read test
	start = time.Now()
	if _, err := os.ReadFile(testFile); err != nil {
		info.Score = 30
		return info
	}
	readElapsed := time.Since(start)
	if readElapsed > 0 {
		info.ReadMBps = float64(benchSize) / readElapsed.Seconds() / 1024 / 1024
	}

	info.Score = scoreDisk(info)
	return info
}

func scoreDisk(info DiskInfo) int {
	// Average read/write speed. SSD typically 500+, HDD ~100-150
	avg := (info.ReadMBps + info.WriteMBps) / 2

	switch {
	case avg >= 1000:
		return 95 // NVMe SSD
	case avg >= 500:
		return 80 // SATA SSD
	case avg >= 200:
		return 60 // Fast HDD or slow SSD
	case avg >= 100:
		return 40 // Typical HDD
	default:
		return 20 // Slow storage
	}
}
