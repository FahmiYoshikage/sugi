package model

import "time"

// LogEntry represents a structured log record ingested or retrieved.
type LogEntry struct {
	ID         int64             `json:"id,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	Level      string            `json:"level"`                // "INFO", "WARN", "ERROR", "DEBUG", etc.
	Service    string            `json:"service"`              // Originating service/application name
	Message    string            `json:"message"`              // Main log message body
	Attributes map[string]string `json:"attributes,omitempty"` // Arbitrary key-value metadata/labels
}

// LogFilter defines query options for filtering logs from persistent storage.
type LogFilter struct {
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Level     string     `json:"level,omitempty"`
	Service   string     `json:"service,omitempty"`
	Search    string     `json:"search,omitempty"` // Substring search in message
	Limit     int        `json:"limit,omitempty"`
	Offset    int        `json:"offset,omitempty"`
}
