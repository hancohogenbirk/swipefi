package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"

	"swipefi/internal/library"
	"swipefi/internal/store"
)

func (a *API) GetTrackArt(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	track, err := a.store.GetTrack(r.Context(), id)
	if errors.Is(err, store.ErrTrackNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	musicDir := a.scanner.MusicDir()
	if musicDir == "" {
		http.NotFound(w, r)
		return
	}

	dataDir := os.Getenv("SWIPEFI_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	cacheDir := filepath.Join(dataDir, "art")

	// Check cache first
	cached, mime, err := readCachedArt(cacheDir, id)
	if err == nil && cached != nil {
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(cached)
		return
	}

	// Extract from file
	fullPath := filepath.Join(musicDir, filepath.FromSlash(track.Path))
	art, err := library.ExtractArt(fullPath)
	if err != nil || art == nil {
		http.NotFound(w, r)
		return
	}

	// Cache to disk
	if err := cacheArt(cacheDir, id, art.Data, art.MimeType); err != nil {
		slog.Warn("cache art failed", "track_id", id, "err", err)
	}

	w.Header().Set("Content-Type", art.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(art.Data)
}

func readCachedArt(cacheDir string, trackID int64) ([]byte, string, error) {
	// Try jpeg first, then png
	for _, ext := range []struct {
		suffix string
		mime   string
	}{
		{".jpg", "image/jpeg"},
		{".png", "image/png"},
	} {
		path := filepath.Join(cacheDir, fmt.Sprintf("%d%s", trackID, ext.suffix))
		data, err := os.ReadFile(path)
		if err == nil {
			return data, ext.mime, nil
		}
	}
	return nil, "", os.ErrNotExist
}

func cacheArt(cacheDir string, trackID int64, data []byte, mimeType string) error {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	ext := ".jpg"
	if mimeType == "image/png" {
		ext = ".png"
	}

	path := filepath.Join(cacheDir, fmt.Sprintf("%d%s", trackID, ext))
	return os.WriteFile(path, data, 0644)
}
