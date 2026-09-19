package main

import (
	"fmt"
	"log"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/collector"
	"github.com/FahmiYoshikage/sugi/internal/model"
	"github.com/FahmiYoshikage/sugi/internal/storage"
)

func main() {
	fmt.Println("=== Sugi Observability Engine (Tahap 2 - Collectors & Time-Series Ring Buffer) ===")
	fs := collector.NewDefaultProcFS()

	cpuCol := collector.NewCPUCollector(fs)
	memCol := collector.NewMemCollector(fs)
	diskCol := collector.NewDiskCollector(fs)
	netCol := collector.NewNetCollector(fs)

	// Ring buffer for 1 hour of time-series (3600 points)
	ringBuffer := storage.NewRingBuffer(storage.DefaultRingBufferCapacity)

	// Initial baseline snapshot
	_, _ = cpuCol.Collect()
	_, _ = diskCol.Collect()
	_, _ = netCol.Collect()

	fmt.Println("Sampling system metrics (waiting 1 second for rate deltas)...")
	time.Sleep(1 * time.Second)

	cpuStats, err := cpuCol.Collect()
	if err != nil {
		log.Fatalf("Error collecting CPU: %v", err)
	}

	memStats, err := memCol.Collect()
	if err != nil {
		log.Fatalf("Error collecting Memory: %v", err)
	}

	diskStats, err := diskCol.Collect()
	if err != nil {
		log.Fatalf("Error collecting Disk: %v", err)
	}

	netStats, err := netCol.Collect()
	if err != nil {
		log.Fatalf("Error collecting Network: %v", err)
	}

	snapshot := model.SystemSnapshot{
		Timestamp: time.Now().UTC(),
		CPU:       cpuStats,
		Memory:    memStats,
		Disk:      diskStats,
		Network:   netStats,
	}

	// Push snapshot into RingBuffer
	ringBuffer.Push(snapshot)

	latest, ok := ringBuffer.GetLatest()
	if !ok {
		log.Fatalf("Failed to retrieve snapshot from RingBuffer")
	}

	fmt.Printf("\n[CPU Metrics]\n")
	fmt.Printf("Total Usage : %.2f%%\n", latest.CPU.TotalUsage)
	fmt.Printf("Active Cores: %d\n", len(latest.CPU.Cores))

	fmt.Printf("\n[Memory Metrics]\n")
	fmt.Printf("Total RAM   : %d MB (%.2f GB)\n", latest.Memory.TotalBytes/(1024*1024), float64(latest.Memory.TotalBytes)/(1024*1024*1024))
	fmt.Printf("Used RAM    : %d MB (%.2f%%)\n", latest.Memory.UsedBytes/(1024*1024), latest.Memory.UsedPercent)

	fmt.Printf("\n[Disk I/O Metrics]\n")
	fmt.Printf("Read Throughput : %.2f KB/s (%.1f IOPS)\n", latest.Disk.TotalReadBytesPerSec/1024.0, latest.Disk.TotalReadIOPS)
	fmt.Printf("Write Throughput: %.2f KB/s (%.1f IOPS)\n", latest.Disk.TotalWriteBytesPerSec/1024.0, latest.Disk.TotalWriteIOPS)
	fmt.Printf("Monitored Disks : %d devices\n", len(latest.Disk.Devices))
	for _, dev := range latest.Disk.Devices {
		if dev.ReadBytesPerSec > 0 || dev.WriteBytesPerSec > 0 {
			fmt.Printf("  - Device %-10s: Read %.2f KB/s (%.1f IOPS) | Write %.2f KB/s (%.1f IOPS)\n",
				dev.Device, dev.ReadBytesPerSec/1024.0, dev.ReadIOPS, dev.WriteBytesPerSec/1024.0, dev.WriteIOPS)
		}
	}

	fmt.Printf("\n[Network I/O Metrics]\n")
	fmt.Printf("Total Ingress (Rx) : %.2f KB/s\n", latest.Network.TotalRxBytesPerSec/1024.0)
	fmt.Printf("Total Egress (Tx)  : %.2f KB/s\n", latest.Network.TotalTxBytesPerSec/1024.0)
	fmt.Printf("Interfaces Monitored: %d\n", len(latest.Network.Interfaces))
	for _, iface := range latest.Network.Interfaces {
		if iface.RxBytesPerSec > 0 || iface.TxBytesPerSec > 0 {
			fmt.Printf("  - Interface %-10s: Rx %.2f KB/s (%.1f pkt/s) | Tx %.2f KB/s (%.1f pkt/s)\n",
				iface.Name, iface.RxBytesPerSec/1024.0, iface.RxPacketsPerSec, iface.TxBytesPerSec/1024.0, iface.TxPacketsPerSec)
		}
	}

	fmt.Printf("\n[RingBuffer Status]\n")
	fmt.Printf("Stored Points: %d / %d (Capacity: 1 Hour @ 1s interval)\n", ringBuffer.Size(), ringBuffer.Capacity())
	fmt.Println("\nTahap 2 verification successful.")
}
