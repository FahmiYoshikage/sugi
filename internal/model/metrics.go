package model

import "time"

// CPUTicks holds raw tick counters parsed from /proc/stat.
type CPUTicks struct {
	ID        string `json:"id"` // "cpu" (aggregate) or "cpu0", "cpu1", etc.
	User      uint64 `json:"user"`
	Nice      uint64 `json:"nice"`
	System    uint64 `json:"system"`
	Idle      uint64 `json:"idle"`
	IOWait    uint64 `json:"iowait"`
	IRQ       uint64 `json:"irq"`
	SoftIRQ   uint64 `json:"softirq"`
	Steal     uint64 `json:"steal"`
	Guest     uint64 `json:"guest"`
	GuestNice uint64 `json:"guest_nice"`
}

// Total returns the sum of all CPU ticks representing elapsed time.
func (c CPUTicks) Total() uint64 {
	return c.User + c.Nice + c.System + c.Idle + c.IOWait + c.IRQ + c.SoftIRQ + c.Steal
}

// IdleTotal returns idle + iowait ticks.
func (c CPUTicks) IdleTotal() uint64 {
	return c.Idle + c.IOWait
}

// NonIdleTotal returns total active work ticks.
func (c CPUTicks) NonIdleTotal() uint64 {
	return c.User + c.Nice + c.System + c.IRQ + c.SoftIRQ + c.Steal
}

// CoreUsage represents calculated CPU percentage metrics for a single core or aggregate.
type CoreUsage struct {
	ID          string  `json:"id"`           // e.g. "total", "cpu0", "cpu1"
	TotalUsage  float64 `json:"total_usage"`  // percentage 0.0 - 100.0
	UserUsage   float64 `json:"user_usage"`   // percentage 0.0 - 100.0
	SystemUsage float64 `json:"system_usage"` // percentage 0.0 - 100.0
	IOWaitUsage float64 `json:"iowait_usage"` // percentage 0.0 - 100.0
	StealUsage  float64 `json:"steal_usage"`  // percentage 0.0 - 100.0
}

// CPUStats represents the overall CPU metrics at a snapshot in time.
type CPUStats struct {
	Timestamp   time.Time   `json:"timestamp"`
	TotalUsage  float64     `json:"total_usage"` // Aggregate percentage
	UserUsage   float64     `json:"user_usage"`
	SystemUsage float64     `json:"system_usage"`
	IOWaitUsage float64     `json:"iowait_usage"`
	Cores       []CoreUsage `json:"cores"` // Per-core breakdown
}

// MemStats represents memory usage metrics parsed from /proc/meminfo.
type MemStats struct {
	Timestamp       time.Time `json:"timestamp"`
	TotalBytes      uint64    `json:"total_bytes"`
	FreeBytes       uint64    `json:"free_bytes"`
	AvailableBytes  uint64    `json:"available_bytes"`
	BuffersBytes    uint64    `json:"buffers_bytes"`
	CachedBytes     uint64    `json:"cached_bytes"`
	UsedBytes       uint64    `json:"used_bytes"`
	UsedPercent     float64   `json:"used_percent"`
	SwapTotalBytes  uint64    `json:"swap_total_bytes"`
	SwapFreeBytes   uint64    `json:"swap_free_bytes"`
	SwapUsedBytes   uint64    `json:"swap_used_bytes"`
	SwapUsedPercent float64   `json:"swap_used_percent"`
}

// RawDiskStats holds raw counter metrics parsed from /proc/diskstats for a single device.
type RawDiskStats struct {
	Major           int    `json:"major"`
	Minor           int    `json:"minor"`
	DeviceName      string `json:"device_name"`
	ReadsCompleted  uint64 `json:"reads_completed"`
	ReadsMerged     uint64 `json:"reads_merged"`
	SectorsRead     uint64 `json:"sectors_read"`
	ReadTimeMs      uint64 `json:"read_time_ms"`
	WritesCompleted uint64 `json:"writes_completed"`
	WritesMerged    uint64 `json:"writes_merged"`
	SectorsWritten  uint64 `json:"sectors_written"`
	WriteTimeMs     uint64 `json:"write_time_ms"`
	IOInProgress    uint64 `json:"io_in_progress"`
	IOTimeMs        uint64 `json:"io_time_ms"`
	WeightedIOTime  uint64 `json:"weighted_io_time"`
}

// DiskDeviceStats represents rate/throughput metrics for a single block device.
type DiskDeviceStats struct {
	Device           string  `json:"device"`
	ReadBytes        uint64  `json:"read_bytes"`
	WriteBytes       uint64  `json:"write_bytes"`
	ReadBytesPerSec  float64 `json:"read_bytes_per_sec"`
	WriteBytesPerSec float64 `json:"write_bytes_per_sec"`
	ReadIOPS         float64 `json:"read_iops"`
	WriteIOPS        float64 `json:"write_iops"`
	IOTimeMs         uint64  `json:"io_time_ms"`
}

// DiskStats represents aggregate and per-device disk I/O metrics.
type DiskStats struct {
	Timestamp            time.Time         `json:"timestamp"`
	TotalReadBytesPerSec float64           `json:"total_read_bytes_per_sec"`
	TotalWriteBytesPerSec float64          `json:"total_write_bytes_per_sec"`
	TotalReadIOPS        float64           `json:"total_read_iops"`
	TotalWriteIOPS       float64           `json:"total_write_iops"`
	Devices              []DiskDeviceStats `json:"devices"`
}

// RawNetStats holds raw interface statistics parsed from /proc/net/dev.
type RawNetStats struct {
	Interface string `json:"interface"`
	RxBytes   uint64 `json:"rx_bytes"`
	RxPackets uint64 `json:"rx_packets"`
	RxErrors  uint64 `json:"rx_errors"`
	RxDrops   uint64 `json:"rx_drops"`
	TxBytes   uint64 `json:"tx_bytes"`
	TxPackets uint64 `json:"tx_packets"`
	TxErrors  uint64 `json:"tx_errors"`
	TxDrops   uint64 `json:"tx_drops"`
}

// NetInterfaceStats represents calculated throughput metrics for a single network interface.
type NetInterfaceStats struct {
	Name            string  `json:"name"`
	RxBytes         uint64  `json:"rx_bytes"`
	TxBytes         uint64  `json:"tx_bytes"`
	RxBytesPerSec   float64 `json:"rx_bytes_per_sec"`
	TxBytesPerSec   float64 `json:"tx_bytes_per_sec"`
	RxPacketsPerSec float64 `json:"rx_packets_per_sec"`
	TxPacketsPerSec float64 `json:"tx_packets_per_sec"`
	RxErrors        uint64  `json:"rx_errors"`
	TxErrors        uint64  `json:"tx_errors"`
	RxDrops         uint64  `json:"rx_drops"`
	TxDrops         uint64  `json:"tx_drops"`
}

// NetStats represents system-wide and per-interface network metrics.
type NetStats struct {
	Timestamp         time.Time           `json:"timestamp"`
	TotalRxBytesPerSec float64            `json:"total_rx_bytes_per_sec"`
	TotalTxBytesPerSec float64            `json:"total_tx_bytes_per_sec"`
	Interfaces        []NetInterfaceStats `json:"interfaces"`
}

// SystemSnapshot combines all metrics at a specific timestamp.
type SystemSnapshot struct {
	Timestamp time.Time `json:"timestamp"`
	CPU       CPUStats  `json:"cpu"`
	Memory    MemStats  `json:"memory"`
	Disk      DiskStats `json:"disk"`
	Network   NetStats  `json:"network"`
}
