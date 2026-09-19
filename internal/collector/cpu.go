package collector

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

// CPUCollector collects CPU metrics from /proc/stat.
type CPUCollector struct {
	fs        ProcFS
	mu        sync.Mutex
	lastTicks map[string]model.CPUTicks
	lastTime  time.Time
}

// NewCPUCollector creates a new CPUCollector instance.
func NewCPUCollector(fs ProcFS) *CPUCollector {
	if fs == nil {
		fs = NewDefaultProcFS()
	}
	return &CPUCollector{
		fs: fs,
	}
}

// ParseProcStat reads and parses CPU tick lines from an io.Reader (such as /proc/stat).
// Returns a map keyed by core ID ("cpu" for aggregate, "cpu0", "cpu1", etc.).
func ParseProcStat(r io.Reader) (map[string]model.CPUTicks, error) {
	result := make(map[string]model.CPUTicks)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		// Only process CPU lines
		if !strings.HasPrefix(line, "cpu") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		id := fields[0]
		// Validate that the label is either "cpu" or starts with "cpu" followed by numbers
		if id != "cpu" && !isCoreLabel(id) {
			continue
		}

		ticks := model.CPUTicks{ID: id}
		parseField := func(idx int) uint64 {
			if idx < len(fields) {
				v, _ := strconv.ParseUint(fields[idx], 10, 64)
				return v
			}
			return 0
		}

		ticks.User = parseField(1)
		ticks.Nice = parseField(2)
		ticks.System = parseField(3)
		ticks.Idle = parseField(4)
		ticks.IOWait = parseField(5)
		ticks.IRQ = parseField(6)
		ticks.SoftIRQ = parseField(7)
		ticks.Steal = parseField(8)
		ticks.Guest = parseField(9)
		ticks.GuestNice = parseField(10)

		result[id] = ticks
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan /proc/stat: %w", err)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no cpu metrics found in /proc/stat")
	}

	return result, nil
}

// isCoreLabel checks if the string is of the form "cpu" + digits.
func isCoreLabel(s string) bool {
	if !strings.HasPrefix(s, "cpu") || len(s) <= 3 {
		return false
	}
	_, err := strconv.Atoi(s[3:])
	return err == nil
}

// coreIndex extracts the numeric core index from "cpu0", "cpu1", etc.
func coreIndex(s string) int {
	if s == "cpu" {
		return -1
	}
	idx, err := strconv.Atoi(s[3:])
	if err != nil {
		return 999999
	}
	return idx
}

// calculateDelta calculates usage percentages between two snapshots for a single CPU core/aggregate.
func calculateDelta(prev, curr model.CPUTicks) model.CoreUsage {
	usage := model.CoreUsage{ID: curr.ID}

	prevIdle := prev.IdleTotal()
	currIdle := curr.IdleTotal()

	prevTotal := prev.Total()
	currTotal := curr.Total()

	if currTotal <= prevTotal {
		return usage
	}

	deltaTotal := float64(currTotal - prevTotal)
	deltaIdle := float64(currIdle - prevIdle)

	totalUsage := ((deltaTotal - deltaIdle) / deltaTotal) * 100.0
	usage.TotalUsage = clampPercentage(roundFloat(totalUsage, 2))

	if curr.User >= prev.User {
		usage.UserUsage = clampPercentage(roundFloat((float64(curr.User-prev.User)/deltaTotal)*100.0, 2))
	}
	if curr.System >= prev.System {
		usage.SystemUsage = clampPercentage(roundFloat((float64(curr.System-prev.System)/deltaTotal)*100.0, 2))
	}
	if curr.IOWait >= prev.IOWait {
		usage.IOWaitUsage = clampPercentage(roundFloat((float64(curr.IOWait-prev.IOWait)/deltaTotal)*100.0, 2))
	}
	if curr.Steal >= prev.Steal {
		usage.StealUsage = clampPercentage(roundFloat((float64(curr.Steal-prev.Steal)/deltaTotal)*100.0, 2))
	}

	return usage
}

// CalculateCPUUsage computes CPU usage between two tick snapshots.
func CalculateCPUUsage(prev, curr map[string]model.CPUTicks) model.CPUStats {
	now := time.Now().UTC()
	stats := model.CPUStats{
		Timestamp: now,
		Cores:     make([]model.CoreUsage, 0),
	}

	// Calculate aggregate if present
	if currAgg, ok := curr["cpu"]; ok {
		if prevAgg, ok := prev["cpu"]; ok {
			aggUsage := calculateDelta(prevAgg, currAgg)
			stats.TotalUsage = aggUsage.TotalUsage
			stats.UserUsage = aggUsage.UserUsage
			stats.SystemUsage = aggUsage.SystemUsage
			stats.IOWaitUsage = aggUsage.IOWaitUsage
		}
	}

	// Collect and sort individual core IDs
	coreIDs := make([]string, 0, len(curr))
	for id := range curr {
		if id != "cpu" {
			coreIDs = append(coreIDs, id)
		}
	}

	sort.Slice(coreIDs, func(i, j int) bool {
		return coreIndex(coreIDs[i]) < coreIndex(coreIDs[j])
	})

	for _, id := range coreIDs {
		currCore := curr[id]
		if prevCore, ok := prev[id]; ok {
			coreUsage := calculateDelta(prevCore, currCore)
			stats.Cores = append(stats.Cores, coreUsage)
		}
	}

	return stats
}

// Collect reads /proc/stat, calculates the delta against the previous tick snapshot, and returns CPUStats.
func (c *CPUCollector) Collect() (model.CPUStats, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	file, err := c.fs.Open("stat")
	if err != nil {
		return model.CPUStats{}, fmt.Errorf("open stat: %w", err)
	}
	defer file.Close()

	currTicks, err := ParseProcStat(file)
	if err != nil {
		return model.CPUStats{}, err
	}

	now := time.Now().UTC()

	// Initial snapshot: cannot calculate delta yet
	if c.lastTicks == nil {
		c.lastTicks = currTicks
		c.lastTime = now

		cores := make([]model.CoreUsage, 0)
		for id := range currTicks {
			if id != "cpu" {
				cores = append(cores, model.CoreUsage{ID: id})
			}
		}
		sort.Slice(cores, func(i, j int) bool {
			return coreIndex(cores[i].ID) < coreIndex(cores[j].ID)
		})

		return model.CPUStats{
			Timestamp: now,
			Cores:     cores,
		}, nil
	}

	stats := CalculateCPUUsage(c.lastTicks, currTicks)
	stats.Timestamp = now

	// Update last snapshot
	c.lastTicks = currTicks
	c.lastTime = now

	return stats, nil
}

func roundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

func clampPercentage(v float64) float64 {
	if v < 0.0 {
		return 0.0
	}
	if v > 100.0 {
		return 100.0
	}
	return v
}
