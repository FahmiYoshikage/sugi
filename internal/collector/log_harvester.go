package collector

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

// Known standard Linux system log files
var defaultSyslogCandidates = []string{
	"/var/log/syslog",   // Debian / Ubuntu
	"/var/log/auth.log", // Debian / Ubuntu security
	"/var/log/messages", // RHEL / CentOS / Fedora
}

// LogHarvesterConfig defines parameters for active file tailing.
type LogHarvesterConfig struct {
	AutoSyslog   bool          // Auto-detect and harvest standard Linux syslog files
	WatchPaths   []string      // Explicit file paths to tail
	TailLines    int           // Historical lines to ingest on startup (default: 50)
	PollInterval time.Duration // Interval to wait upon EOF (default: 1s)
	MaxRate      int           // Maximum lines ingested per second per file (default: 500)
}

// DefaultLogHarvesterConfig returns production defaults with safe resource boundaries.
func DefaultLogHarvesterConfig() LogHarvesterConfig {
	return LogHarvesterConfig{
		AutoSyslog:   true,
		WatchPaths:   nil,
		TailLines:    50,
		PollInterval: 1 * time.Second,
		MaxRate:      500,
	}
}

// LogHarvester monitors and harvests Linux host log files in real-time.
type LogHarvester struct {
	cfg      LogHarvesterConfig
	onLog    func(entry model.LogEntry)
	stopChan chan struct{}
	wg       sync.WaitGroup
	active   []string
	mu       sync.Mutex
}

// NewLogHarvester creates a new log harvester instance.
func NewLogHarvester(cfg LogHarvesterConfig, onLog func(entry model.LogEntry)) *LogHarvester {
	if cfg.TailLines <= 0 {
		cfg.TailLines = 50
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.MaxRate <= 0 {
		cfg.MaxRate = 500
	}

	return &LogHarvester{
		cfg:      cfg,
		onLog:    onLog,
		stopChan: make(chan struct{}),
	}
}

// ActiveFiles returns the list of log files currently being actively harvested.
func (h *LogHarvester) ActiveFiles() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	res := make([]string, len(h.active))
	copy(res, h.active)
	return res
}

// Start begins tailing detected log files in background goroutines.
func (h *LogHarvester) Start(ctx context.Context) {
	targets := make(map[string]struct{})

	// Add custom configured paths
	for _, p := range h.cfg.WatchPaths {
		p = strings.TrimSpace(p)
		if p != "" {
			targets[p] = struct{}{}
		}
	}

	// Auto-detect available system log files if enabled
	if h.cfg.AutoSyslog {
		for _, path := range defaultSyslogCandidates {
			if fileIsReadable(path) {
				targets[path] = struct{}{}
			}
		}
	}

	if len(targets) == 0 {
		log.Printf("[LogHarvester] No accessible system log files found. Passive API ingestion remains active.")
		return
	}

	for path := range targets {
		h.mu.Lock()
		h.active = append(h.active, path)
		h.mu.Unlock()

		h.wg.Add(1)
		go h.tailFile(ctx, path)
	}

	log.Printf("[LogHarvester] Actively harvesting %d log file(s): %v", len(targets), h.ActiveFiles())
}

// Stop gracefully signals all tailers to finish and waits for shutdown.
func (h *LogHarvester) Stop() {
	close(h.stopChan)
	h.wg.Wait()
}

// tailFile monitors a single file, ingests initial historical lines, and continuously tails appends.
func (h *LogHarvester) tailFile(ctx context.Context, path string) {
	defer h.wg.Done()

	file, err := os.Open(path)
	if err != nil {
		log.Printf("[LogHarvester] Unable to open %s: %v", path, err)
		return
	}
	defer file.Close()

	// 1. Ingest initial recent lines on startup
	if h.cfg.TailLines > 0 {
		initialLines := readLastLines(file, h.cfg.TailLines)
		for _, line := range initialLines {
			if strings.TrimSpace(line) != "" {
				entry := ParseSyslogLine(line, path)
				if h.onLog != nil {
					h.onLog(entry)
				}
			}
		}
	}

	// Seek to EOF to begin tailing live appends
	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		offset = 0
	}

	reader := bufio.NewReaderSize(file, 64*1024)
	rateTicker := time.NewTicker(time.Second)
	defer rateTicker.Stop()
	linesThisSecond := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopChan:
			return
		case <-rateTicker.C:
			linesThisSecond = 0
		default:
		}

		// Rate limiting check
		if linesThisSecond >= h.cfg.MaxRate {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// Check for file truncation / log rotation
				stat, statErr := file.Stat()
				if statErr == nil && stat.Size() < offset {
					// File was truncated or rotated
					_, _ = file.Seek(0, io.SeekStart)
					offset = 0
					reader.Reset(file)
					continue
				}

				// Wait before next check
				select {
				case <-ctx.Done():
					return
				case <-h.stopChan:
					return
				case <-time.After(h.cfg.PollInterval):
					continue
				}
			}

			// Transient I/O error
			time.Sleep(h.cfg.PollInterval)
			continue
		}

		offset += int64(len(line))
		linesThisSecond++

		trimmed := strings.TrimRight(line, "\r\n")
		if len(trimmed) > 4096 {
			trimmed = trimmed[:4096] // Truncate oversized lines
		}
		if trimmed != "" {
			entry := ParseSyslogLine(trimmed, path)
			if h.onLog != nil {
				h.onLog(entry)
			}
		}
	}
}

// ParseSyslogLine converts standard Linux syslog messages (RFC 5424 / RFC 3164) into structured LogEntry.
func ParseSyslogLine(raw string, sourcePath string) model.LogEntry {
	raw = strings.TrimSpace(raw)
	now := time.Now().UTC()

	entry := model.LogEntry{
		Timestamp: now,
		Level:     "INFO",
		Service:   "syslog",
		Message:   raw,
		Attributes: map[string]string{
			"source": sourcePath,
		},
	}

	if raw == "" {
		return entry
	}

	parts := strings.Fields(raw)
	if len(parts) < 3 {
		return entry
	}

	msgIndex := 0

	// 1. Try parsing RFC 5424 (ISO 8601: "2026-09-21T12:00:00.123456+07:00 ...")
	if t, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
		entry.Timestamp = t.UTC()
		msgIndex = 1
	} else if t, err := time.Parse(time.RFC3339, parts[0]); err == nil {
		entry.Timestamp = t.UTC()
		msgIndex = 1
	} else if len(parts) >= 3 {
		// 2. Try parsing RFC 3164 (BSD syslog: "Sep 21 12:00:00 ...")
		timeStr := parts[0] + " " + parts[1] + " " + parts[2]
		year := now.Year()
		timeStrWithYear := time.Now().Format("2006") + " " + timeStr
		if t, err := time.Parse("2006 Jan 2 15:04:05", timeStrWithYear); err == nil {
			// Adjust year if parsed date is in future
			if t.After(now.Add(24 * time.Hour)) {
				t = t.AddDate(-1, 0, 0)
			}
			entry.Timestamp = t.UTC()
			msgIndex = 3
		} else if t, err := time.Parse("2006 Jan _2 15:04:05", timeStrWithYear); err == nil {
			if t.After(now.Add(24 * time.Hour)) {
				t = t.AddDate(-1, 0, 0)
			}
			entry.Timestamp = t.UTC()
			msgIndex = 3
		}
		_ = year
	}

	// Extract hostname and service
	if msgIndex > 0 && len(parts) > msgIndex {
		// Hostname
		entry.Attributes["hostname"] = parts[msgIndex]
		msgIndex++

		// Service / Tag (e.g. "systemd[1]:" or "sshd[123]:" or "kernel:")
		if len(parts) > msgIndex {
			serviceToken := parts[msgIndex]
			serviceToken = strings.TrimSuffix(serviceToken, ":")

			// Check for PID in bracket: "sshd[1234]"
			if idx := strings.Index(serviceToken, "["); idx != -1 {
				entry.Service = serviceToken[:idx]
				if endIdx := strings.Index(serviceToken, "]"); endIdx > idx {
					entry.Attributes["pid"] = serviceToken[idx+1 : endIdx]
				}
			} else {
				entry.Service = serviceToken
			}
			msgIndex++
		}

		// Remaining tokens form the message body
		if len(parts) >= msgIndex {
			entry.Message = strings.Join(parts[msgIndex:], " ")
		}
	}

	// 3. Infer Log Level from content semantics
	lowerMsg := strings.ToLower(entry.Message)
	switch {
	case strings.Contains(lowerMsg, "fail") || strings.Contains(lowerMsg, "fatal") ||
		strings.Contains(lowerMsg, "error") || strings.Contains(lowerMsg, "panic") ||
		strings.Contains(lowerMsg, "segfault") || strings.Contains(lowerMsg, "critical") ||
		strings.Contains(lowerMsg, "alert") || strings.Contains(lowerMsg, "emerg"):
		entry.Level = "ERROR"
	case strings.Contains(lowerMsg, "warn") || strings.Contains(lowerMsg, "warning") ||
		strings.Contains(lowerMsg, "denied") || strings.Contains(lowerMsg, "reject") ||
		strings.Contains(lowerMsg, "timeout") || strings.Contains(lowerMsg, "retry"):
		entry.Level = "WARN"
	case strings.Contains(lowerMsg, "debug") || strings.Contains(lowerMsg, "trace"):
		entry.Level = "DEBUG"
	default:
		entry.Level = "INFO"
	}

	return entry
}

// readLastLines reads up to N lines backwards from the end of an open file without loading whole file.
func readLastLines(file *os.File, n int) []string {
	if n <= 0 {
		return nil
	}

	stat, err := file.Stat()
	if err != nil || stat.Size() == 0 {
		return nil
	}

	const bufSize = 64 * 1024
	fileSize := stat.Size()
	readBytes := int64(bufSize)
	if readBytes > fileSize {
		readBytes = fileSize
	}

	buf := make([]byte, readBytes)
	_, err = file.ReadAt(buf, fileSize-readBytes)
	if err != nil && err != io.EOF {
		return nil
	}

	lines := strings.Split(string(buf), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if len(lines) > n {
		return lines[len(lines)-n:]
	}
	return lines
}

func fileIsReadable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
