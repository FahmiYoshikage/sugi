package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/collector"
	"github.com/FahmiYoshikage/sugi/internal/model"
	"github.com/FahmiYoshikage/sugi/internal/storage"
)

func main() {
	fmt.Println("=== Sugi Observability Engine (Tahap 3 - Persistent Storage & WAL SQLite) ===")
	fs := collector.NewDefaultProcFS()

	cpuCol := collector.NewCPUCollector(fs)
	memCol := collector.NewMemCollector(fs)
	diskCol := collector.NewDiskCollector(fs)
	netCol := collector.NewNetCollector(fs)

	// In-memory ring buffer for 1 hour of time-series metrics
	ringBuffer := storage.NewRingBuffer(storage.DefaultRingBufferCapacity)

	// Persistent embedded SQLite storage (Pure Go, WAL mode)
	sqliteStorage, err := storage.NewSQLiteStorage("sugi.db")
	if err != nil {
		log.Fatalf("Failed to initialize SQLite storage: %v", err)
	}
	defer sqliteStorage.Close()

	// Async batch log writer with bounded channel
	writerCfg := storage.DefaultAsyncLogWriterConfig()
	writerCfg.FlushInterval = 200 * time.Millisecond
	logWriter := storage.NewAsyncLogWriter(sqliteStorage, writerCfg)

	// Baseline snapshot
	_, _ = cpuCol.Collect()
	_, _ = diskCol.Collect()
	_, _ = netCol.Collect()

	fmt.Println("Sampling system metrics (waiting 1 second for rate deltas)...")
	time.Sleep(1 * time.Second)

	cpuStats, _ := cpuCol.Collect()
	memStats, _ := memCol.Collect()
	diskStats, _ := diskCol.Collect()
	netStats, _ := netCol.Collect()

	snapshot := model.SystemSnapshot{
		Timestamp: time.Now().UTC(),
		CPU:       cpuStats,
		Memory:    memStats,
		Disk:      diskStats,
		Network:   netStats,
	}
	ringBuffer.Push(snapshot)

	// Enqueue demonstration structured logs
	now := time.Now().UTC()
	logWriter.Enqueue(model.LogEntry{
		Timestamp:  now,
		Level:      "INFO",
		Service:    "sugi-core",
		Message:    "Sugi engine started successfully in single-binary mode",
		Attributes: map[string]string{"version": "v0.1.0", "storage": "sqlite-wal"},
	})
	logWriter.Enqueue(model.LogEntry{
		Timestamp:  now.Add(50 * time.Millisecond),
		Level:      "INFO",
		Service:    "collector",
		Message:    fmt.Sprintf("Procfs sampled: CPU %.2f%%, RAM %.2f%%, Active Cores: %d", cpuStats.TotalUsage, memStats.UsedPercent, len(cpuStats.Cores)),
		Attributes: map[string]string{"type": "procfs"},
	})
	logWriter.Enqueue(model.LogEntry{
		Timestamp:  now.Add(100 * time.Millisecond),
		Level:      "WARN",
		Service:    "net-monitor",
		Message:    fmt.Sprintf("Network interface monitored: %d active interfaces", len(netStats.Interfaces)),
		Attributes: map[string]string{"rx_kb": fmt.Sprintf("%.2f", netStats.TotalRxBytesPerSec/1024.0)},
	})

	// Flush async writer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = logWriter.Close(ctx)

	// Query stored logs from SQLite
	totalLogs, err := sqliteStorage.CountLogs()
	if err != nil {
		log.Fatalf("Count logs error: %v", err)
	}

	recentLogs, err := sqliteStorage.QueryLogs(model.LogFilter{Limit: 3})
	if err != nil {
		log.Fatalf("Query logs error: %v", err)
	}

	latest, _ := ringBuffer.GetLatest()

	fmt.Printf("\n[Metrics Summary]\n")
	fmt.Printf("CPU Usage    : %.2f%% (%d cores)\n", latest.CPU.TotalUsage, len(latest.CPU.Cores))
	fmt.Printf("RAM Used     : %d MB (%.2f%% of %d MB)\n", latest.Memory.UsedBytes/(1024*1024), latest.Memory.UsedPercent, latest.Memory.TotalBytes/(1024*1024))
	fmt.Printf("Disk I/O     : Read %.2f KB/s | Write %.2f KB/s\n", latest.Disk.TotalReadBytesPerSec/1024.0, latest.Disk.TotalWriteBytesPerSec/1024.0)
	fmt.Printf("Network I/O  : Ingress %.2f KB/s | Egress %.2f KB/s\n", latest.Network.TotalRxBytesPerSec/1024.0, latest.Network.TotalTxBytesPerSec/1024.0)
	fmt.Printf("RingBuffer   : %d / %d points stored\n", ringBuffer.Size(), ringBuffer.Capacity())

	fmt.Printf("\n[Persistent Storage: SQLite (WAL Mode)]\n")
	fmt.Printf("Total Logs Persisted: %d records\n", totalLogs)
	fmt.Println("Recent Logs:")
	for _, l := range recentLogs {
		fmt.Printf("  [%s] %-5s (%-12s): %s | Attrs: %v\n",
			l.Timestamp.Format("15:04:05.000"), l.Level, l.Service, l.Message, l.Attributes)
	}

	fmt.Println("\nTahap 3 verification successful.")
}
