package collector

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

const mockProcStatT1 = `cpu  10000 500 3000 80000 1000 100 200 0 0 0
cpu0 5000 250 1500 40000 500 50 100 0 0 0
cpu1 5000 250 1500 40000 500 50 100 0 0 0
intr 12345678 0 0
ctxt 87654321
btime 1700000000
processes 12345
procs_running 2
procs_blocked 0
`

const mockProcStatT2 = `cpu  10080 500 3020 80080 1020 100 200 0 0 0
cpu0 5050 250 1510 40030 510 50 100 0 0 0
cpu1 5030 250 1510 40050 510 50 100 0 0 0
intr 12345999 0 0
ctxt 87655555
btime 1700000000
processes 12350
procs_running 1
procs_blocked 0
`

func TestParseProcStat(t *testing.T) {
	reader := strings.NewReader(mockProcStatT1)
	ticks, err := ParseProcStat(reader)
	if err != nil {
		t.Fatalf("unexpected error parsing /proc/stat: %v", err)
	}

	if len(ticks) != 3 {
		t.Fatalf("expected 3 cpu entries (cpu, cpu0, cpu1), got %d", len(ticks))
	}

	agg, ok := ticks["cpu"]
	if !ok {
		t.Fatal("expected 'cpu' aggregate entry")
	}

	if agg.User != 10000 {
		t.Errorf("expected User=10000, got %d", agg.User)
	}
	if agg.Nice != 500 {
		t.Errorf("expected Nice=500, got %d", agg.Nice)
	}
	if agg.System != 3000 {
		t.Errorf("expected System=3000, got %d", agg.System)
	}
	if agg.Idle != 80000 {
		t.Errorf("expected Idle=80000, got %d", agg.Idle)
	}
	if agg.IOWait != 1000 {
		t.Errorf("expected IOWait=1000, got %d", agg.IOWait)
	}
	if agg.IRQ != 100 {
		t.Errorf("expected IRQ=100, got %d", agg.IRQ)
	}
	if agg.SoftIRQ != 200 {
		t.Errorf("expected SoftIRQ=200, got %d", agg.SoftIRQ)
	}

	core0, ok := ticks["cpu0"]
	if !ok {
		t.Fatal("expected 'cpu0' entry")
	}
	if core0.User != 5000 || core0.Idle != 40000 {
		t.Errorf("unexpected core0 values: %+v", core0)
	}
}

func TestParseProcStat_Invalid(t *testing.T) {
	// Empty reader
	_, err := ParseProcStat(strings.NewReader(""))
	if err == nil {
		t.Error("expected error for empty /proc/stat, got nil")
	}

	// No cpu lines
	_, err = ParseProcStat(strings.NewReader("intr 12345\nctxt 6789\n"))
	if err == nil {
		t.Error("expected error for /proc/stat without cpu lines, got nil")
	}
}

func TestCalculateCPUUsage_Delta(t *testing.T) {
	t1, err := ParseProcStat(strings.NewReader(mockProcStatT1))
	if err != nil {
		t.Fatalf("parse T1 failed: %v", err)
	}
	t2, err := ParseProcStat(strings.NewReader(mockProcStatT2))
	if err != nil {
		t.Fatalf("parse T2 failed: %v", err)
	}

	stats := CalculateCPUUsage(t1, t2)

	// In T1 to T2 for aggregate "cpu":
	// User: 10000 -> 10080 (+80)
	// Nice: 500 -> 500 (+0)
	// System: 3000 -> 3020 (+20)
	// Idle: 80000 -> 80080 (+80)
	// IOWait: 1000 -> 1020 (+20)
	// IRQ: 100 -> 100 (+0)
	// SoftIRQ: 200 -> 200 (+0)
	// Delta Total = 80 + 0 + 20 + 80 + 20 + 0 + 0 = 200 ticks
	// Delta Idle = Idle (80) + IOWait (20) = 100 ticks
	// Total Usage = ((200 - 100) / 200) * 100 = 50.0%
	// User Usage = (80 / 200) * 100 = 40.0%
	// System Usage = (20 / 200) * 100 = 10.0%
	// IOWait Usage = (20 / 200) * 100 = 10.0%

	if stats.TotalUsage != 50.0 {
		t.Errorf("expected aggregate TotalUsage=50.0, got %f", stats.TotalUsage)
	}
	if stats.UserUsage != 40.0 {
		t.Errorf("expected aggregate UserUsage=40.0, got %f", stats.UserUsage)
	}
	if stats.SystemUsage != 10.0 {
		t.Errorf("expected aggregate SystemUsage=10.0, got %f", stats.SystemUsage)
	}
	if stats.IOWaitUsage != 10.0 {
		t.Errorf("expected aggregate IOWaitUsage=10.0, got %f", stats.IOWaitUsage)
	}

	// Verify Cores
	if len(stats.Cores) != 2 {
		t.Fatalf("expected 2 cores, got %d", len(stats.Cores))
	}

	// cpu0:
	// User: +50, System: +10, Idle: +30, IOWait: +10
	// Total delta = 100, Idle delta = 40
	// Total Usage = ((100 - 40) / 100) * 100 = 60.0%
	core0 := stats.Cores[0]
	if core0.ID != "cpu0" {
		t.Errorf("expected Cores[0].ID='cpu0', got %s", core0.ID)
	}
	if core0.TotalUsage != 60.0 {
		t.Errorf("expected cpu0 TotalUsage=60.0, got %f", core0.TotalUsage)
	}

	// cpu1:
	// User: +30, System: +10, Idle: +50, IOWait: +10
	// Total delta = 100, Idle delta = 60
	// Total Usage = ((100 - 60) / 100) * 100 = 40.0%
	core1 := stats.Cores[1]
	if core1.ID != "cpu1" {
		t.Errorf("expected Cores[1].ID='cpu1', got %s", core1.ID)
	}
	if core1.TotalUsage != 40.0 {
		t.Errorf("expected cpu1 TotalUsage=40.0, got %f", core1.TotalUsage)
	}
}

func TestCalculateCPUUsage_ZeroDelta(t *testing.T) {
	ticks, _ := ParseProcStat(strings.NewReader(mockProcStatT1))
	stats := CalculateCPUUsage(ticks, ticks)

	if stats.TotalUsage != 0.0 {
		t.Errorf("expected 0.0 usage when delta is zero, got %f", stats.TotalUsage)
	}
	for _, core := range stats.Cores {
		if core.TotalUsage != 0.0 {
			t.Errorf("expected core %s to have 0.0 usage, got %f", core.ID, core.TotalUsage)
		}
	}
}

// MockProcFS implements ProcFS for unit tests
type mockProcFS struct {
	content string
}

func (m *mockProcFS) Open(name string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte(m.content))), nil
}

func TestCPUCollector_CollectLifecycle(t *testing.T) {
	fs := &mockProcFS{content: mockProcStatT1}
	col := NewCPUCollector(fs)

	// First collection should initialize baseline
	s1, err := col.Collect()
	if err != nil {
		t.Fatalf("first collect error: %v", err)
	}
	if s1.TotalUsage != 0.0 {
		t.Errorf("first collect should return 0.0, got %f", s1.TotalUsage)
	}
	if len(s1.Cores) != 2 {
		t.Errorf("expected 2 cores, got %d", len(s1.Cores))
	}

	// Update mock content to T2
	fs.content = mockProcStatT2
	s2, err := col.Collect()
	if err != nil {
		t.Fatalf("second collect error: %v", err)
	}
	if s2.TotalUsage != 50.0 {
		t.Errorf("second collect should return 50.0, got %f", s2.TotalUsage)
	}
}

func BenchmarkParseProcStat(b *testing.B) {
	data := []byte(mockProcStatT1)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = ParseProcStat(bytes.NewReader(data))
	}
}

func BenchmarkCalculateCPUUsage(b *testing.B) {
	t1, _ := ParseProcStat(strings.NewReader(mockProcStatT1))
	t2, _ := ParseProcStat(strings.NewReader(mockProcStatT2))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = CalculateCPUUsage(t1, t2)
	}
}
