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
- [x] **Tahap 2: I/O Collectors & Time Series In-Memory Storage**
  - Parsing `/proc/diskstats` (throughputs KB/s, IOPS, device metrics)
  - Parsing `/proc/net/dev` (Ingress/Egress KB/s, packet rates, interfaces)
  - In-memory circular ring buffer (1 jam metrik time-series, zero-allocation write path: 37ns/op, 0 B/op)
- [x] **Tahap 3: Persistent Storage & Retention Engine**
  - Pure-Go SQLite engine (`modernc.org/sqlite`) dengan mode WAL aktif (`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`)
  - Asynchronous batch log writer (`AsyncLogWriter`) dengan bounded channel & zero-blocking
  - Background ticker untuk automatic retention pruning (misal penghapusan log > 7 hari)
- [x] **Tahap 4: Ingestion Pipeline & Core Orchestration**
  - HTTP Ingestion API `POST /api/v1/logs` (mendukung JSON array/object & raw line text)
  - HTTP Query API `GET /api/v1/logs` dengan parameter filter (`level`, `service`, `search`, `limit`)
  - HTTP Metrics API `GET /api/v1/metrics` & `GET /api/v1/metrics/history`
  - CLI flags & environment configuration (`-port`, `-db`, `-retention`, `-interval`)
  - Server orchestrator dengan graceful shutdown (`SIGINT`, `SIGTERM`)
  - Terverifikasi efisiensi resource: **RAM ~16.7 MB** (target < 30 MB) & **CPU ~0.3%** (target < 1%)
- [x] **Tahap 5: Real-Time SSE Broadcaster & Embedded Web UI**
  - Endpoint Server-Sent Events `GET /api/v1/stream` untuk streaming metrik 1-detik ke browser
  - Web dashboard dark-mode responsif disatukan ke biner executable melalui `//go:embed`
  - Engine charting canvas murni ultra-ringan (~200 baris vanilla JS, zero npm, zero CDN eksternal)
  - Log explorer interaktif dan form uji coba ingesti log dengan proteksi honeypot anti-spam
  - Halaman Custom 404 dan favicon SVG
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
