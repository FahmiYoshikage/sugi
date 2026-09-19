package collector

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

// MemCollector collects memory metrics from /proc/meminfo.
type MemCollector struct {
	fs ProcFS
}

// NewMemCollector creates a new MemCollector instance.
func NewMemCollector(fs ProcFS) *MemCollector {
	if fs == nil {
		fs = NewDefaultProcFS()
	}
	return &MemCollector{
		fs: fs,
	}
}

// ParseProcMeminfo reads and parses an io.Reader containing /proc/meminfo content.
func ParseProcMeminfo(r io.Reader) (model.MemStats, error) {
	scanner := bufio.NewScanner(r)
	stats := model.MemStats{
		Timestamp: time.Now().UTC(),
	}

	rawKB := make(map[string]uint64)

	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.IndexByte(line, ':')
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		valPart := strings.TrimSpace(line[idx+1:])

		// valPart is typically "<number> kB" or just "<number>"
		fields := strings.Fields(valPart)
		if len(fields) == 0 {
			continue
		}

		val, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}

		rawKB[key] = val
	}

	if err := scanner.Err(); err != nil {
		return stats, fmt.Errorf("scan /proc/meminfo: %w", err)
	}

	if len(rawKB) == 0 {
		return stats, fmt.Errorf("no memory metrics found in /proc/meminfo")
	}

	const kbToBytes = 1024

	stats.TotalBytes = rawKB["MemTotal"] * kbToBytes
	stats.FreeBytes = rawKB["MemFree"] * kbToBytes
	stats.BuffersBytes = rawKB["Buffers"] * kbToBytes
	stats.CachedBytes = rawKB["Cached"] * kbToBytes

	if stats.TotalBytes == 0 {
		return stats, fmt.Errorf("invalid /proc/meminfo: MemTotal is 0 or missing")
	}

	// MemAvailable is available since Linux 3.14
	if availKB, ok := rawKB["MemAvailable"]; ok && availKB > 0 {
		stats.AvailableBytes = availKB * kbToBytes
		if stats.TotalBytes >= stats.AvailableBytes {
			stats.UsedBytes = stats.TotalBytes - stats.AvailableBytes
		}
	} else {
		// Fallback for legacy Linux kernels: Used = Total - Free - Buffers - Cached
		nonUsed := stats.FreeBytes + stats.BuffersBytes + stats.CachedBytes
		if stats.TotalBytes >= nonUsed {
			stats.UsedBytes = stats.TotalBytes - nonUsed
			stats.AvailableBytes = nonUsed
		} else {
			stats.UsedBytes = 0
			stats.AvailableBytes = stats.TotalBytes
		}
	}

	stats.UsedPercent = clampPercentage(roundFloat((float64(stats.UsedBytes)/float64(stats.TotalBytes))*100.0, 2))

	// Swap metrics
	if swapTotalKB, ok := rawKB["SwapTotal"]; ok && swapTotalKB > 0 {
		stats.SwapTotalBytes = swapTotalKB * kbToBytes
		stats.SwapFreeBytes = rawKB["SwapFree"] * kbToBytes
		if stats.SwapTotalBytes >= stats.SwapFreeBytes {
			stats.SwapUsedBytes = stats.SwapTotalBytes - stats.SwapFreeBytes
			stats.SwapUsedPercent = clampPercentage(roundFloat((float64(stats.SwapUsedBytes)/float64(stats.SwapTotalBytes))*100.0, 2))
		}
	}

	return stats, nil
}

// Collect reads /proc/meminfo and returns current MemStats.
func (m *MemCollector) Collect() (model.MemStats, error) {
	file, err := m.fs.Open("meminfo")
	if err != nil {
		return model.MemStats{}, fmt.Errorf("open meminfo: %w", err)
	}
	defer file.Close()

	return ParseProcMeminfo(file)
}
