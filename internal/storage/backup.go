package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Backup creates a point-in-time, consistent snapshot of the active SQLite database.
// It uses SQLite's VACUUM INTO command for zero-downtime, non-blocking online backups.
func (s *SQLiteStorage) Backup(destPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	// SQLite VACUUM INTO requires target file to not exist prior to command
	if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing backup file: %w", err)
	}

	// First flush WAL frames to the main database file
	if _, err := s.db.Exec("PRAGMA wal_checkpoint(PASSIVE);"); err != nil {
		return fmt.Errorf("wal checkpoint: %w", err)
	}

	// Execute atomic snapshot backup
	query := fmt.Sprintf("VACUUM INTO '%s';", destPath)
	if _, err := s.db.Exec(query); err != nil {
		return fmt.Errorf("execute vacuum into: %w", err)
	}

	return nil
}

// RestoreDatabase safely copies a backup file into the target database path.
// It also cleans up any residual WAL/SHM files.
func RestoreDatabase(backupPath string, destDBPath string) error {
	src, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer src.Close()

	destDir := filepath.Dir(destDBPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}

	// Remove old db and WAL/SHM artifacts
	_ = os.Remove(destDBPath)
	_ = os.Remove(destDBPath + "-wal")
	_ = os.Remove(destDBPath + "-shm")

	dest, err := os.OpenFile(destDBPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create restored db file: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, src); err != nil {
		return fmt.Errorf("copy backup data: %w", err)
	}

	return nil
}
