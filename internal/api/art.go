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

const (
	artCacheSubdir = "art"
	artCacheMaxAge = 86400 // 24 hours in seconds
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

	cacheDir := filepath.Join(a.dataDir, artCacheSubdir)

	// Check cache first
	cached, mime, err := readCachedArt(cacheDir, id)
	if err == nil && cached != nil {
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", artCacheMaxAge))
		w.Write(cached)
		return
	}

	// Check if we already know there's no art (avoid re-scraping)
	if hasNoArtMarker(cacheDir, id) {
		http.NotFound(w, r)
		return
	}

	// Try extracting embedded art from the file
	fullPath := filepath.Join(musicDir, filepath.FromSlash(track.Path))
	art, err := library.ExtractArt(fullPath)

	// If no embedded art, try scraping from MusicBrainz/Cover Art Archive
	if (err != nil || art == nil) && track.Artist != "" && track.Album != "" {
		slog.Info("no embedded art, trying MusicBrainz", "artist", track.Artist, "album", track.Album)
		scraped, scrapErr := library.FetchCoverArt(track.Artist, track.Album)
		if scrapErr != nil {
			slog.Warn("cover art scrape failed", "err", scrapErr)
		} else if scraped != nil {
			art = scraped
		}
	}

	if art == nil {
		// Cache a "no art" marker so we don't re-scrape every request
		cacheNoArt(cacheDir, id)
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

func hasNoArtMarker(cacheDir string, trackID int64) bool {
	path := filepath.Join(cacheDir, fmt.Sprintf("%d.noart", trackID))
	_, err := os.Stat(path)
	return err == nil
}

func cacheNoArt(cacheDir string, trackID int64) {
	os.MkdirAll(cacheDir, 0755)
	path := filepath.Join(cacheDir, fmt.Sprintf("%d.noart", trackID))
	os.WriteFile(path, []byte("no art"), 0644)
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

// DeleteCachedArt removes all cached art files for the given track ID.
func DeleteCachedArt(cacheDir string, trackID int64) {
	for _, ext := range []string{".jpg", ".png", ".noart"} {
		path := filepath.Join(cacheDir, fmt.Sprintf("%d%s", trackID, ext))
		os.Remove(path)
	}
}
