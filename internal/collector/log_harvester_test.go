package collector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

func TestParseSyslogLine_RFC5424(t *testing.T) {
	raw := "2026-09-21T12:00:01.123456+00:00 myhost sshd[4012]: Failed password for invalid user admin from 192.168.1.50 port 49123 ssh2"
	entry := ParseSyslogLine(raw, "/var/log/auth.log")

	if entry.Service != "sshd" {
		t.Errorf("expected service 'sshd', got %q", entry.Service)
	}
	if entry.Level != "ERROR" {
		t.Errorf("expected level 'ERROR', got %q", entry.Level)
	}
	if entry.Attributes["pid"] != "4012" {
		t.Errorf("expected pid '4012', got %q", entry.Attributes["pid"])
	}
	if entry.Attributes["hostname"] != "myhost" {
		t.Errorf("expected hostname 'myhost', got %q", entry.Attributes["hostname"])
	}
	if !strings.Contains(entry.Message, "Failed password") {
		t.Errorf("expected message to contain 'Failed password', got %q", entry.Message)
	}
}

func TestParseSyslogLine_RFC3164(t *testing.T) {
	raw := "Sep 21 12:00:00 myhost systemd[1]: Starting Sugi Observability Engine..."
	entry := ParseSyslogLine(raw, "/var/log/syslog")

	if entry.Service != "systemd" {
		t.Errorf("expected service 'systemd', got %q", entry.Service)
	}
	if entry.Level != "INFO" {
		t.Errorf("expected level 'INFO', got %q", entry.Level)
	}
	if entry.Attributes["pid"] != "1" {
		t.Errorf("expected pid '1', got %q", entry.Attributes["pid"])
	}
	if !strings.Contains(entry.Message, "Starting Sugi") {
		t.Errorf("expected message to contain 'Starting Sugi', got %q", entry.Message)
	}
}

func TestParseSyslogLine_WarnInference(t *testing.T) {
	raw := "2026-09-21T12:00:01Z host kernel: [ 1234.56 ] Warning: CPU temperature high, throttling"
	entry := ParseSyslogLine(raw, "/var/log/syslog")

	if entry.Level != "WARN" {
		t.Errorf("expected level 'WARN', got %q", entry.Level)
	}
}

func TestLogHarvester_Tailing(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// Write initial lines
	initialContent := "2026-09-21T10:00:00Z srv app: Line 1 initialized\n2026-09-21T10:00:01Z srv app: Line 2 running\n"
	if err := os.WriteFile(logFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write initial log: %v", err)
	}

	var mu sync.Mutex
	var collected []model.LogEntry

	cfg := LogHarvesterConfig{
		AutoSyslog:   false,
		WatchPaths:   []string{logFile},
		TailLines:    10,
		PollInterval: 50 * time.Millisecond,
		MaxRate:      100,
	}

	harvester := NewLogHarvester(cfg, func(entry model.LogEntry) {
		mu.Lock()
		collected = append(collected, entry)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	harvester.Start(ctx)

	// Wait for initial lines to be ingested
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	initialCount := len(collected)
	mu.Unlock()

	if initialCount < 2 {
		t.Errorf("expected at least 2 initial lines, got %d", initialCount)
	}

	// Append a new line
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open for append: %v", err)
	}
	_, _ = f.WriteString("2026-09-21T10:00:02Z srv app: Line 3 appended dynamically\n")
	_ = f.Close()

	// Wait for tailer to catch append
	time.Sleep(200 * time.Millisecond)

	harvester.Stop()

	mu.Lock()
	finalCount := len(collected)
	mu.Unlock()

	if finalCount <= initialCount {
		t.Errorf("expected appended line to be collected, initial=%d, final=%d", initialCount, finalCount)
	}
}

func TestReadLastLines(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "multiline.log")

	var content strings.Builder
	for i := 1; i <= 20; i++ {
		content.WriteString(fmt.Sprintf("Line %d\n", i))
	}
	if err := os.WriteFile(logFile, []byte(content.String()), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	f, err := os.Open(logFile)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer f.Close()

	lines := readLastLines(f, 5)
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	if lines[len(lines)-1] != "Line 20" {
		t.Errorf("expected last line 'Line 20', got %q", lines[len(lines)-1])
	}
}
