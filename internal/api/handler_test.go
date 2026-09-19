package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
	"github.com/FahmiYoshikage/sugi/internal/storage"
)

func setupTestServer(t *testing.T) (*APIHandler, http.Handler, *storage.AsyncLogWriter) {
	t.Helper()
	rb := storage.NewRingBuffer(100)

	dbPath := filepath.Join(t.TempDir(), "api_test.db")
	sqlStorage, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init sqlite: %v", err)
	}

	cfg := storage.DefaultAsyncLogWriterConfig()
	cfg.FlushInterval = 50 * time.Millisecond
	writer := storage.NewAsyncLogWriter(sqlStorage, cfg)

	h := NewAPIHandler(rb, sqlStorage, writer, "v0.1.0-test")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	handler := Chain(mux, CORSMiddleware(), RecoveryMiddleware())

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = writer.Close(ctx)
		_ = sqlStorage.Close()
	})

	return h, handler, writer
}

func TestHandleHealth(t *testing.T) {
	_, handler, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode health json: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
}

func TestHandleIngestLogs_JSON(t *testing.T) {
	_, handler, writer := setupTestServer(t)

	// Single JSON object
	single := `{"level":"INFO","service":"auth-service","message":"User logged in successfully","attributes":{"user_id":"42"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bytes.NewBufferString(single))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["accepted"].(float64) != 1 {
		t.Errorf("expected accepted=1, got %v", resp["accepted"])
	}

	// JSON Array of logs
	arr := `[
		{"level":"ERROR","service":"payment","message":"Charge card failed"},
		{"level":"WARN","service":"payment","message":"Retry attempt 1"}
	]`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bytes.NewBufferString(arr))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()

	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", rec2.Code)
	}

	// Allow flush
	time.Sleep(150 * time.Millisecond)
	_ = writer
}

func TestHandleIngestLogs_RawText(t *testing.T) {
	_, handler, _ := setupTestServer(t)

	raw := `2026-09-20 00:00:01 INFO Starting application
2026-09-20 00:00:02 ERROR Connection refused to redis:6379
2026-09-20 00:00:03 WARN Retrying redis connection
`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs?service=worker", bytes.NewBufferString(raw))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["accepted"].(float64) != 3 {
		t.Errorf("expected accepted=3, got %v", resp["accepted"])
	}
}

func TestHandleQueryLogs(t *testing.T) {
	h, handler, writer := setupTestServer(t)

	// Directly enqueue and flush logs
	writer.Enqueue(model.LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     "ERROR",
		Service:   "order-service",
		Message:   "Out of stock for item 101",
	})
	writer.Enqueue(model.LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     "INFO",
		Service:   "user-service",
		Message:   "Profile viewed",
	})

	time.Sleep(150 * time.Millisecond)

	// Query with service filter
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?service=order-service", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp struct {
		Count int              `json:"count"`
		Logs  []model.LogEntry `json:"logs"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}

	if resp.Count != 1 || resp.Logs[0].Service != "order-service" {
		t.Errorf("unexpected query result: %+v", resp)
	}

	_ = h
}

func TestHandleGetMetrics(t *testing.T) {
	h, handler, _ := setupTestServer(t)

	// Before any metrics
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	// Push a metric
	h.ringBuffer.Push(model.SystemSnapshot{
		Timestamp: time.Now().UTC(),
		CPU: model.CPUStats{
			TotalUsage: 42.5,
		},
	})

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)

	var snap model.SystemSnapshot
	if err := json.NewDecoder(rec2.Body).Decode(&snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}

	if snap.CPU.TotalUsage != 42.5 {
		t.Errorf("expected CPU 42.5, got %f", snap.CPU.TotalUsage)
	}
}

func TestHandleGetMetricsHistory(t *testing.T) {
	h, handler, _ := setupTestServer(t)

	for i := 1; i <= 5; i++ {
		h.ringBuffer.Push(model.SystemSnapshot{
			Timestamp: time.Now().UTC(),
			CPU: model.CPUStats{
				TotalUsage: float64(i * 10),
			},
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/history?n=3", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Count     int                    `json:"count"`
		Snapshots []model.SystemSnapshot `json:"snapshots"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Count != 3 || len(resp.Snapshots) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", resp.Count)
	}
}
