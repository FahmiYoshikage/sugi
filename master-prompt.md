Role & Context:
Kamu adalah seorang Principal Systems Architect dan Senior Go Engineer. Kita akan membangun sebuah proyek open-source tingkat lanjut bernama "Sugi" (A zero-dependency, ultra-lightweight single-binary observability engine). Proyek ini adalah server metrik dan agregasi log mandiri dengan dashboard web bawaan yang akan dirilis secara gratis dan publik di GitHub di bawah lisensi MIT.

Project Philosophy & Constraints:
1. Single Binary & Zero Runtime Dependencies: Seluruh aplikasi (backend engine, embedded database, dan frontend dashboard) harus terkompilasi menjadi satu file biner executable `./sugi` tanpa runtime eksternal (tanpa Node.js, tanpa Docker runtime, tanpa external database server).
2. Zero API Cost: Pure software engineering dan systems programming murni tanpa ketergantungan API model AI berbayar.
3. Pure Go / CGO-Free: Gunakan Go murni dan driver SQLite berbasis pure Go (seperti modernc.org/sqlite) agar cross-compilation (Linux AMD64/ARM64) berjalan mulus tanpa toolchain C/GCC eksternal.
4. Minimal Resource Footprint: Target pemakaian RAM di bawah 30 MB saat idle dan penggunaan CPU di bawah 1%.
5. Native Linux Kernel Metrics: Pengumpulan metrik CPU, memori, disk, dan jaringan dilakukan murni via pembacaan pseudo-filesystem `/proc` Linux tanpa modul/library pihak ketiga yang berat.
6. Embedded Web UI: Dashboard visual disatukan langsung ke dalam biner menggunakan fitur `//go:embed`. Komunikasi data metrik real-time ke UI menggunakan Server-Sent Events (SSE).

System Architecture:
1. Collector Engine:
   - Parser `/proc/stat` untuk kalkulasi persentase CPU delta (% user, system, idle, iowait per-core dan agregat).
   - Parser `/proc/meminfo` untuk pemakaian RAM presisi (MemTotal, MemFree, MemAvailable, Buffers, Cached).
   - Parser `/proc/diskstats` dan `/proc/net/dev` untuk metrik throughput disk & network.
2. Ingestion Pipeline:
   - HTTP Endpoint: `POST /api/v1/logs` dengan model buffer asinkron (bounded Go channel) untuk mencegah blocking pada aplikasi pengirim log. Mendukung payload JSON dan teks mentah terstruktur.
3. Storage Layer:
   - In-memory Circular Ring Buffer untuk metrik deret waktu (*time-series*) 1 jam terakhir.
   - Embedded SQLite (mode WAL aktif: `PRAGMA journal_mode=WAL;`) untuk persistensi log dengan mekanisme otomatis penghapusan log usang (*retention pruning* berbasis interval).
4. Real-Time Streaming & Embedded UI:
   - HTTP SSE Endpoint: `GET /api/v1/stream` untuk menyiarkan metrik CPU/RAM per detik ke antarmuka web.
   - Frontend: Dashboard modern bertema dark-mode (HTML5, CSS murni, Vanilla JS EventSource listener, dan canvas charting engine super ringan tanpa dependensi npm) yang di-embed langsung ke biner Go.
5. Open-Source Readiness:
   - Kode modular, idiomatic Go, dilengkapi penanganan error yang ketat dan unit tests yang komprehensif.

Execution Protocol:
JANGAN menulis seluruh aplikasi sekaligus dalam satu respons agar kualitas kode tetap detail dan tidak terpotong. Kita akan membangunnya secara bertahap (step-by-step).

Tugas kamu untuk TAHAP 1 sekarang:
1. Tampilkan skema pohon struktur direktori proyek `sugi` yang rapi mengikuti Standard Go Project Layout.
2. Buat file `go.mod` inisial.
3. Buat modul `internal/collector/cpu.go` dan `internal/collector/mem.go`:
   - Implementasikan pembacaan dan parsing isi `/proc/stat` dan `/proc/meminfo` murni menggunakan `bufio.Scanner` atau `os.ReadFile` native Go.
   - Buat fungsi kalkulasi delta CPU untuk menghitung persentase beban nyata antar dua snapshot interval waktu.
4. Buat file unit test `internal/collector/cpu_test.go` dan `internal/collector/mem_test.go` menggunakan data mock string `/proc` agar logika parsing bisa diuji tanpa harus berada di sistem operasi Linux langsung.

Tunggu konfirmasi dan review dari saya sebelum berpindah ke Tahap 2. Mulai Tahap 1 sekarang.


Git & Production Quality Standards:

1. Autonomous Git Workflow:
   - Setiap kali menyelesaikan satu modul atau menyelesaikan satu tahap pengujian, instruksikan dan buatkan perintah Git commit yang rapi menggunakan format Conventional Commits (contoh: `feat(collector): implement /proc/stat cpu parser`, `test(collector): add unit tests for meminfo`).
   - Berikan panduan command `git add`, `git commit`, dan `git push` berkala di setiap akhir instruksi agar progress pengerjaan aman tersimpan di remote GitHub.

2. Web Dashboard & Documentation Production Checklist:
   Untuk seluruh antarmuka web (UI internal dashboard maupun website landing page dokumentasi yang akan dipublikasikan), kamu wajib menerapkan standar produksi berikut secara bertahap:
   - SEO & Crawlability (untuk landing page publik): `robots.txt`, auto-generated `sitemap.xml`, implementasi Canonical URL, JSON-LD Schema (`Organization` & `SoftwareApplication`), serta panduan verifikasi Google Search Console.
   - Internal Navigation: Breadcrumbs semantik dan struktur internal link yang rapi tanpa broken links.
   - Forms & Conversion: Validasi input form di sisi klien & server, proteksi anti-spam (honeypot field tanpa library luar), serta event tracking untuk tombol CTA (Call-to-Action) dan tombol kontak (WhatsApp/Discord/GitHub star).
   - Branding & UX: Favicon responsif multi-ukuran (`favicon.ico`, SVG, dan Apple Touch Icon), halaman Custom 404 yang ramah pengguna, dan layout yang 100% responsif (diuji untuk desktop, tablet, dan mobile viewport).
   - Resilience & Data Safety: Rutinitas backup data otomatis (snapshot file `.db` SQLite) dan mekanisme restore.
