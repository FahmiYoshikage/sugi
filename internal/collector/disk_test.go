package collector

import (
	"strings"
	"testing"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

const mockDiskstatsT1 = ` 259       0 nvme0n1 1000 0 20000 500 500 0 10000 250 0 100 500 0 0 0 0 0 0
 259       1 nvme0n1p1 500 0 10000 250 250 0 5000 125 0 50 250 0 0 0 0 0 0
   8       0 sda 2000 0 40000 1000 1000 0 20000 500 0 200 1000 0 0 0 0 0 0
   8       1 sda1 1000 0 20000 500 500 0 10000 250 0 100 500 0 0 0 0 0 0
   7       0 loop0 100 0 200 50 0 0 0 0 0 10 50 0 0 0 0 0 0
`

const mockDiskstatsT2 = ` 259       0 nvme0n1 1100 0 24000 550 550 0 12000 280 0 110 550 0 0 0 0 0 0
 259       1 nvme0n1p1 550 0 12000 275 275 0 6000 140 0 55 275 0 0 0 0 0 0
   8       0 sda 2200 0 48000 1100 1100 0 24000 600 0 220 1100 0 0 0 0 0 0
   8       1 sda1 1100 0 24000 550 550 0 12000 300 0 110 550 0 0 0 0 0 0
   7       0 loop0 100 0 200 50 0 0 0 0 0 10 50 0 0 0 0 0 0
`

func TestParseProcDiskstats(t *testing.T) {
	reader := strings.NewReader(mockDiskstatsT1)
	stats, err := ParseProcDiskstats(reader)
	if err != nil {
		t.Fatalf("unexpected error parsing diskstats: %v", err)
	}

	// loop0 should be skipped, so remaining are nvme0n1, nvme0n1p1, sda, sda1 (4 devices)
	if len(stats) != 4 {
		t.Fatalf("expected 4 devices (loop0 skipped), got %d", len(stats))
	}

	if _, ok := stats["loop0"]; ok {
		t.Error("expected loop0 to be filtered out")
	}

	nvme, ok := stats["nvme0n1"]
	if !ok {
		t.Fatal("expected nvme0n1 in parsed stats")
	}
	if nvme.ReadsCompleted != 1000 {
		t.Errorf("expected ReadsCompleted=1000, got %d", nvme.ReadsCompleted)
	}
	if nvme.SectorsRead != 20000 {
		t.Errorf("expected SectorsRead=20000, got %d", nvme.SectorsRead)
	}
	if nvme.WritesCompleted != 500 {
		t.Errorf("expected WritesCompleted=500, got %d", nvme.WritesCompleted)
	}
	if nvme.SectorsWritten != 10000 {
		t.Errorf("expected SectorsWritten=10000, got %d", nvme.SectorsWritten)
	}
}

func TestCalculateDiskStats_Rates(t *testing.T) {
	t1, err := ParseProcDiskstats(strings.NewReader(mockDiskstatsT1))
	if err != nil {
		t.Fatalf("parse T1 failed: %v", err)
	}
	t2, err := ParseProcDiskstats(strings.NewReader(mockDiskstatsT2))
	if err != nil {
		t.Fatalf("parse T2 failed: %v", err)
	}

	// Elapsed 2 seconds
	duration := 2 * time.Second
	stats := CalculateDiskStats(t1, t2, duration)

	var nvmeDev *model.DiskDeviceStats
	for i := range stats.Devices {
		if stats.Devices[i].Device == "nvme0n1" {
			nvmeDev = &stats.Devices[i]
			break
		}
	}
	if nvmeDev == nil {
		t.Fatal("nvme0n1 not found in calculated stats")
	}

	// nvme0n1 delta:
	// sectors_read delta: 4000 * 512 = 2,048,000 bytes / 2s = 1,024,000 B/s
	// sectors_written delta: 2000 * 512 = 1,024,000 bytes / 2s = 512,000 B/s
	// reads_completed delta: 100 / 2s = 50 IOPS
	// writes_completed delta: 50 / 2s = 25 IOPS
	if nvmeDev.ReadBytesPerSec != 1024000.0 {
		t.Errorf("expected ReadBytesPerSec=1024000, got %f", nvmeDev.ReadBytesPerSec)
	}
	if nvmeDev.WriteBytesPerSec != 512000.0 {
		t.Errorf("expected WriteBytesPerSec=512000, got %f", nvmeDev.WriteBytesPerSec)
	}
	if nvmeDev.ReadIOPS != 50.0 {
		t.Errorf("expected ReadIOPS=50.0, got %f", nvmeDev.ReadIOPS)
	}
	if nvmeDev.WriteIOPS != 25.0 {
		t.Errorf("expected WriteIOPS=25.0, got %f", nvmeDev.WriteIOPS)
	}

	// Total primary disks are nvme0n1 + sda
	// sda delta:
	// sectors_read delta: 8000 * 512 = 4,096,000 / 2s = 2,048,000 B/s
	// sectors_written delta: 4000 * 512 = 2,048,000 / 2s = 1,024,000 B/s
	// reads delta: 200 / 2s = 100 IOPS
	// writes delta: 100 / 2s = 50 IOPS
	// Totals:
	// Read: 1,024,000 + 2,048,000 = 3,072,000 B/s
	// Write: 512,000 + 1,024,000 = 1,536,000 B/s
	// ReadIOPS: 50 + 100 = 150 IOPS
	// WriteIOPS: 25 + 50 = 75 IOPS
	if stats.TotalReadBytesPerSec != 3072000.0 {
		t.Errorf("expected TotalReadBytesPerSec=3072000.0, got %f", stats.TotalReadBytesPerSec)
	}
	if stats.TotalWriteBytesPerSec != 1536000.0 {
		t.Errorf("expected TotalWriteBytesPerSec=1536000.0, got %f", stats.TotalWriteBytesPerSec)
	}
	if stats.TotalReadIOPS != 150.0 {
		t.Errorf("expected TotalReadIOPS=150.0, got %f", stats.TotalReadIOPS)
	}
	if stats.TotalWriteIOPS != 75.0 {
		t.Errorf("expected TotalWriteIOPS=75.0, got %f", stats.TotalWriteIOPS)
	}
}

func TestDiskCollector_Collect(t *testing.T) {
	fs := &mockProcFS{content: mockDiskstatsT1}
	col := NewDiskCollector(fs)

	s1, err := col.Collect()
	if err != nil {
		t.Fatalf("first collect error: %v", err)
	}
	if s1.TotalReadBytesPerSec != 0.0 {
		t.Errorf("first collect should have 0 rate, got %f", s1.TotalReadBytesPerSec)
	}

	fs.content = mockDiskstatsT2
	s2, err := col.Collect()
	if err != nil {
		t.Fatalf("second collect error: %v", err)
	}
	if len(s2.Devices) == 0 {
		t.Fatal("expected devices in second collect")
	}
}
