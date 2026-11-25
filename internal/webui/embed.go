package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist/*
var distFS embed.FS

// Assets returns the embedded web/dist filesystem (rooted at dist).
func Assets() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// HasBuiltUI reports whether Vite built index.html is present inside embedded assets.
func HasBuiltUI() bool {
	sub, err := Assets()
	if err != nil {
		return false
	}
	f, err := sub.Open("index.html")
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// FileServer serves static assets from embedded dist/ (or local override dir if provided).
func FileServer(overrideDir string) http.Handler {
	if overrideDir != "" {
		return http.FileServer(http.Dir(overrideDir))
	}
	sub, err := Assets()
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}
