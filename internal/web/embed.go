// Package web embeds the built React SPA into the Go binary and serves it with
// a history-fallback so client-side routes (e.g. /app, /login) resolve to
// index.html.
//
// NOTE on the embed root: Go's //go:embed cannot reference parent directories
// (no "..") and resolves patterns relative to THIS source file's directory.
// Vite therefore builds into internal/web/dist (configured via vite.config.ts
// outDir) so the assets sit next to this file and can be embedded.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded SPA. Existing static
// assets are served directly; everything else returns index.html (SPA history
// fallback). API routing is handled upstream — this handler is mounted only for
// non-/api paths.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		cleaned := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleaned == "" {
			serveIndex(w, sub)
			return
		}
		if _, statErr := fs.Stat(sub, cleaned); statErr == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, sub)
	}), nil
}

func serveIndex(w http.ResponseWriter, sub fs.FS) {
	data, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.Error(w, "SPA not built", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
