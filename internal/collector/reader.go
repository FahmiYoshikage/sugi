package collector

import (
	"io"
	"os"
	"path/filepath"
)

// ProcFS defines an interface to read pseudo-filesystem files.
type ProcFS interface {
	Open(name string) (io.ReadCloser, error)
}

// DefaultProcFS reads directly from the host filesystem (/proc).
type DefaultProcFS struct {
	BaseDir string
}

// NewDefaultProcFS creates a ProcFS rooted at /proc or a custom base directory.
func NewDefaultProcFS(baseDir ...string) *DefaultProcFS {
	base := "/proc"
	if len(baseDir) > 0 && baseDir[0] != "" {
		base = baseDir[0]
	}
	return &DefaultProcFS{BaseDir: base}
}

// Open opens a file relative to the procfs base directory.
func (p *DefaultProcFS) Open(name string) (io.ReadCloser, error) {
	fullPath := filepath.Join(p.BaseDir, name)
	return os.Open(fullPath)
}
