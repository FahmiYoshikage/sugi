package collector

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

const (
	// SectorSizeBytes is defined as 512 bytes across Linux kernel block layer stats.
	SectorSizeBytes = 512
)

// DiskCollector collects I/O statistics from /proc/diskstats.
type DiskCollector struct {
	fs        ProcFS
	mu        sync.Mutex
	lastStats map[string]model.RawDiskStats
	lastTime  time.Time
}

// NewDiskCollector creates a new DiskCollector instance.
func NewDiskCollector(fs ProcFS) *DiskCollector {
	if fs == nil {
		fs = NewDefaultProcFS()
	}
	return &DiskCollector{
		fs: fs,
	}
}

// shouldSkipDevice returns true for virtual or non-physical block devices (e.g. loop, ram).
func shouldSkipDevice(name string) bool {
	return strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram")
}

// ParseProcDiskstats parses disk statistics from an io.Reader (such as /proc/diskstats).
func ParseProcDiskstats(r io.Reader) (map[string]model.RawDiskStats, error) {
	scanner := bufio.NewScanner(r)
	result := make(map[string]model.RawDiskStats)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}

		devName := fields[2]
		if shouldSkipDevice(devName) {
			continue
		}

		major, _ := strconv.Atoi(fields[0])
		minor, _ := strconv.Atoi(fields[1])

		parseUint := func(idx int) uint64 {
			v, _ := strconv.ParseUint(fields[idx], 10, 64)
			return v
		}

		stats := model.RawDiskStats{
			Major:           major,
			Minor:           minor,
			DeviceName:      devName,
			ReadsCompleted:  parseUint(3),
			ReadsMerged:     parseUint(4),
			SectorsRead:     parseUint(5),
			ReadTimeMs:      parseUint(6),
			WritesCompleted: parseUint(7),
			WritesMerged:    parseUint(8),
			SectorsWritten:  parseUint(9),
			WriteTimeMs:     parseUint(10),
			IOInProgress:    parseUint(11),
			IOTimeMs:        parseUint(12),
			WeightedIOTime:  parseUint(13),
		}

		result[devName] = stats
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan /proc/diskstats: %w", err)
	}

	return result, nil
}

// CalculateDiskStats computes rate metrics between two disk stat snapshots over elapsed duration.
func CalculateDiskStats(prev, curr map[string]model.RawDiskStats, duration time.Duration) model.DiskStats {
	now := time.Now().UTC()
	seconds := duration.Seconds()
	if seconds <= 0 {
		seconds = 1.0
	}

	stats := model.DiskStats{
		Timestamp: now,
		Devices:   make([]model.DiskDeviceStats, 0, len(curr)),
	}

	devNames := make([]string, 0, len(curr))
	for name := range curr {
		devNames = append(devNames, name)
	}
	sort.Strings(devNames)

	for _, name := range devNames {
		currDev := curr[name]
		readBytes := currDev.SectorsRead * SectorSizeBytes
		writeBytes := currDev.SectorsWritten * SectorSizeBytes

		devStat := model.DiskDeviceStats{
			Device:     name,
			ReadBytes:  readBytes,
			WriteBytes: writeBytes,
			IOTimeMs:   currDev.IOTimeMs,
		}

		if prevDev, ok := prev[name]; ok {
			if currDev.SectorsRead >= prevDev.SectorsRead {
				deltaReadSectors := currDev.SectorsRead - prevDev.SectorsRead
				devStat.ReadBytesPerSec = roundFloat(float64(deltaReadSectors*SectorSizeBytes)/seconds, 2)
			}
			if currDev.SectorsWritten >= prevDev.SectorsWritten {
				deltaWriteSectors := currDev.SectorsWritten - prevDev.SectorsWritten
				devStat.WriteBytesPerSec = roundFloat(float64(deltaWriteSectors*SectorSizeBytes)/seconds, 2)
			}
			if currDev.ReadsCompleted >= prevDev.ReadsCompleted {
				deltaReads := currDev.ReadsCompleted - prevDev.ReadsCompleted
				devStat.ReadIOPS = roundFloat(float64(deltaReads)/seconds, 2)
			}
			if currDev.WritesCompleted >= prevDev.WritesCompleted {
				deltaWrites := currDev.WritesCompleted - prevDev.WritesCompleted
				devStat.WriteIOPS = roundFloat(float64(deltaWrites)/seconds, 2)
			}
		}

		// Only aggregate primary/root devices (e.g., avoid double counting both whole disk and partition)
		// Usually whole disks don't end with a partition number e.g. nvme0n1 (vs nvme0n1p1) or sda (vs sda1)
		if isPrimaryDisk(name) {
			stats.TotalReadBytesPerSec += devStat.ReadBytesPerSec
			stats.TotalWriteBytesPerSec += devStat.WriteBytesPerSec
			stats.TotalReadIOPS += devStat.ReadIOPS
			stats.TotalWriteIOPS += devStat.WriteIOPS
		}

		stats.Devices = append(stats.Devices, devStat)
	}

	stats.TotalReadBytesPerSec = roundFloat(stats.TotalReadBytesPerSec, 2)
	stats.TotalWriteBytesPerSec = roundFloat(stats.TotalWriteBytesPerSec, 2)
	stats.TotalReadIOPS = roundFloat(stats.TotalReadIOPS, 2)
	stats.TotalWriteIOPS = roundFloat(stats.TotalWriteIOPS, 2)

	return stats
}

// isPrimaryDisk heuristically determines if a disk is a whole block device rather than a partition.
func isPrimaryDisk(name string) bool {
	// nvmeXnY vs nvmeXnYpZ
	if strings.HasPrefix(name, "nvme") {
		return !strings.Contains(name, "p")
	}
	// mmcblkX vs mmcblkXpY
	if strings.HasPrefix(name, "mmcblk") {
		return !strings.Contains(name, "p")
	}
	// sdX, vdX, hdX, xvdX
	for _, prefix := range []string{"sd", "vd", "hd", "xvd"} {
		if strings.HasPrefix(name, prefix) {
			suffix := strings.TrimPrefix(name, prefix)
			// Whole disk has letters like "a", "b", partitions have "a1", "b2"
			if len(suffix) == 1 && suffix[0] >= 'a' && suffix[0] <= 'z' {
				return true
			}
			return false
		}
	}
	return false
}

// Collect reads /proc/diskstats and calculates rates against previous snapshot.
func (d *DiskCollector) Collect() (model.DiskStats, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	file, err := d.fs.Open("diskstats")
	if err != nil {
		return model.DiskStats{}, fmt.Errorf("open diskstats: %w", err)
	}
	defer file.Close()

	currStats, err := ParseProcDiskstats(file)
	if err != nil {
		return model.DiskStats{}, err
	}

	now := time.Now().UTC()

	if d.lastStats == nil {
		d.lastStats = currStats
		d.lastTime = now

		devices := make([]model.DiskDeviceStats, 0, len(currStats))
		for name, dev := range currStats {
			devices = append(devices, model.DiskDeviceStats{
				Device:     name,
				ReadBytes:  dev.SectorsRead * SectorSizeBytes,
				WriteBytes: dev.SectorsWritten * SectorSizeBytes,
				IOTimeMs:   dev.IOTimeMs,
			})
		}
		sort.Slice(devices, func(i, j int) bool {
			return devices[i].Device < devices[j].Device
		})

		return model.DiskStats{
			Timestamp: now,
			Devices:   devices,
		}, nil
	}

	duration := now.Sub(d.lastTime)
	stats := CalculateDiskStats(d.lastStats, currStats, duration)
	stats.Timestamp = now

	d.lastStats = currStats
	d.lastTime = now

	return stats, nil
}
