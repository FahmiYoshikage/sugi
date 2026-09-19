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
	Timestamp  time.Time   `json:"timestamp"`
	TotalUsage float64     `json:"total_usage"` // Aggregate percentage
	UserUsage  float64     `json:"user_usage"`
	SystemUsage float64    `json:"system_usage"`
	IOWaitUsage float64    `json:"iowait_usage"`
	Cores      []CoreUsage `json:"cores"` // Per-core breakdown
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

// SystemSnapshot combines all metrics at a specific timestamp.
type SystemSnapshot struct {
	Timestamp time.Time `json:"timestamp"`
	CPU       CPUStats  `json:"cpu"`
	Memory    MemStats  `json:"memory"`
}
