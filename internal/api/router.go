package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"swipefi/internal/library"
	"swipefi/internal/store"
)

type API struct {
	store   *store.Store
	scanner *library.Scanner
}

func NewAPI(s *store.Store, scanner *library.Scanner) *API {
	return &API{store: s, scanner: scanner}
}

func NewRouter(api *API, musicDir string) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", api.Health)
		r.Get("/folders", api.ListFolders)
		r.Get("/tracks", api.ListTracks)
		r.Get("/tracks/{id}", api.GetTrack)
		r.Post("/library/scan", api.ScanLibrary)
	})

	// Raw audio file streaming
	fileServer := http.StripPrefix("/stream/", http.FileServer(http.Dir(musicDir)))
	r.Handle("/stream/*", fileServer)

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
