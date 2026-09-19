package ui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed public/*
var publicFS embed.FS

// Handler returns an http.Handler serving the embedded frontend assets with custom 404 fallback.
func Handler() http.Handler {
	sub, err := fs.Sub(publicFS, "public")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Check if the requested file exists in embedded FS
		f, err := sub.Open(path)
		if err != nil {
			// Serve custom 404 page with StatusNotFound code
			notFoundFile, err404 := sub.Open("404.html")
			if err404 == nil {
				defer notFoundFile.Close()
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.Copy(w, notFoundFile)
				return
			}
			http.NotFound(w, r)
			return
		}
		_ = f.Close()

		fileServer.ServeHTTP(w, r)
	})
}
