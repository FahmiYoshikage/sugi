package storage

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

// AsyncLogWriterConfig configures the asynchronous batch log writer.
type AsyncLogWriterConfig struct {
	BufferSize    int           // Channel capacity, e.g. 5000
	BatchSize     int           // Maximum logs per SQLite batch insert, e.g. 200
	FlushInterval time.Duration // Maximum wait time before flushing a partial batch, e.g. 500ms
	Retention     time.Duration // Log retention period, e.g. 7 * 24 * time.Hour
	PruneInterval time.Duration // Interval to run retention pruning, e.g. 1 * time.Hour
}

// DefaultAsyncLogWriterConfig provides sensible production defaults.
func DefaultAsyncLogWriterConfig() AsyncLogWriterConfig {
	return AsyncLogWriterConfig{
		BufferSize:    5000,
		BatchSize:     200,
		FlushInterval: 500 * time.Millisecond,
		Retention:     7 * 24 * time.Hour,
		PruneInterval: 1 * time.Hour,
	}
}

// AsyncLogWriter buffers incoming logs in a non-blocking Go channel and writes them to SQLite in atomic batches.
type AsyncLogWriter struct {
	storage *SQLiteStorage
	cfg     AsyncLogWriterConfig
	ch      chan model.LogEntry
	done    chan struct{}
	wg      sync.WaitGroup
	closed  bool
	mu      sync.Mutex
}

// NewAsyncLogWriter initializes and starts the background writer and pruner.
func NewAsyncLogWriter(storage *SQLiteStorage, cfg AsyncLogWriterConfig) *AsyncLogWriter {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 5000
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 200
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 500 * time.Millisecond
	}
	if cfg.Retention <= 0 {
		cfg.Retention = 7 * 24 * time.Hour
	}
	if cfg.PruneInterval <= 0 {
		cfg.PruneInterval = 1 * time.Hour
	}

	w := &AsyncLogWriter{
		storage: storage,
		cfg:     cfg,
		ch:      make(chan model.LogEntry, cfg.BufferSize),
		done:    make(chan struct{}),
	}

	w.wg.Add(2)
	go w.batchWorker()
	go w.pruneWorker()

	return w
}

// Enqueue submits a log entry to the write buffer.
// Returns true if enqueued, or false if the buffer is full (drop policy to protect system RAM).
func (w *AsyncLogWriter) Enqueue(entry model.LogEntry) bool {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return false
	}
	w.mu.Unlock()

	select {
	case w.ch <- entry:
		return true
	default:
		// Buffer is full: drop to prevent memory explosion and caller blockage
		return false
	}
}

// batchWorker continuously batches logs and commits them to SQLite.
func (w *AsyncLogWriter) batchWorker() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]model.LogEntry, 0, w.cfg.BatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := w.storage.WriteBatch(batch); err != nil {
			log.Printf("[AsyncLogWriter] Error writing batch of %d logs: %v", len(batch), err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry, ok := <-w.ch:
			if !ok {
				// Channel closed: flush remaining
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= w.cfg.BatchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-w.done:
			// Drain remaining entries in channel
			for {
				select {
				case entry, ok := <-w.ch:
					if !ok {
						flush()
						return
					}
					batch = append(batch, entry)
					if len(batch) >= w.cfg.BatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// pruneWorker runs periodic retention cleanup.
func (w *AsyncLogWriter) pruneWorker() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.cfg.PruneInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pruned, err := w.storage.PruneOldLogs(w.cfg.Retention)
			if err != nil {
				log.Printf("[AsyncLogWriter] Error pruning old logs: %v", err)
			} else if pruned > 0 {
				log.Printf("[AsyncLogWriter] Pruned %d expired logs (retention: %s)", pruned, w.cfg.Retention)
			}

		case <-w.done:
			return
		}
	}
}

// Close gracefully stops workers, flushes buffered logs, and shuts down writer.
func (w *AsyncLogWriter) Close(ctx context.Context) error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()

	close(w.done)

	// Wait for workers to finish with timeout context
	c := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(c)
	}()

	select {
	case <-c:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
