package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/server"
	"github.com/FahmiYoshikage/sugi/internal/storage"
)

const version = "0.2.0"

func main() {
	portFlag := flag.Int("port", 8080, "HTTP server port")
	dbFlag := flag.String("db", "sugi.db", "SQLite database file path")
	retentionFlag := flag.String("retention", "7d", "Log retention period (e.g. 24h, 7d, 30d)")
	intervalFlag := flag.Duration("interval", 1*time.Second, "System metric sampling interval")
	backupDirFlag := flag.String("backup-dir", "backups", "Directory for automated SQLite backups")
	backupIntervalFlag := flag.Duration("backup-interval", 0, "Automated backup interval (e.g. 24h, 0 to disable)")
	backupNowFlag := flag.String("backup-to", "", "Perform an immediate point-in-time backup to specified file and exit")
	restoreFromFlag := flag.String("restore-from", "", "Restore SQLite database from backup file into -db and exit")
	autoSyslogFlag := flag.Bool("auto-syslog", true, "Auto-harvest Linux host syslog (/var/log/syslog, /var/log/auth.log, /var/log/messages)")
	watchLogsFlag := flag.String("watch-logs", "", "Comma-separated list of log file paths to actively tail")
	versionFlag := flag.Bool("version", false, "Print version and exit")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("Sugi Observability Engine v%s\n", version)
		os.Exit(0)
	}

	// Handle immediate restore command
	if *restoreFromFlag != "" {
		fmt.Printf("Restoring database from %s to %s...\n", *restoreFromFlag, *dbFlag)
		if err := storage.RestoreDatabase(*restoreFromFlag, *dbFlag); err != nil {
			log.Fatalf("[FATAL] Restore failed: %v", err)
		}
		fmt.Println("Database restored successfully.")
		os.Exit(0)
	}

	// Handle immediate backup snapshot command
	if *backupNowFlag != "" {
		fmt.Printf("Creating snapshot backup of %s into %s...\n", *dbFlag, *backupNowFlag)
		s, err := storage.NewSQLiteStorage(*dbFlag)
		if err != nil {
			log.Fatalf("[FATAL] Failed to open database: %v", err)
		}
		if err := s.Backup(*backupNowFlag); err != nil {
			log.Fatalf("[FATAL] Backup failed: %v", err)
		}
		_ = s.Close()
		fmt.Println("Snapshot backup created successfully.")
		os.Exit(0)
	}

	retention := parseRetention(*retentionFlag)

	cfg := server.Config{
		Port:           *portFlag,
		DBPath:         *dbFlag,
		Retention:      retention,
		SampleInterval: *intervalFlag,
		BackupInterval: *backupIntervalFlag,
		BackupDir:      *backupDirFlag,
		Version:        version,
		AutoSyslog:     *autoSyslogFlag,
		WatchLogs:      *watchLogsFlag,
	}

	printBanner(cfg)

	srv, err := server.NewServer(cfg)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize Sugi server: %v", err)
	}

	// Trap termination signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("[FATAL] Server exited with error: %v", err)
		}
	}()

	sig := <-sigChan
	log.Printf("[Sugi] Received shutdown signal (%s)", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Graceful shutdown error: %v", err)
	}
}

func parseRetention(s string) time.Duration {
	if s == "" {
		return 7 * 24 * time.Hour
	}
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil && days > 0 {
			return time.Duration(days) * 24 * time.Hour
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Printf("[WARN] Invalid retention %q, defaulting to 7 days", s)
		return 7 * 24 * time.Hour
	}
	return d
}

func printBanner(cfg server.Config) {
	banner := `
  ____             _ 
 / ___| _   _  __ _(_)
 \___ \| | | |/ _` + "`" + ` | |
  ___) | |_| | (_| | |
 |____/ \__,_|\__, |_|
              |___/   v` + cfg.Version + `
`
	fmt.Println(banner)
	fmt.Println("  Zero-dependency, Ultra-lightweight Observability Engine")
	fmt.Printf("  -> HTTP Port       : :%d\n", cfg.Port)
	fmt.Printf("  -> SQLite Database : %s (WAL Mode)\n", cfg.DBPath)
	fmt.Printf("  -> Log Retention   : %s\n", cfg.Retention)
	fmt.Printf("  -> Sampling Rate   : %s\n", cfg.SampleInterval)
	if cfg.AutoSyslog {
		fmt.Println("  -> Host Log Auto   : Active (monitoring /var/log/syslog)")
	}
	if cfg.WatchLogs != "" {
		fmt.Printf("  -> Watched Logs    : %s\n", cfg.WatchLogs)
	}
	if cfg.BackupInterval > 0 {
		fmt.Printf("  -> Auto Backup     : every %s to %s\n", cfg.BackupInterval, cfg.BackupDir)
	}
	fmt.Println("")
}
