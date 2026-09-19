package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
	"github.com/FahmiYoshikage/sugi/internal/storage"
)

// APIHandler coordinates HTTP requests for metrics and log ingestion/querying.
type APIHandler struct {
	ringBuffer *storage.RingBuffer
	sqlite     *storage.SQLiteStorage
	logWriter  *storage.AsyncLogWriter
	sseHub     *SSEHub
	startTime  time.Time
	version    string
}

// NewAPIHandler creates a new APIHandler instance.
func NewAPIHandler(
	ringBuffer *storage.RingBuffer,
	sqlite *storage.SQLiteStorage,
	logWriter *storage.AsyncLogWriter,
	sseHub *SSEHub,
	version string,
) *APIHandler {
	if sseHub == nil {
		sseHub = NewSSEHub()
	}
	return &APIHandler{
		ringBuffer: ringBuffer,
		sqlite:     sqlite,
		logWriter:  logWriter,
		sseHub:     sseHub,
		startTime:  time.Now().UTC(),
		version:    version,
	}
}

// SSEHub returns the associated SSEHub.
func (h *APIHandler) SSEHub() *SSEHub {
	return h.sseHub
}

// RegisterRoutes registers all REST endpoints on the provided ServeMux.
func (h *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/logs", h.HandleIngestLogs)
	mux.HandleFunc("GET /api/v1/logs", h.HandleQueryLogs)
	mux.HandleFunc("GET /api/v1/metrics", h.HandleGetMetrics)
	mux.HandleFunc("GET /api/v1/metrics/history", h.HandleGetMetricsHistory)
	mux.HandleFunc("GET /api/v1/stream", h.HandleSSEStream)
	mux.HandleFunc("GET /health", h.HandleHealth)
	mux.HandleFunc("GET /api/v1/health", h.HandleHealth)
}

// HandleHealth returns application health and uptime.
func (h *APIHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "ok",
		"version":     h.version,
		"uptime_sec":  int(time.Since(h.startTime).Seconds()),
		"stored_logs": h.getLogCount(),
	})
}

func (h *APIHandler) getLogCount() int64 {
	if h.sqlite != nil {
		c, _ := h.sqlite.CountLogs()
		return c
	}
	return 0
}

// HandleIngestLogs accepts log entries via JSON (single/array) or raw text.
func (h *APIHandler) HandleIngestLogs(w http.ResponseWriter, r *http.Request) {
	if h.logWriter == nil {
		http.Error(w, `{"error":"log writer not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Limit body to 10MB to prevent memory exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if len(bytes.TrimSpace(body)) == 0 {
		http.Error(w, `{"error":"empty request body"}`, http.StatusBadRequest)
		return
	}

	contentType := r.Header.Get("Content-Type")
	var entries []model.LogEntry

	if strings.Contains(contentType, "application/json") || bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) || bytes.HasPrefix(bytes.TrimSpace(body), []byte("[")) {
		entries, err = parseJSONLogs(body)
		if err != nil {
			http.Error(w, `{"error":"invalid json format: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
	} else {
		// Fallback: parse raw text line-by-line
		service := r.URL.Query().Get("service")
		if service == "" {
			service = r.Header.Get("X-Service-Name")
		}
		if service == "" {
			service = "default"
		}
		entries = parseRawTextLogs(body, service)
	}

	accepted := 0
	dropped := 0
	now := time.Now().UTC()

	for _, entry := range entries {
		if entry.Timestamp.IsZero() {
			entry.Timestamp = now
		}
		if entry.Level == "" {
			entry.Level = "INFO"
		}
		if entry.Service == "" {
			entry.Service = "default"
		}

		if h.logWriter.Enqueue(entry) {
			accepted++
		} else {
			dropped++
		}
	}

	if accepted == 0 && dropped > 0 {
		writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"error":    "log ingestion buffer full",
			"accepted": 0,
			"dropped":  dropped,
		})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":   "accepted",
		"accepted": accepted,
		"dropped":  dropped,
	})
}

// HandleQueryLogs retrieves filtered logs from SQLite.
func (h *APIHandler) HandleQueryLogs(w http.ResponseWriter, r *http.Request) {
	if h.sqlite == nil {
		http.Error(w, `{"error":"sqlite storage not configured"}`, http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	filter := model.LogFilter{
		Level:   q.Get("level"),
		Service: q.Get("service"),
		Search:  q.Get("search"),
	}

	if limitStr := q.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = l
		}
	}
	if offsetStr := q.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			filter.Offset = o
		}
	}

	if startStr := q.Get("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			filter.StartTime = &t
		}
	}
	if endStr := q.Get("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			filter.EndTime = &t
		}
	}

	logs, err := h.sqlite.QueryLogs(filter)
	if err != nil {
		http.Error(w, `{"error":"query failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if logs == nil {
		logs = []model.LogEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(logs),
		"logs":  logs,
	})
}

// HandleGetMetrics returns the most recent system metrics snapshot.
func (h *APIHandler) HandleGetMetrics(w http.ResponseWriter, r *http.Request) {
	if h.ringBuffer == nil {
		http.Error(w, `{"error":"metrics buffer not configured"}`, http.StatusServiceUnavailable)
		return
	}

	latest, ok := h.ringBuffer.GetLatest()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": "waiting for initial metrics snapshot",
		})
		return
	}

	writeJSON(w, http.StatusOK, latest)
}

// HandleGetMetricsHistory returns historical snapshots stored in the ring buffer.
func (h *APIHandler) HandleGetMetricsHistory(w http.ResponseWriter, r *http.Request) {
	if h.ringBuffer == nil {
		http.Error(w, `{"error":"metrics buffer not configured"}`, http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	nStr := q.Get("n")
	var snapshots []model.SystemSnapshot

	if nStr != "" {
		if n, err := strconv.Atoi(nStr); err == nil && n > 0 {
			snapshots = h.ringBuffer.GetLastN(n)
		}
	}

	if snapshots == nil {
		snapshots = h.ringBuffer.GetAll()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":     len(snapshots),
		"capacity":  h.ringBuffer.Capacity(),
		"snapshots": snapshots,
	})
}

// Helper: parse JSON logs (array or single object)
func parseJSONLogs(data []byte) ([]model.LogEntry, error) {
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("[")) {
		var entries []model.LogEntry
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return nil, err
		}
		return entries, nil
	}

	var single model.LogEntry
	if err := json.Unmarshal(trimmed, &single); err != nil {
		return nil, err
	}
	return []model.LogEntry{single}, nil
}

// Helper: parse raw text line-delimited logs
func parseRawTextLogs(data []byte, service string) []model.LogEntry {
	var entries []model.LogEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	now := time.Now().UTC()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		level := detectLogLevel(line)
		entries = append(entries, model.LogEntry{
			Timestamp: now,
			Level:     level,
			Service:   service,
			Message:   line,
		})
	}

	return entries
}

// detectLogLevel heuristically extracts log severity from raw log text.
func detectLogLevel(line string) string {
	upper := strings.ToUpper(line)
	switch {
	case strings.Contains(upper, "ERROR") || strings.Contains(upper, "FATAL") || strings.Contains(upper, "CRIT"):
		return "ERROR"
	case strings.Contains(upper, "WARN"):
		return "WARN"
	case strings.Contains(upper, "DEBUG") || strings.Contains(upper, "TRACE"):
		return "DEBUG"
	default:
		return "INFO"
	}
}

// writeJSON writes a JSON response with status code and headers.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
