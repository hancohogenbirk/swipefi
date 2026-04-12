package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"swipefi/internal/library"
	"swipefi/internal/store"
)

type processingState struct {
	mu        sync.Mutex
	active    bool
	operation string
	total     int
	completed int
	errors    []string
}

type ProcessingStatus struct {
	Active    bool     `json:"active"`
	Operation string   `json:"operation,omitempty"`
	Total     int      `json:"total,omitempty"`
	Completed int      `json:"completed,omitempty"`
	Errors    []string `json:"errors,omitempty"`
}

func (ps *processingState) Start(op string, total int) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.active {
		return fmt.Errorf("operation already in progress")
	}
	ps.active = true
	ps.operation = op
	ps.total = total
	ps.completed = 0
	ps.errors = nil
	return nil
}

func (ps *processingState) Advance() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.completed++
}

func (ps *processingState) AddError(e string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.errors = append(ps.errors, e)
}

func (ps *processingState) Complete() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.active = false
}

func (ps *processingState) Status() ProcessingStatus {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ProcessingStatus{
		Active:    ps.active,
		Operation: ps.operation,
		Total:     ps.total,
		Completed: ps.completed,
		Errors:    append([]string(nil), ps.errors...),
	}
}

func (a *API) GetProcessingStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.processing.Status())
}

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
	if a.scanner.GetStatus().Scanning {
		writeError(w, http.StatusConflict, "library scan in progress, please wait")
		return
	}

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

	if err := a.processing.Start("restore", len(req.IDs)); err != nil {
		writeError(w, http.StatusConflict, "operation already in progress")
		return
	}

	deleteDir := library.DeleteDir(musicDir)
	ids := make([]int64, len(req.IDs))
	copy(ids, req.IDs)

	go func() {
		defer a.processing.Complete()
		ctx := context.Background()

		for _, id := range ids {
			track, err := a.store.GetTrack(ctx, id)
			if err != nil {
				slog.Warn("restore: track not found", "id", id, "err", err)
				a.processing.AddError(fmt.Sprintf("track %d not found", id))
				a.processing.Advance()
				continue
			}

			src := filepath.Join(deleteDir, filepath.FromSlash(track.Path))
			dst := filepath.Join(musicDir, filepath.FromSlash(track.Path))

			slog.Info("restore: attempting", "id", id, "src", src, "dst", dst)

			// Check if source file exists
			if _, err := os.Stat(src); err != nil {
				slog.Error("restore: source file not found", "src", src, "err", err)
				// File might not be in to_delete — just unmark in DB
				if err := a.store.UnmarkDeleted(ctx, id); err != nil {
					slog.Error("restore: unmark", "id", id, "err", err)
				}
				a.processing.AddError(fmt.Sprintf("%s: file not found in to_delete", track.Title))
				a.processing.Advance()
				continue
			}

			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				slog.Error("restore: mkdir", "path", dst, "err", err)
				a.processing.AddError(fmt.Sprintf("%s: mkdir failed", track.Title))
				a.processing.Advance()
				continue
			}

			if err := os.Rename(src, dst); err != nil {
				slog.Error("restore: rename", "src", src, "dst", dst, "err", err)
				a.processing.AddError(fmt.Sprintf("%s: move failed: %v", track.Title, err))
				a.processing.Advance()
				continue
			}

			if err := a.store.UnmarkDeleted(ctx, id); err != nil {
				slog.Error("restore: unmark", "id", id, "err", err)
				a.processing.Advance()
				continue
			}

			library.CleanupEmptyDirs(filepath.Dir(src), deleteDir)

			slog.Info("restored track", "id", id, "path", track.Path)
			a.processing.Advance()
		}

		// Trigger partial rescan of affected folders so restored tracks show in library
		rescanFolders := make(map[string]bool)
		for _, id := range ids {
			track, err := a.store.GetTrack(ctx, id)
			if err == nil && !track.Deleted {
				folder := filepath.Dir(track.Path)
				rescanFolders[folder] = true
			}
		}
		for folder := range rescanFolders {
			a.scanner.ScanFolder(ctx, folder)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]string{"status": "processing"})
}

func (a *API) PurgeDeleted(w http.ResponseWriter, r *http.Request) {
	if a.scanner.GetStatus().Scanning {
		writeError(w, http.StatusConflict, "library scan in progress, please wait")
		return
	}

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
	deleteDir := library.DeleteDir(musicDir)

	cacheDir := filepath.Join(a.dataDir, artCacheSubdir)

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

	if err := a.processing.Start("purge", len(ids)); err != nil {
		writeError(w, http.StatusConflict, "operation already in progress")
		return
	}

	idsCopy := make([]int64, len(ids))
	copy(idsCopy, ids)

	go func() {
		defer a.processing.Complete()
		ctx := context.Background()

		for _, id := range idsCopy {
			track, err := a.store.GetTrack(ctx, id)
			if err != nil {
				slog.Warn("purge: track not found", "id", id, "err", err)
				a.processing.Advance()
				continue
			}

			deletedFilePath := filepath.Join(deleteDir, filepath.FromSlash(track.Path))
			if err := os.Remove(deletedFilePath); err != nil && !os.IsNotExist(err) {
				slog.Error("purge: remove file", "path", deletedFilePath, "err", err)
				a.processing.AddError(fmt.Sprintf("%s: remove failed", track.Title))
				a.processing.Advance()
				continue
			}

			DeleteCachedArt(cacheDir, id)

			if err := a.store.PurgeTrack(ctx, id); err != nil {
				slog.Error("purge: db delete", "id", id, "err", err)
				a.processing.AddError(fmt.Sprintf("%s: db delete failed", track.Title))
				a.processing.Advance()
				continue
			}

			originalDir := filepath.Dir(filepath.Join(musicDir, filepath.FromSlash(track.Path)))
			library.CleanupEmptyDirs(originalDir, musicDir)

			library.CleanupEmptyDirs(filepath.Dir(deletedFilePath), deleteDir)

			slog.Info("purged track", "id", id, "path", track.Path)
			a.processing.Advance()
		}
	}()

	writeJSON(w, http.StatusOK, map[string]string{"status": "processing"})
}
