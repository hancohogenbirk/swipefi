package library

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"swipefi/internal/store"
)

type ScanStatus struct {
	Scanning bool `json:"scanning"`
	Scanned  int  `json:"scanned"`
	Total    int  `json:"total"` // estimated from previous scan or directory walk
}

type Scanner struct {
	mu          sync.Mutex
	musicDir    string
	store       *store.Store
	status      ScanStatus
	scanCancel  context.CancelFunc
	initialScan bool
}

func NewScanner(musicDir string, s *store.Store) *Scanner {
	return &Scanner{musicDir: musicDir, store: s}
}

func (sc *Scanner) SetMusicDir(dir string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.scanCancel != nil {
		sc.scanCancel()
	}
	sc.musicDir = dir
}

func (sc *Scanner) MusicDir() string {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.musicDir
}

func (sc *Scanner) GetStatus() ScanStatus {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.status
}

func (sc *Scanner) IsInitialScan() bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.status.Scanning && sc.initialScan
}

func (sc *Scanner) Scan(ctx context.Context) (int, error) {
	sc.mu.Lock()
	if sc.scanCancel != nil {
		sc.scanCancel()
	}
	scanCtx, cancel := context.WithCancel(ctx)
	sc.scanCancel = cancel
	musicDir := sc.musicDir
	sc.mu.Unlock()
	defer cancel()

	slog.Info("starting library scan", "dir", musicDir)

	// Check if this is an initial scan (DB empty)
	trackCount, _ := sc.store.TrackCount(scanCtx)
	sc.mu.Lock()
	sc.initialScan = trackCount == 0
	sc.mu.Unlock()

	// Quick count of audio files (only readdir, no file opens — fast even over SMB)
	total := 0
	filepath.WalkDir(musicDir, func(path string, d fs.DirEntry, err error) error {
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

	sc.mu.Lock()
	sc.status = ScanStatus{Scanning: true, Scanned: 0, Total: total}
	sc.mu.Unlock()
	slog.Info("scan: counted files", "total", total)

	existingPaths := make(map[string]bool)
	var count int

	err := filepath.WalkDir(musicDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("walk error", "path", path, "err", err)
			return nil
		}

		if scanCtx.Err() != nil {
			return scanCtx.Err()
		}

		if d.IsDir() {
			name := d.Name()
			if name == "to_delete" || name == "@eaDir" || name == "#recycle" ||
				strings.HasPrefix(name, "@") || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !IsAudioFile(d.Name()) {
			return nil
		}

		relPath, err := filepath.Rel(musicDir, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)
		existingPaths[relPath] = true

		// Skip metadata reading for tracks already in DB (major speedup on rescan)
		if sc.store.TrackExistsByPath(scanCtx, relPath) {
			count++
			sc.mu.Lock()
			sc.status.Scanned = count
			sc.mu.Unlock()
			return nil
		}

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

		if err := sc.store.UpsertTrack(scanCtx, track); err != nil {
			slog.Warn("upsert error", "path", relPath, "err", err)
			return nil
		}

		count++
		sc.mu.Lock()
		sc.status.Scanned = count
		sc.mu.Unlock()

		if count%100 == 0 {
			slog.Info("scan progress", "tracks", count, "total", total)
		}

		return nil
	})

	sc.mu.Lock()
	sc.status.Scanning = false
	sc.status.Scanned = count
	sc.initialScan = false
	sc.mu.Unlock()

	if err != nil {
		return count, err
	}

	// Mark tracks whose files no longer exist on disk
	// Skip if walk found no files (likely mount not ready)
	var orphaned int
	if count > 0 {
		orphaned, err = sc.store.MarkMissingAsDeleted(scanCtx, existingPaths, musicDir)
		if err != nil {
			slog.Warn("cleanup orphaned tracks failed", "err", err)
		} else if orphaned > 0 {
			slog.Info("marked orphaned tracks as deleted", "count", orphaned)
		}
	}

	slog.Info("library scan complete", "tracks_found", count, "orphaned", orphaned)
	return count, nil
}

// ScanFolder rescans a specific folder and its subfolders.
// Unlike Scan, it does not mark missing tracks as deleted.
func (sc *Scanner) ScanFolder(ctx context.Context, folder string) (int, error) {
	sc.mu.Lock()
	musicDir := sc.musicDir
	sc.mu.Unlock()

	if musicDir == "" {
		return 0, nil
	}

	dir := filepath.Join(musicDir, filepath.FromSlash(folder))
	slog.Info("partial rescan", "folder", folder, "dir", dir)

	var count int

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() {
			name := d.Name()
			if name == "to_delete" || name == "@eaDir" || name == "#recycle" ||
				strings.HasPrefix(name, "@") || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !IsAudioFile(d.Name()) {
			return nil
		}

		relPath, err := filepath.Rel(musicDir, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)

		meta, err := ReadMetadata(path, relPath)
		if err != nil {
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
			return nil
		}

		count++
		return nil
	})

	if err != nil {
		return count, err
	}

	slog.Info("partial rescan complete", "folder", folder, "tracks", count)
	return count, nil
}

func (sc *Scanner) ListFolders(path string) ([]FolderEntry, error) {
	sc.mu.Lock()
	musicDir := sc.musicDir
	sc.mu.Unlock()

	dir := musicDir
	if path != "" {
		dir = filepath.Join(musicDir, filepath.FromSlash(path))
	}

	// Prevent directory traversal
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	absMusic, err := filepath.Abs(musicDir)
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
