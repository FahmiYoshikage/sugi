package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestServerLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "server_test.db")
	cfg := Config{
		Port:           0, // not starting listener on real port in this unit test
		DBPath:         dbPath,
		Retention:      1 * time.Hour,
		SampleInterval: 50 * time.Millisecond,
		Version:        "test-version",
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Start sampling in background
	go srv.metricSamplingLoop()

	// Wait for at least 1 sample to be collected
	time.Sleep(150 * time.Millisecond)

	// Verify ring buffer has snapshots
	if srv.ringBuffer.Size() == 0 {
		t.Error("expected ring buffer to have received samples")
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}
