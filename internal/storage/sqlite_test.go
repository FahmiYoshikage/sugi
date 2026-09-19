package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

func newTestStorage(t *testing.T) *SQLiteStorage {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_sugi.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create test sqlite storage: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	return s
}

func TestSQLiteStorage_WriteAndQuery(t *testing.T) {
	s := newTestStorage(t)

	now := time.Now().UTC()
	entries := []model.LogEntry{
		{
			Timestamp:  now.Add(-2 * time.Minute),
			Level:      "INFO",
			Service:    "api-gateway",
			Message:    "Request received",
			Attributes: map[string]string{"method": "GET", "path": "/health"},
		},
		{
			Timestamp:  now.Add(-1 * time.Minute),
			Level:      "WARN",
			Service:    "api-gateway",
			Message:    "High latency detected",
			Attributes: map[string]string{"latency_ms": "450"},
		},
		{
			Timestamp:  now,
			Level:      "ERROR",
			Service:    "auth-service",
			Message:    "Database connection timed out",
			Attributes: map[string]string{"db": "users_replica"},
		},
	}

	if err := s.WriteBatch(entries); err != nil {
		t.Fatalf("write batch failed: %v", err)
	}

	count, err := s.CountLogs()
	if err != nil {
		t.Fatalf("count logs failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 logs, got %d", count)
	}

	// Query all logs (should be ordered DESC: newest first)
	logs, err := s.QueryLogs(model.LogFilter{})
	if err != nil {
		t.Fatalf("query logs failed: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}

	if logs[0].Level != "ERROR" || logs[0].Service != "auth-service" {
		t.Errorf("expected newest log to be ERROR from auth-service, got %+v", logs[0])
	}
	if logs[0].Attributes["db"] != "users_replica" {
		t.Errorf("expected attribute db=users_replica, got %+v", logs[0].Attributes)
	}
}

func TestSQLiteStorage_Filters(t *testing.T) {
	s := newTestStorage(t)

	now := time.Now().UTC()
	_ = s.WriteBatch([]model.LogEntry{
		{Timestamp: now.Add(-10 * time.Minute), Level: "DEBUG", Service: "worker", Message: "Job started"},
		{Timestamp: now.Add(-5 * time.Minute), Level: "INFO", Service: "worker", Message: "Job completed"},
		{Timestamp: now.Add(-1 * time.Minute), Level: "ERROR", Service: "payment", Message: "Payment gateway token invalid"},
	})

	// Filter by Level
	errorLogs, err := s.QueryLogs(model.LogFilter{Level: "error"})
	if err != nil {
		t.Fatalf("query error level failed: %v", err)
	}
	if len(errorLogs) != 1 || errorLogs[0].Level != "ERROR" {
		t.Errorf("expected 1 ERROR log, got %d", len(errorLogs))
	}

	// Filter by Service
	workerLogs, err := s.QueryLogs(model.LogFilter{Service: "worker"})
	if err != nil {
		t.Fatalf("query service failed: %v", err)
	}
	if len(workerLogs) != 2 {
		t.Errorf("expected 2 worker logs, got %d", len(workerLogs))
	}

	// Filter by Search text
	searchLogs, err := s.QueryLogs(model.LogFilter{Search: "token"})
	if err != nil {
		t.Fatalf("query search failed: %v", err)
	}
	if len(searchLogs) != 1 || searchLogs[0].Service != "payment" {
		t.Errorf("expected 1 payment log containing 'token', got %d", len(searchLogs))
	}
}

func TestSQLiteStorage_PruneOldLogs(t *testing.T) {
	s := newTestStorage(t)

	now := time.Now().UTC()
	// Insert 2 old logs (10 days ago) and 1 recent log (1 hour ago)
	_ = s.WriteBatch([]model.LogEntry{
		{Timestamp: now.Add(-10 * 24 * time.Hour), Level: "INFO", Service: "app", Message: "Old log 1"},
		{Timestamp: now.Add(-8 * 24 * time.Hour), Level: "INFO", Service: "app", Message: "Old log 2"},
		{Timestamp: now.Add(-1 * time.Hour), Level: "INFO", Service: "app", Message: "Recent log"},
	})

	// Retention = 7 days
	pruned, err := s.PruneOldLogs(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if pruned != 2 {
		t.Errorf("expected 2 pruned logs, got %d", pruned)
	}

	count, err := s.CountLogs()
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 remaining log, got %d", count)
	}
}

func TestAsyncLogWriter_Lifecycle(t *testing.T) {
	s := newTestStorage(t)

	cfg := AsyncLogWriterConfig{
		BufferSize:    100,
		BatchSize:     10,
		FlushInterval: 100 * time.Millisecond,
		Retention:     7 * 24 * time.Hour,
		PruneInterval: 1 * time.Hour,
	}
	writer := NewAsyncLogWriter(s, cfg)

	// Enqueue 25 logs
	for i := 0; i < 25; i++ {
		ok := writer.Enqueue(model.LogEntry{
			Timestamp: time.Now().UTC(),
			Level:     "INFO",
			Service:   "test-svc",
			Message:   "Async log message",
		})
		if !ok {
			t.Fatalf("failed to enqueue log %d", i)
		}
	}

	// Close gracefully to flush all batches
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := writer.Close(ctx); err != nil {
		t.Fatalf("writer close failed: %v", err)
	}

	count, err := s.CountLogs()
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 25 {
		t.Errorf("expected 25 logs committed, got %d", count)
	}
}

func BenchmarkSQLite_WriteBatch(b *testing.B) {
	dbPath := filepath.Join(b.TempDir(), "bench_sugi.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		b.Fatalf("open sqlite: %v", err)
	}
	defer s.Close()

	batch := make([]model.LogEntry, 100)
	now := time.Now().UTC()
	for i := 0; i < 100; i++ {
		batch[i] = model.LogEntry{
			Timestamp:  now,
			Level:      "INFO",
			Service:    "bench-service",
			Message:    "Sample benchmark log entry for SQLite WAL testing",
			Attributes: map[string]string{"env": "production", "worker": "4"},
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = s.WriteBatch(batch)
	}
}
