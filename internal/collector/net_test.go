package collector

import (
	"strings"
	"testing"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

const mockNetDevT1 = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000000     1000    0    0    0     0          0         0  1000000     1000    0    0    0     0       0          0
  eth0: 5000000     5000    0    0    0     0          0         0  2000000     2000    0    0    0     0       0          0
 wlan0: 8000000     8000    0    5    0     0          0         0  3000000     3000    0    2    0     0       0          0
`

const mockNetDevT2 = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1050000     1050    0    0    0     0          0         0  1050000     1050    0    0    0     0       0          0
  eth0: 5200000     5100    0    0    0     0          0         0  2100000     2050    0    0    0     0       0          0
 wlan0: 8400000     8200    0    5    0     0          0         0  3200000     3100    0    2    0     0       0          0
`

func TestParseProcNetDev(t *testing.T) {
	reader := strings.NewReader(mockNetDevT1)
	stats, err := ParseProcNetDev(reader)
	if err != nil {
		t.Fatalf("unexpected error parsing net/dev: %v", err)
	}

	if len(stats) != 3 {
		t.Fatalf("expected 3 interfaces (lo, eth0, wlan0), got %d", len(stats))
	}

	eth0, ok := stats["eth0"]
	if !ok {
		t.Fatal("expected eth0 interface in parsed stats")
	}
	if eth0.RxBytes != 5000000 {
		t.Errorf("expected RxBytes=5000000, got %d", eth0.RxBytes)
	}
	if eth0.TxBytes != 2000000 {
		t.Errorf("expected TxBytes=2000000, got %d", eth0.TxBytes)
	}
	if eth0.RxPackets != 5000 {
		t.Errorf("expected RxPackets=5000, got %d", eth0.RxPackets)
	}
	if eth0.TxPackets != 2000 {
		t.Errorf("expected TxPackets=2000, got %d", eth0.TxPackets)
	}

	wlan0, ok := stats["wlan0"]
	if !ok {
		t.Fatal("expected wlan0 interface in parsed stats")
	}
	if wlan0.RxDrops != 5 || wlan0.TxDrops != 2 {
		t.Errorf("unexpected drops for wlan0: %+v", wlan0)
	}
}

func TestCalculateNetStats_Rates(t *testing.T) {
	t1, err := ParseProcNetDev(strings.NewReader(mockNetDevT1))
	if err != nil {
		t.Fatalf("parse T1 failed: %v", err)
	}
	t2, err := ParseProcNetDev(strings.NewReader(mockNetDevT2))
	if err != nil {
		t.Fatalf("parse T2 failed: %v", err)
	}

	duration := 2 * time.Second
	stats := CalculateNetStats(t1, t2, duration)

	var eth0Stat *model.NetInterfaceStats
	for i := range stats.Interfaces {
		if stats.Interfaces[i].Name == "eth0" {
			eth0Stat = &stats.Interfaces[i]
			break
		}
	}
	if eth0Stat == nil {
		t.Fatal("eth0 not found in calculated stats")
	}

	// eth0 delta:
	// Rx: 200,000 bytes / 2s = 100,000 B/s
	// Tx: 100,000 bytes / 2s = 50,000 B/s
	// RxPackets: 100 / 2s = 50 pkt/s
	// TxPackets: 50 / 2s = 25 pkt/s
	if eth0Stat.RxBytesPerSec != 100000.0 {
		t.Errorf("expected RxBytesPerSec=100000.0, got %f", eth0Stat.RxBytesPerSec)
	}
	if eth0Stat.TxBytesPerSec != 50000.0 {
		t.Errorf("expected TxBytesPerSec=50000.0, got %f", eth0Stat.TxBytesPerSec)
	}
	if eth0Stat.RxPacketsPerSec != 50.0 {
		t.Errorf("expected RxPacketsPerSec=50.0, got %f", eth0Stat.RxPacketsPerSec)
	}
	if eth0Stat.TxPacketsPerSec != 25.0 {
		t.Errorf("expected TxPacketsPerSec=25.0, got %f", eth0Stat.TxPacketsPerSec)
	}

	// System totals (excluding lo):
	// eth0: Rx 100k, Tx 50k
	// wlan0: Rx 200k, Tx 100k
	// Total: Rx 300k, Tx 150k
	if stats.TotalRxBytesPerSec != 300000.0 {
		t.Errorf("expected TotalRxBytesPerSec=300000.0, got %f", stats.TotalRxBytesPerSec)
	}
	if stats.TotalTxBytesPerSec != 150000.0 {
		t.Errorf("expected TotalTxBytesPerSec=150000.0, got %f", stats.TotalTxBytesPerSec)
	}
}

func TestNetCollector_Collect(t *testing.T) {
	fs := &mockProcFS{content: mockNetDevT1}
	col := NewNetCollector(fs)

	s1, err := col.Collect()
	if err != nil {
		t.Fatalf("first collect failed: %v", err)
	}
	if s1.TotalRxBytesPerSec != 0.0 {
		t.Errorf("first collect rate should be 0, got %f", s1.TotalRxBytesPerSec)
	}

	fs.content = mockNetDevT2
	s2, err := col.Collect()
	if err != nil {
		t.Fatalf("second collect failed: %v", err)
	}
	if len(s2.Interfaces) != 3 {
		t.Errorf("expected 3 interfaces, got %d", len(s2.Interfaces))
	}
}
