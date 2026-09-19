package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

func TestSQLiteStorage_BackupAndRestore(t *testing.T) {
	tempDir := t.TempDir()
	originalDBPath := filepath.Join(tempDir, "original.db")
	backupPath := filepath.Join(tempDir, "backups", "snapshot.db")
	restoredDBPath := filepath.Join(tempDir, "restored.db")

	// 1. Create and populate original database
	s, err := NewSQLiteStorage(originalDBPath)
	if err != nil {
		t.Fatalf("failed to create original sqlite: %v", err)
	}

	testEntries := []model.LogEntry{
		{
			Timestamp:  time.Now().UTC().Add(-10 * time.Second),
			Level:      "INFO",
			Service:    "auth-service",
			Message:    "Initial admin session created",
			Attributes: map[string]string{"session_id": "sess_99"},
		},
		{
			Timestamp:  time.Now().UTC(),
			Level:      "ERROR",
			Service:    "payment",
			Message:    "Webhook delivery failed",
			Attributes: map[string]string{"attempt": "3"},
		},
	}

	if err := s.WriteBatch(testEntries); err != nil {
		t.Fatalf("failed to write test batch: %v", err)
	}

	// 2. Perform online backup
	if err := s.Backup(backupPath); err != nil {
		t.Fatalf("backup failed: %v", err)
	}
	_ = s.Close()

	// 3. Restore to new database path
	if err := RestoreDatabase(backupPath, restoredDBPath); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// 4. Open restored database and verify integrity
	restoredStorage, err := NewSQLiteStorage(restoredDBPath)
	if err != nil {
		t.Fatalf("failed to open restored sqlite: %v", err)
	}
	defer restoredStorage.Close()

	count, err := restoredStorage.CountLogs()
	if err != nil {
		t.Fatalf("count restored logs: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 logs in restored database, got %d", count)
	}

	logs, err := restoredStorage.QueryLogs(model.LogFilter{Service: "payment"})
	if err != nil {
		t.Fatalf("query restored logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Message != "Webhook delivery failed" {
		t.Errorf("unexpected restored query: %+v", logs)
	}
}
