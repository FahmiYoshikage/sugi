package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/api"
	"github.com/FahmiYoshikage/sugi/internal/collector"
	"github.com/FahmiYoshikage/sugi/internal/model"
	"github.com/FahmiYoshikage/sugi/internal/storage"
)

// Config configures the Sugi server orchestrator.
type Config struct {
	Port           int
	DBPath         string
	Retention      time.Duration
	SampleInterval time.Duration
	Version        string
}

// DefaultConfig returns production default configurations.
func DefaultConfig() Config {
	return Config{
		Port:           8080,
		DBPath:         "sugi.db",
		Retention:      7 * 24 * time.Hour,
		SampleInterval: 1 * time.Second,
		Version:        "0.1.0",
	}
}

// Server is the top-level Sugi orchestrator managing all subsystem lifecycles.
type Server struct {
	cfg        Config
	httpServer *http.Server
	ringBuffer *storage.RingBuffer
	sqlite     *storage.SQLiteStorage
	logWriter  *storage.AsyncLogWriter
	apiHandler *api.APIHandler

	cpuCol  *collector.CPUCollector
	memCol  *collector.MemCollector
	diskCol *collector.DiskCollector
	netCol  *collector.NetCollector

	stopChan chan struct{}
}

// NewServer initializes and wires all collectors, storage, and API components.
func NewServer(cfg Config) (*Server, error) {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "sugi.db"
	}
	if cfg.Retention <= 0 {
		cfg.Retention = 7 * 24 * time.Hour
	}
	if cfg.SampleInterval <= 0 {
		cfg.SampleInterval = 1 * time.Second
	}
	if cfg.Version == "" {
		cfg.Version = "0.1.0"
	}

	// 1. Storage: RingBuffer
	ringBuffer := storage.NewRingBuffer(storage.DefaultRingBufferCapacity)

	// 2. Storage: Pure Go SQLite
	sqlite, err := storage.NewSQLiteStorage(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("init sqlite storage: %w", err)
	}

	// 3. Storage: Async Batch Log Writer
	writerCfg := storage.DefaultAsyncLogWriterConfig()
	writerCfg.Retention = cfg.Retention
	logWriter := storage.NewAsyncLogWriter(sqlite, writerCfg)

	// 4. Collectors
	fs := collector.NewDefaultProcFS()
	cpuCol := collector.NewCPUCollector(fs)
	memCol := collector.NewMemCollector(fs)
	diskCol := collector.NewDiskCollector(fs)
	netCol := collector.NewNetCollector(fs)

	// Take baseline snapshot
	_, _ = cpuCol.Collect()
	_, _ = diskCol.Collect()
	_, _ = netCol.Collect()

	// 5. HTTP API Handler & Routing
	apiHandler := api.NewAPIHandler(ringBuffer, sqlite, logWriter, cfg.Version)
	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux)

	wrappedMux := api.Chain(
		mux,
		api.CORSMiddleware(),
		api.RecoveryMiddleware(),
		api.LoggerMiddleware(),
	)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      wrappedMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		cfg:        cfg,
		httpServer: httpServer,
		ringBuffer: ringBuffer,
		sqlite:     sqlite,
		logWriter:  logWriter,
		apiHandler: apiHandler,
		cpuCol:     cpuCol,
		memCol:     memCol,
		diskCol:    diskCol,
		netCol:     netCol,
		stopChan:   make(chan struct{}),
	}, nil
}

// Start begins background sampling and starts the HTTP server.
func (s *Server) Start() error {
	go s.metricSamplingLoop()

	log.Printf("[Sugi] HTTP server listening on port %d...", s.cfg.Port)
	log.Printf("[Sugi] Logs database: %s (retention: %s)", s.cfg.DBPath, s.cfg.Retention)
	log.Printf("[Sugi] Metric sampling rate: %s", s.cfg.SampleInterval)

	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// metricSamplingLoop periodically collects system metrics and pushes snapshots into the ring buffer.
func (s *Server) metricSamplingLoop() {
	ticker := time.NewTicker(s.cfg.SampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cpuStats, err := s.cpuCol.Collect()
			if err != nil {
				continue
			}
			memStats, err := s.memCol.Collect()
			if err != nil {
				continue
			}
			diskStats, _ := s.diskCol.Collect()
			netStats, _ := s.netCol.Collect()

			snapshot := model.SystemSnapshot{
				Timestamp: time.Now().UTC(),
				CPU:       cpuStats,
				Memory:    memStats,
				Disk:      diskStats,
				Network:   netStats,
			}
			s.ringBuffer.Push(snapshot)

		case <-s.stopChan:
			return
		}
	}
}

// Shutdown gracefully shuts down HTTP server, stops sampling, flushes logs, and closes SQLite.
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("[Sugi] Initiating graceful shutdown...")
	close(s.stopChan)

	// Shutdown HTTP listener first to stop incoming requests
	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Printf("[Sugi] Error shutting down HTTP server: %v", err)
	}

	// Flush and close log writer
	if err := s.logWriter.Close(ctx); err != nil {
		log.Printf("[Sugi] Error closing async log writer: %v", err)
	}

	// Close SQLite database
	if err := s.sqlite.Close(); err != nil {
		log.Printf("[Sugi] Error closing SQLite storage: %v", err)
	}

	log.Println("[Sugi] Shutdown complete.")
	return nil
}
