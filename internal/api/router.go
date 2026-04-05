package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"swipefi/internal/dlna"
	"swipefi/internal/library"
	"swipefi/internal/player"
	"swipefi/internal/store"
)

type API struct {
	store     *store.Store
	scanner   *library.Scanner
	player    *player.Player
	discovery *dlna.Discovery
	hub       *Hub
}

func NewAPI(s *store.Store, scanner *library.Scanner, p *player.Player, d *dlna.Discovery, hub *Hub) *API {
	return &API{store: s, scanner: scanner, player: p, discovery: d, hub: hub}
}

func NewRouter(api *API, musicDir string) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", api.Health)

		// Library
		r.Get("/folders", api.ListFolders)
		r.Get("/tracks", api.ListTracks)
		r.Get("/tracks/{id}", api.GetTrack)
		r.Post("/library/scan", api.ScanLibrary)

		// Player
		r.Post("/player/play", api.PlayerPlay)
		r.Post("/player/pause", api.PlayerPause)
		r.Post("/player/resume", api.PlayerResume)
		r.Post("/player/next", api.PlayerNext)
		r.Post("/player/prev", api.PlayerPrev)
		r.Post("/player/seek", api.PlayerSeek)
		r.Post("/player/reject", api.PlayerReject)
		r.Get("/player/state", api.PlayerState)

		// Devices
		r.Get("/devices", api.ListDevices)
		r.Post("/devices/select", api.SelectDevice)
		r.Post("/devices/scan", api.RescanDevices)
	})

	// WebSocket
	r.Get("/ws", api.hub.HandleWS)

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
