package http

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// The production Docker build copies the real Vite output (web/dist) into
// dist/ before compiling; the committed placeholder keeps plain `go build`
// working without a Node toolchain (design D1 — embed path is prod/CI-only).
//
//go:embed all:dist
var distFS embed.FS

// spaFallback serves the embedded SPA: real files when they exist, otherwise
// index.html so client-side routes resolve. API paths stay plain 404s.
func spaFallback() http.HandlerFunc {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("embedded dist directory missing: " + err.Error())
	}

	fileServer := http.FileServerFS(dist)

	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			http.NotFound(w, r)
			return
		}

		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}

		if _, err := fs.Stat(dist, name); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		http.ServeFileFS(w, r, dist, "index.html")
	}
}
