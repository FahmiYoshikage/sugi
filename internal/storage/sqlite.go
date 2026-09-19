package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
	_ "modernc.org/sqlite"
)

// SQLiteStorage manages persistent log storage using pure Go embedded SQLite.
type SQLiteStorage struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteStorage opens or creates a SQLite database with WAL mode enabled.
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	if dbPath == "" {
		dbPath = "sugi.db"
	}

	// modernc.org/sqlite connection string
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db %q: %w", dbPath, err)
	}

	// Configure pool limits for embedded single-process observability
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(1 * time.Hour)

	storage := &SQLiteStorage{db: db}
	if err := storage.initSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init sqlite schema: %w", err)
	}

	return storage, nil
}

// initSchema applies PRAGMAs and creates tables and indices if not present.
func (s *SQLiteStorage) initSchema() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA cache_size=-2000;", // ~2MB cache size
	}

	for _, pragma := range pragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			return fmt.Errorf("exec %q: %w", pragma, err)
		}
	}

	schema := `
	CREATE TABLE IF NOT EXISTS system_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		level TEXT NOT NULL,
		service TEXT NOT NULL,
		message TEXT NOT NULL,
		attributes TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON system_logs(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_logs_service_ts ON system_logs(service, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_logs_level_ts ON system_logs(level, timestamp DESC);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("create tables/indices: %w", err)
	}

	return nil
}

// WriteLog inserts a single log entry into SQLite.
func (s *SQLiteStorage) WriteLog(entry model.LogEntry) error {
	return s.WriteBatch([]model.LogEntry{entry})
}

// WriteBatch inserts multiple log entries in a single atomic transaction.
func (s *SQLiteStorage) WriteBatch(entries []model.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.Prepare(`
		INSERT INTO system_logs (timestamp, level, service, message, attributes)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert statement: %w", err)
	}
	defer stmt.Close()

	for _, entry := range entries {
		ts := entry.Timestamp.UnixMilli()
		if ts == 0 {
			ts = time.Now().UTC().UnixMilli()
		}

		var attrStr string
		if len(entry.Attributes) > 0 {
			data, err := json.Marshal(entry.Attributes)
			if err == nil {
				attrStr = string(data)
			}
		}

		level := strings.ToUpper(strings.TrimSpace(entry.Level))
		if level == "" {
			level = "INFO"
		}

		service := strings.TrimSpace(entry.Service)
		if service == "" {
			service = "default"
		}

		if _, err := stmt.Exec(ts, level, service, entry.Message, attrStr); err != nil {
			return fmt.Errorf("exec insert: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch transaction: %w", err)
	}

	return nil
}

// QueryLogs searches logs according to criteria defined in LogFilter.
func (s *SQLiteStorage) QueryLogs(filter model.LogFilter) ([]model.LogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var (
		conditions []string
		args       []interface{}
	)

	if filter.StartTime != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, filter.StartTime.UnixMilli())
	}
	if filter.EndTime != nil {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, filter.EndTime.UnixMilli())
	}
	if filter.Level != "" {
		conditions = append(conditions, "level = ?")
		args = append(args, strings.ToUpper(filter.Level))
	}
	if filter.Service != "" {
		conditions = append(conditions, "service = ?")
		args = append(args, filter.Service)
	}
	if filter.Search != "" {
		conditions = append(conditions, "message LIKE ?")
		args = append(args, "%"+filter.Search+"%")
	}

	query := "SELECT id, timestamp, level, service, message, attributes FROM system_logs"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY timestamp DESC"

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query += " LIMIT ?"
	args = append(args, limit)

	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query logs: %w", err)
	}
	defer rows.Close()

	var result []model.LogEntry
	for rows.Next() {
		var (
			entry   model.LogEntry
			tsMilli int64
			attrStr sql.NullString
		)

		if err := rows.Scan(&entry.ID, &tsMilli, &entry.Level, &entry.Service, &entry.Message, &attrStr); err != nil {
			return nil, fmt.Errorf("scan log row: %w", err)
		}

		entry.Timestamp = time.UnixMilli(tsMilli).UTC()
		if attrStr.Valid && attrStr.String != "" {
			var attrs map[string]string
			if err := json.Unmarshal([]byte(attrStr.String), &attrs); err == nil {
				entry.Attributes = attrs
			}
		}

		result = append(result, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return result, nil
}

// PruneOldLogs removes records older than the specified retention duration.
// Returns the count of pruned log entries.
func (s *SQLiteStorage) PruneOldLogs(retention time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	thresholdMilli := time.Now().UTC().Add(-retention).UnixMilli()
	res, err := s.db.Exec("DELETE FROM system_logs WHERE timestamp < ?", thresholdMilli)
	if err != nil {
		return 0, fmt.Errorf("prune logs older than %s: %w", retention, err)
	}

	return res.RowsAffected()
}

// CountLogs returns the total count of stored logs.
func (s *SQLiteStorage) CountLogs() (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM system_logs").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count logs: %w", err)
	}
	return count, nil
}

// Close gracefully closes the SQLite database connection.
func (s *SQLiteStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}
