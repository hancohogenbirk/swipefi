package api

import (
	"io/fs"
	"net/http"
	"os"
	"strings"
)

// ServeFrontend returns a handler that serves the embedded frontend files,
// falling back to index.html for SPA client-side routing.
func ServeFrontend(distFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't serve frontend for API, stream, or ws paths
		path := r.URL.Path
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/stream/") || path == "/ws" {
			http.NotFound(w, r)
			return
		}

		// Try to serve the file directly
		f, err := distFS.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for any unmatched path
		if !os.IsNotExist(err) && !strings.Contains(err.Error(), "not found") {
			fileServer.ServeHTTP(w, r)
			return
		}

		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
