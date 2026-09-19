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

// NetCollector collects network interface statistics from /proc/net/dev.
type NetCollector struct {
	fs        ProcFS
	mu        sync.Mutex
	lastStats map[string]model.RawNetStats
	lastTime  time.Time
}

// NewNetCollector creates a new NetCollector instance.
func NewNetCollector(fs ProcFS) *NetCollector {
	if fs == nil {
		fs = NewDefaultProcFS()
	}
	return &NetCollector{
		fs: fs,
	}
}

// ParseProcNetDev parses interface metrics from an io.Reader (such as /proc/net/dev).
func ParseProcNetDev(r io.Reader) (map[string]model.RawNetStats, error) {
	scanner := bufio.NewScanner(r)
	result := make(map[string]model.RawNetStats)

	for scanner.Scan() {
		line := scanner.Text()
		colonIdx := strings.IndexByte(line, ':')
		if colonIdx == -1 {
			continue // Skip header lines
		}

		iface := strings.TrimSpace(line[:colonIdx])
		dataPart := strings.TrimSpace(line[colonIdx+1:])
		fields := strings.Fields(dataPart)
		if len(fields) < 16 {
			continue
		}

		parseUint := func(idx int) uint64 {
			v, _ := strconv.ParseUint(fields[idx], 10, 64)
			return v
		}

		stats := model.RawNetStats{
			Interface: iface,
			RxBytes:   parseUint(0),
			RxPackets: parseUint(1),
			RxErrors:  parseUint(2),
			RxDrops:   parseUint(3),
			TxBytes:   parseUint(8),
			TxPackets: parseUint(9),
			TxErrors:  parseUint(10),
			TxDrops:   parseUint(11),
		}

		result[iface] = stats
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan /proc/net/dev: %w", err)
	}

	return result, nil
}

// CalculateNetStats computes network throughput rates between two snapshots.
func CalculateNetStats(prev, curr map[string]model.RawNetStats, duration time.Duration) model.NetStats {
	now := time.Now().UTC()
	seconds := duration.Seconds()
	if seconds <= 0 {
		seconds = 1.0
	}

	stats := model.NetStats{
		Timestamp:  now,
		Interfaces: make([]model.NetInterfaceStats, 0, len(curr)),
	}

	ifaceNames := make([]string, 0, len(curr))
	for name := range curr {
		ifaceNames = append(ifaceNames, name)
	}
	sort.Strings(ifaceNames)

	for _, name := range ifaceNames {
		currIface := curr[name]
		ifaceStat := model.NetInterfaceStats{
			Name:     name,
			RxBytes:  currIface.RxBytes,
			TxBytes:  currIface.TxBytes,
			RxErrors: currIface.RxErrors,
			TxErrors: currIface.TxErrors,
			RxDrops:  currIface.RxDrops,
			TxDrops:  currIface.TxDrops,
		}

		if prevIface, ok := prev[name]; ok {
			if currIface.RxBytes >= prevIface.RxBytes {
				deltaRx := currIface.RxBytes - prevIface.RxBytes
				ifaceStat.RxBytesPerSec = roundFloat(float64(deltaRx)/seconds, 2)
			}
			if currIface.TxBytes >= prevIface.TxBytes {
				deltaTx := currIface.TxBytes - prevIface.TxBytes
				ifaceStat.TxBytesPerSec = roundFloat(float64(deltaTx)/seconds, 2)
			}
			if currIface.RxPackets >= prevIface.RxPackets {
				deltaRxPkt := currIface.RxPackets - prevIface.RxPackets
				ifaceStat.RxPacketsPerSec = roundFloat(float64(deltaRxPkt)/seconds, 2)
			}
			if currIface.TxPackets >= prevIface.TxPackets {
				deltaTxPkt := currIface.TxPackets - prevIface.TxPackets
				ifaceStat.TxPacketsPerSec = roundFloat(float64(deltaTxPkt)/seconds, 2)
			}
		}

		// Aggregate system-wide throughput (excluding loopback 'lo' interface)
		if name != "lo" {
			stats.TotalRxBytesPerSec += ifaceStat.RxBytesPerSec
			stats.TotalTxBytesPerSec += ifaceStat.TxBytesPerSec
		}

		stats.Interfaces = append(stats.Interfaces, ifaceStat)
	}

	stats.TotalRxBytesPerSec = roundFloat(stats.TotalRxBytesPerSec, 2)
	stats.TotalTxBytesPerSec = roundFloat(stats.TotalTxBytesPerSec, 2)

	return stats
}

// Collect reads /proc/net/dev and calculates throughput rates against previous snapshot.
func (n *NetCollector) Collect() (model.NetStats, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	file, err := n.fs.Open("net/dev")
	if err != nil {
		return model.NetStats{}, fmt.Errorf("open net/dev: %w", err)
	}
	defer file.Close()

	currStats, err := ParseProcNetDev(file)
	if err != nil {
		return model.NetStats{}, err
	}

	now := time.Now().UTC()

	if n.lastStats == nil {
		n.lastStats = currStats
		n.lastTime = now

		interfaces := make([]model.NetInterfaceStats, 0, len(currStats))
		for name, iface := range currStats {
			interfaces = append(interfaces, model.NetInterfaceStats{
				Name:     name,
				RxBytes:  iface.RxBytes,
				TxBytes:  iface.TxBytes,
				RxErrors: iface.RxErrors,
				TxErrors: iface.TxErrors,
				RxDrops:  iface.RxDrops,
				TxDrops:  iface.TxDrops,
			})
		}
		sort.Slice(interfaces, func(i, j int) bool {
			return interfaces[i].Name < interfaces[j].Name
		})

		return model.NetStats{
			Timestamp:  now,
			Interfaces: interfaces,
		}, nil
	}

	duration := now.Sub(n.lastTime)
	stats := CalculateNetStats(n.lastStats, currStats, duration)
	stats.Timestamp = now

	n.lastStats = currStats
	n.lastTime = now

	return stats, nil
}
