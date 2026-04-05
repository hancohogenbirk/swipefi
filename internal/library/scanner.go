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

type Scanner struct {
	musicDir string
	store    *store.Store
}

func NewScanner(musicDir string, s *store.Store) *Scanner {
	return &Scanner{musicDir: musicDir, store: s}
}

func (sc *Scanner) Scan(ctx context.Context) (int, error) {
	slog.Info("starting library scan", "dir", sc.musicDir)

	var count int
	err := filepath.WalkDir(sc.musicDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("walk error", "path", path, "err", err)
			return nil // skip errors, continue scanning
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() {
			// Skip the to_delete directory
			if d.Name() == "to_delete" {
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
		// Normalize to forward slashes for consistent storage
		relPath = filepath.ToSlash(relPath)

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
		if count%100 == 0 {
			slog.Info("scan progress", "tracks", count)
		}

		return nil
	})

	if err != nil {
		return count, err
	}

	slog.Info("library scan complete", "tracks", count)
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
		if e.Name() == "to_delete" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		folders = append(folders, FolderEntry{
			Name: e.Name(),
			Path: filepath.ToSlash(filepath.Join(path, e.Name())),
		})
	}

	return folders, nil
}

type FolderEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}
