package library

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"swipefi/internal/store"
)

type ScanStatus struct {
	Scanning bool `json:"scanning"`
	Scanned  int  `json:"scanned"`
	Total    int  `json:"total"` // estimated from previous scan or directory walk
}

type Scanner struct {
	musicDir string
	store    *store.Store
	status   ScanStatus
}

func NewScanner(musicDir string, s *store.Store) *Scanner {
	return &Scanner{musicDir: musicDir, store: s}
}

func (sc *Scanner) SetMusicDir(dir string) {
	sc.musicDir = dir
}

func (sc *Scanner) MusicDir() string {
	return sc.musicDir
}

func (sc *Scanner) GetStatus() ScanStatus {
	return sc.status
}

func (sc *Scanner) Scan(ctx context.Context) (int, error) {
	slog.Info("starting library scan", "dir", sc.musicDir)

	// Quick count of audio files for progress estimation
	total := 0
	filepath.WalkDir(sc.musicDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "to_delete" || name == "@eaDir" || name == "#recycle" ||
				strings.HasPrefix(name, "@") || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if IsAudioFile(d.Name()) {
			total++
		}
		return nil
	})

	sc.status = ScanStatus{Scanning: true, Scanned: 0, Total: total}
	slog.Info("scan: counted files", "total", total)

	existingPaths := make(map[string]bool)
	var count int

	err := filepath.WalkDir(sc.musicDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("walk error", "path", path, "err", err)
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() {
			name := d.Name()
			// Skip Synology system dirs, recycle bins, hidden dirs, and to_delete
			if name == "to_delete" || name == "@eaDir" || name == "#recycle" ||
				strings.HasPrefix(name, "@") || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !IsAudioFile(d.Name()) {
			return nil
		}

		relPath, err := filepath.Rel(sc.musicDir, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)
		existingPaths[relPath] = true

		meta, err := ReadMetadata(path, relPath)
		if err != nil {
			slog.Warn("metadata error", "path", relPath, "err", err)
			return nil
		}

		track := &store.Track{
			Path:       meta.Path,
			Title:      meta.Title,
			Artist:     meta.Artist,
			Album:      meta.Album,
			DurationMs: meta.DurationMs,
			Format:     meta.Format,
			AddedAt:    meta.AddedAt,
		}

		if err := sc.store.UpsertTrack(ctx, track); err != nil {
			slog.Warn("upsert error", "path", relPath, "err", err)
			return nil
		}

		count++
		sc.status.Scanned = count
		if count%100 == 0 {
			slog.Info("scan progress", "tracks", count, "total", total)
		}

		return nil
	})

	sc.status.Scanning = false
	sc.status.Scanned = count

	if err != nil {
		return count, err
	}

	// Mark tracks whose files no longer exist on disk
	orphaned, err := sc.store.MarkMissingAsDeleted(ctx, existingPaths)
	if err != nil {
		slog.Warn("cleanup orphaned tracks failed", "err", err)
	} else if orphaned > 0 {
		slog.Info("marked orphaned tracks as deleted", "count", orphaned)
	}

	slog.Info("library scan complete", "tracks_found", count, "orphaned", orphaned)
	return count, nil
}

func (sc *Scanner) ListFolders(path string) ([]FolderEntry, error) {
	dir := sc.musicDir
	if path != "" {
		dir = filepath.Join(sc.musicDir, filepath.FromSlash(path))
	}

	// Prevent directory traversal
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	absMusic, err := filepath.Abs(sc.musicDir)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(absDir, absMusic) {
		return nil, os.ErrPermission
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var folders []FolderEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "to_delete" || name == "#recycle" ||
			strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@") {
			continue
		}

		folderPath := filepath.ToSlash(filepath.Join(path, name))

		// Only include folders that have tracks (recursively) in the DB
		if sc.store.HasTracksInFolder(folderPath) {
			folders = append(folders, FolderEntry{
				Name: name,
				Path: folderPath,
			})
		}
	}

	return folders, nil
}

type FolderEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}
