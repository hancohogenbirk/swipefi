package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"swipefi/internal/library"

	"swipefi/internal/store"
)

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	count, err := a.store.TrackCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"tracks": count,
	})
}

func (a *API) ListFolders(w http.ResponseWriter, r *http.Request) {
	// During initial scan (DB was empty), return empty to avoid showing partial results
	if a.scanner.IsInitialScan() {
		writeJSON(w, http.StatusOK, []library.FolderEntry{})
		return
	}

	path := r.URL.Query().Get("path")

	folders, err := a.scanner.ListFolders(path)
	if err != nil {
		slog.Error("list folders", "path", path, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list folders")
		return
	}

	if folders == nil {
		folders = []library.FolderEntry{}
	}

	writeJSON(w, http.StatusOK, folders)
}

func (a *API) ListTracks(w http.ResponseWriter, r *http.Request) {
	folder := r.URL.Query().Get("folder")
	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")
	direct := r.URL.Query().Get("direct") == "true"

	var tracks []store.Track
	var err error
	if direct {
		tracks, err = a.store.ListTracksDirectOnly(r.Context(), folder, sort, order)
	} else {
		tracks, err = a.store.ListTracks(r.Context(), folder, sort, order)
	}
	if err != nil {
		slog.Error("list tracks", "folder", folder, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list tracks")
		return
	}

	if tracks == nil {
		tracks = []store.Track{}
	}

	writeJSON(w, http.StatusOK, tracks)
}

func (a *API) GetTrack(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid track id")
		return
	}

	track, err := a.store.GetTrack(r.Context(), id)
	if errors.Is(err, store.ErrTrackNotFound) {
		writeError(w, http.StatusNotFound, "track not found")
		return
	}
	if err != nil {
		slog.Error("get track", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to get track")
		return
	}

	writeJSON(w, http.StatusOK, track)
}

func (a *API) ScanStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.scanner.GetStatus())
}

func (a *API) ScanLibrary(w http.ResponseWriter, r *http.Request) {
	count, err := a.scanner.Scan(r.Context())
	if err != nil {
		slog.Error("scan library", "err", err)
		writeError(w, http.StatusInternalServerError, "scan failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"tracks": count,
	})
}
