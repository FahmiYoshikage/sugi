package main

import (
	"fmt"
	"log"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/collector"
)

func main() {
	fmt.Println("=== Sugi Observability Engine (Tahap 1 - Core Collectors) ===")
	fs := collector.NewDefaultProcFS()

	cpuCol := collector.NewCPUCollector(fs)
	memCol := collector.NewMemCollector(fs)

	// Baseline snapshot
	_, err := cpuCol.Collect()
	if err != nil {
		log.Fatalf("Error collecting baseline CPU: %v", err)
	}

	fmt.Println("Collecting initial system metrics (waiting 1 second for CPU delta)...")
	time.Sleep(1 * time.Second)

	cpuStats, err := cpuCol.Collect()
	if err != nil {
		log.Fatalf("Error collecting CPU stats: %v", err)
	}

	memStats, err := memCol.Collect()
	if err != nil {
		log.Fatalf("Error collecting Memory stats: %v", err)
	}

	fmt.Printf("\n[CPU Metrics]\n")
	fmt.Printf("Total Usage : %.2f%%\n", cpuStats.TotalUsage)
	fmt.Printf("User Usage  : %.2f%%\n", cpuStats.UserUsage)
	fmt.Printf("System Usage: %.2f%%\n", cpuStats.SystemUsage)
	fmt.Printf("IOWait Usage: %.2f%%\n", cpuStats.IOWaitUsage)
	fmt.Printf("Active Cores: %d\n", len(cpuStats.Cores))
	for _, core := range cpuStats.Cores {
		fmt.Printf("  - Core %-5s: %.2f%%\n", core.ID, core.TotalUsage)
	}

	fmt.Printf("\n[Memory Metrics]\n")
	fmt.Printf("Total RAM   : %d MB (%.2f GB)\n", memStats.TotalBytes/(1024*1024), float64(memStats.TotalBytes)/(1024*1024*1024))
	fmt.Printf("Used RAM    : %d MB (%.2f%%)\n", memStats.UsedBytes/(1024*1024), memStats.UsedPercent)
	fmt.Printf("Free RAM    : %d MB\n", memStats.FreeBytes/(1024*1024))
	fmt.Printf("Available   : %d MB\n", memStats.AvailableBytes/(1024*1024))
	fmt.Printf("Cached      : %d MB\n", memStats.CachedBytes/(1024*1024))
	if memStats.SwapTotalBytes > 0 {
		fmt.Printf("Swap Total  : %d MB\n", memStats.SwapTotalBytes/(1024*1024))
		fmt.Printf("Swap Used   : %d MB (%.2f%%)\n", memStats.SwapUsedBytes/(1024*1024), memStats.SwapUsedPercent)
	}

	fmt.Println("\nTahap 1 verification successful.")
}
