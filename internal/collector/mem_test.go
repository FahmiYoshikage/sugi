package collector

import (
	"bytes"
	"strings"
	"testing"
)

const mockMeminfoModern = `MemTotal:       16000000 kB
MemFree:         2000000 kB
MemAvailable:    8000000 kB
Buffers:          500000 kB
Cached:          4000000 kB
SwapCached:        40000 kB
Active:          5000000 kB
Inactive:        7000000 kB
SwapTotal:       2000000 kB
SwapFree:        1500000 kB
`

const mockMeminfoLegacy = `MemTotal:       16000000 kB
MemFree:         4000000 kB
Buffers:         2000000 kB
Cached:          2000000 kB
SwapTotal:             0 kB
SwapFree:              0 kB
`

func TestParseProcMeminfo_Modern(t *testing.T) {
	reader := strings.NewReader(mockMeminfoModern)
	stats, err := ParseProcMeminfo(reader)
	if err != nil {
		t.Fatalf("unexpected error parsing modern meminfo: %v", err)
	}

	const expectedTotal = 16000000 * 1024
	if stats.TotalBytes != expectedTotal {
		t.Errorf("expected TotalBytes=%d, got %d", expectedTotal, stats.TotalBytes)
	}

	const expectedAvailable = 8000000 * 1024
	if stats.AvailableBytes != expectedAvailable {
		t.Errorf("expected AvailableBytes=%d, got %d", expectedAvailable, stats.AvailableBytes)
	}

	const expectedUsed = 8000000 * 1024
	if stats.UsedBytes != expectedUsed {
		t.Errorf("expected UsedBytes=%d, got %d", expectedUsed, stats.UsedBytes)
	}

	if stats.UsedPercent != 50.0 {
		t.Errorf("expected UsedPercent=50.0, got %f", stats.UsedPercent)
	}

	const expectedSwapTotal = 2000000 * 1024
	const expectedSwapUsed = 500000 * 1024
	if stats.SwapTotalBytes != expectedSwapTotal {
		t.Errorf("expected SwapTotalBytes=%d, got %d", expectedSwapTotal, stats.SwapTotalBytes)
	}
	if stats.SwapUsedBytes != expectedSwapUsed {
		t.Errorf("expected SwapUsedBytes=%d, got %d", expectedSwapUsed, stats.SwapUsedBytes)
	}
	if stats.SwapUsedPercent != 25.0 {
		t.Errorf("expected SwapUsedPercent=25.0, got %f", stats.SwapUsedPercent)
	}
}

func TestParseProcMeminfo_Legacy(t *testing.T) {
	reader := strings.NewReader(mockMeminfoLegacy)
	stats, err := ParseProcMeminfo(reader)
	if err != nil {
		t.Fatalf("unexpected error parsing legacy meminfo: %v", err)
	}

	// Used = Total - Free - Buffers - Cached
	// 16M - 4M - 2M - 2M = 8M kB = 50%
	const expectedUsed = 8000000 * 1024
	if stats.UsedBytes != expectedUsed {
		t.Errorf("expected UsedBytes=%d, got %d", expectedUsed, stats.UsedBytes)
	}
	if stats.UsedPercent != 50.0 {
		t.Errorf("expected UsedPercent=50.0, got %f", stats.UsedPercent)
	}
	if stats.SwapTotalBytes != 0 || stats.SwapUsedBytes != 0 || stats.SwapUsedPercent != 0.0 {
		t.Errorf("expected zero swap on system with no swap, got %+v", stats)
	}
}

func TestParseProcMeminfo_Errors(t *testing.T) {
	// Empty reader
	_, err := ParseProcMeminfo(strings.NewReader(""))
	if err == nil {
		t.Error("expected error on empty reader, got nil")
	}

	// Missing MemTotal
	_, err = ParseProcMeminfo(strings.NewReader("MemFree: 1000 kB\nBuffers: 500 kB\n"))
	if err == nil {
		t.Error("expected error on missing MemTotal, got nil")
	}
}

func TestMemCollector_Collect(t *testing.T) {
	fs := &mockProcFS{content: mockMeminfoModern}
	collector := NewMemCollector(fs)

	stats, err := collector.Collect()
	if err != nil {
		t.Fatalf("unexpected collect error: %v", err)
	}

	if stats.UsedPercent != 50.0 {
		t.Errorf("expected UsedPercent=50.0, got %f", stats.UsedPercent)
	}
}

func BenchmarkParseProcMeminfo(b *testing.B) {
	data := []byte(mockMeminfoModern)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = ParseProcMeminfo(bytes.NewReader(data))
	}
}
