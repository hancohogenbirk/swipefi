package api

import (
	"context"
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
	scanStatus := a.scanner.GetStatus()
	azStatus := a.analyzer.GetStatus()
	writeJSON(w, http.StatusOK, map[string]any{
		"scanning":             scanStatus.Scanning,
		"scanned":              scanStatus.Scanned,
		"total":                scanStatus.Total,
		"phase":                scanStatus.Phase,
		"analyzing":            azStatus.Running,
		"analyzed":             azStatus.Analyzed,
		"analysis_total":       azStatus.Total,
	})
}

func (a *API) ScanLibrary(w http.ResponseWriter, r *http.Request) {
	count, err := a.scanner.Scan(r.Context(), false)
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

func (a *API) RescanLibrary(w http.ResponseWriter, r *http.Request) {
	if a.scanner.GetStatus().Scanning {
		writeError(w, http.StatusConflict, "scan already in progress")
		return
	}

	a.scanner.MarkScanning()
	go func() {
		count, err := a.scanner.Scan(context.Background(), true)
		if err != nil {
			slog.Error("force rescan failed", "err", err)
			return
		}
		slog.Info("force rescan complete", "tracks", count)

		// Run flacalyzer analysis if enabled (reset scores so all tracks get re-analyzed)
		enabled, _ := a.store.GetConfig(store.ConfigKeyFlacalyzerEnabled)
		if enabled == "true" && a.analyzer.Available() {
			musicDir := a.scanner.MusicDir()
			if err := a.store.ResetTranscodeScores(context.Background(), musicDir); err != nil {
				slog.Error("reset transcode scores", "err", err)
			}
			if err := a.analyzer.Run(context.Background(), musicDir); err != nil {
				slog.Error("transcode analysis after rescan", "err", err)
			}
		}
	}()

	writeJSON(w, http.StatusOK, map[string]string{"status": "scanning"})
}
