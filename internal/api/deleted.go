package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"swipefi/internal/library"
	"swipefi/internal/store"
)

func (a *API) ListDeleted(w http.ResponseWriter, r *http.Request) {
	tracks, err := a.store.ListDeleted(r.Context())
	if err != nil {
		slog.Error("list deleted", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list deleted tracks")
		return
	}
	if tracks == nil {
		tracks = []store.Track{}
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (a *API) RestoreDeleted(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	musicDir := a.scanner.MusicDir()
	if musicDir == "" {
		writeError(w, http.StatusBadRequest, "music directory not configured")
		return
	}
	deleteDir := filepath.Join(musicDir, "to_delete")

	restored := 0
	for _, id := range req.IDs {
		track, err := a.store.GetTrack(r.Context(), id)
		if err != nil {
			slog.Warn("restore: track not found", "id", id, "err", err)
			continue
		}

		src := filepath.Join(deleteDir, filepath.FromSlash(track.Path))
		dst := filepath.Join(musicDir, filepath.FromSlash(track.Path))

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			slog.Error("restore: mkdir", "path", dst, "err", err)
			continue
		}

		if err := os.Rename(src, dst); err != nil {
			slog.Error("restore: rename", "src", src, "dst", dst, "err", err)
			continue
		}

		if err := a.store.UnmarkDeleted(r.Context(), id); err != nil {
			slog.Error("restore: unmark", "id", id, "err", err)
			continue
		}

		library.CleanupEmptyDirs(filepath.Dir(src), deleteDir)

		slog.Info("restored track", "id", id, "path", track.Path)
		restored++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"restored": restored,
	})
}

func (a *API) PurgeDeleted(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
		All bool    `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	musicDir := a.scanner.MusicDir()
	if musicDir == "" {
		writeError(w, http.StatusBadRequest, "music directory not configured")
		return
	}
	deleteDir := filepath.Join(musicDir, "to_delete")

	dataDir := os.Getenv("SWIPEFI_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	cacheDir := filepath.Join(dataDir, "art")

	ids := req.IDs
	if req.All {
		tracks, err := a.store.ListDeleted(r.Context())
		if err != nil {
			slog.Error("purge: list deleted", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to list deleted tracks")
			return
		}
		ids = make([]int64, len(tracks))
		for i, t := range tracks {
			ids[i] = t.ID
		}
	}

	purged := 0
	for _, id := range ids {
		track, err := a.store.GetTrack(r.Context(), id)
		if err != nil {
			slog.Warn("purge: track not found", "id", id, "err", err)
			continue
		}

		deletedFilePath := filepath.Join(deleteDir, filepath.FromSlash(track.Path))
		if err := os.Remove(deletedFilePath); err != nil && !os.IsNotExist(err) {
			slog.Error("purge: remove file", "path", deletedFilePath, "err", err)
			continue
		}

		DeleteCachedArt(cacheDir, id)

		if err := a.store.PurgeTrack(r.Context(), id); err != nil {
			slog.Error("purge: db delete", "id", id, "err", err)
			continue
		}

		originalDir := filepath.Dir(filepath.Join(musicDir, filepath.FromSlash(track.Path)))
		library.CleanupEmptyDirs(originalDir, musicDir)

		library.CleanupEmptyDirs(filepath.Dir(deletedFilePath), deleteDir)

		slog.Info("purged track", "id", id, "path", track.Path)
		purged++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"purged": purged,
	})
}
