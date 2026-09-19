# Sugi

> **Zero-dependency, ultra-lightweight single-binary observability engine.**

Sugi adalah server metrik dan agregasi log mandiri (*standalone*) berbasis Go murni (*CGO-free*) dengan dashboard visual bawaan yang di-embed langsung ke dalam satu biner executable `./sugi`.

---

## Filosofi & Karakteristik Utama

1. **Single Binary & Zero Runtime Dependencies**: Seluruh aplikasi (backend engine, embedded database, dan frontend web dashboard) terkompilasi menjadi satu file biner `./sugi` tanpa kebutuhan runtime Node.js, Docker daemon, atau external database.
2. **Pure Go / CGO-Free**: Mendukung cross-compilation instan untuk arsitektur Linux AMD64 dan ARM64 tanpa toolchain C/GCC.
3. **Ultra-Low Resource Footprint**: Target pemakaian RAM di bawah **30 MB** saat idle dan CPU **< 1%**.
4. **Native Linux Kernel Metrics**: Mengumpulkan metrik sistem (CPU, RAM, Disk, Jaringan) langsung dari pseudo-filesystem `/proc` Linux tanpa library eksternal yang berat.
5. **Real-Time Streaming**: Disiarkan secara instan ke antarmuka dashboard menggunakan Server-Sent Events (SSE) via `GET /api/v1/stream`.
6. **Embedded Dark-Mode UI**: Dashboard web modern berbasis pure HTML5, vanilla CSS, dan canvas charting engine super ringan tanpa dependensi npm / bundler.

---

## Status Roadmap Proyek

- [x] **Tahap 1: Foundation & Core Collectors**
  - Standard Go Project Layout
  - Pembacaan & parsing native `/proc/stat` (CPU delta total, breakdown, dan per-core)
  - Pembacaan & parsing native `/proc/meminfo` (RAM presisi dan legacy fallback)
  - Unit tests & benchmarks dengan mock fixtures
- [ ] **Tahap 2: I/O Collectors & Time Series In-Memory Storage**
  - Parsing `/proc/diskstats` dan `/proc/net/dev`
  - In-memory circular ring buffer (1 jam metrik time-series)
- [ ] **Tahap 3: Persistent Storage & Retention Engine**
  - Pure-Go SQLite engine (`modernc.org/sqlite`) dengan WAL mode
  - Batch log writer & auto-pruning background worker
- [ ] **Tahap 4: Ingestion Pipeline & Core Orchestration**
  - `POST /api/v1/logs` dengan bounded channel buffer & backpressure policy
  - Graceful shutdown & CLI configuration flags
- [ ] **Tahap 5: Real-Time SSE Broadcaster & Embedded Web UI**
  - Endpoint `GET /api/v1/stream`
  - Dark-mode responsive dashboard via `//go:embed`
- [ ] **Tahap 6: Hardening, Benchmarking & Release**
  - Profiling pprof RAM & CPU
  - Multi-arch cross-compilation release (Linux AMD64/ARM64)
  - Production SEO & landing documentation checklist

---

## Struktur Direktori

```
sugi/
├── cmd/
│   └── sugi/
│       └── main.go             # Entrypoint aplikasi Sugi
├── internal/
│   ├── collector/              # Native Linux kernel /proc collectors
│   │   ├── cpu.go              # Parser /proc/stat & kalkulasi delta beban CPU
│   │   ├── cpu_test.go         # Unit test & benchmark CPU parser
│   │   ├── mem.go              # Parser /proc/meminfo
│   │   ├── mem_test.go         # Unit test & benchmark memory parser
│   │   └── reader.go           # Abstraksi pembaca procfs
│   ├── model/                  # Data structures (metrik CPU, Mem, System snapshot)
│   │   └── metrics.go
│   ├── storage/                # Ring buffer & SQLite engine (Tahap 2 & 3)
│   ├── api/                    # Ingestion & SSE HTTP handlers (Tahap 4 & 5)
│   └── ui/                     # Embedded Web dashboard assets (Tahap 5)
├── go.mod                      # Go module definition
└── README.md
```

---

## Menjalankan Unit Test & Benchmark

```bash
# Jalankan seluruh unit test dengan race detector aktif
go test -v -race ./...

# Jalankan benchmark alokasi memori
go test -bench=. -benchmem ./internal/collector/...

# Uji coba live collector pada mesin Linux lokal
go run ./cmd/sugi/main.go
```

---

## Lisensi

Didistribusikan secara gratis dan terbuka di bawah lisensi [MIT](LICENSE).
