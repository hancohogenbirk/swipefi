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

// DeleteDirName is the folder name used for rejected tracks (relative to music dir).
const DeleteDirName = "to_delete"

// skippedDirs are directories excluded from scanning, browsing, and cleanup.
var skippedDirs = map[string]bool{
	DeleteDirName: true,
	"@eaDir":      true, // Synology extended attributes
	"#recycle":    true, // Synology recycle bin
}

// DeleteDir returns the full path to the delete directory for a given music dir.
func DeleteDir(musicDir string) string {
	return filepath.Join(musicDir, DeleteDirName)
}

// IsSkippedDir returns true if the directory name should be skipped during
// scanning, browsing, and cleanup (system dirs, hidden dirs, delete dir).
func IsSkippedDir(name string) bool {
	if skippedDirs[name] {
		return true
	}
	return strings.HasPrefix(name, "@") ||
		strings.HasPrefix(name, ".")
}

type ScanStatus struct {
	Scanning bool   `json:"scanning"`
	Scanned  int    `json:"scanned"`
	Total    int    `json:"total"`
	Phase    string `json:"phase,omitempty"` // "counting", "scanning", "cleanup"
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

// MarkScanning sets the scan status to scanning immediately.
// Call before starting the scan goroutine so the frontend sees scanning=true
// on the first poll (avoids race between goroutine start and status check).
func (sc *Scanner) MarkScanning() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.status.Scanning = true
	sc.status.Scanned = 0
	sc.status.Total = 0
	sc.status.Phase = "counting"
}

func (sc *Scanner) Scan(ctx context.Context, force bool, purgeOrphans ...bool) (int, error) {
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
			if IsSkippedDir(name) {
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
	sc.status = ScanStatus{Scanning: true, Scanned: 0, Total: total, Phase: "scanning"}
	sc.mu.Unlock()
	slog.Info("scan: counted files", "total", total)

	existingPaths := make(map[string]bool)
	var count int
	var batch []*store.Track
	const batchSize = 500

	flushBatch := func() {
		if len(batch) == 0 {
			return
		}
		if err := sc.store.UpsertTrackBatch(scanCtx, batch); err != nil {
			slog.Warn("batch upsert error", "err", err)
		}
		batch = batch[:0]
	}

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
			if IsSkippedDir(name) {
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
		if !force && sc.store.TrackExistsByPath(scanCtx, relPath) {
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
			MusicDir:   musicDir,
		}

		batch = append(batch, track)
		if len(batch) >= batchSize {
			flushBatch()
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

	// Flush remaining batch
	flushBatch()

	if err != nil {
		sc.mu.Lock()
		sc.status.Scanning = false
		sc.initialScan = false
		sc.mu.Unlock()
		return count, err
	}

	// Handle tracks whose files no longer exist on disk
	// Skip if walk found no files (likely mount not ready)
	// NOTE: keep Scanning=true until cleanup is done — the purge/soft-delete
	// phase modifies the DB and track counts change during this time.
	shouldPurge := len(purgeOrphans) > 0 && purgeOrphans[0]
	var orphaned int
	if count > 0 {
		sc.mu.Lock()
		sc.status.Phase = "cleanup"
		sc.mu.Unlock()
		if shouldPurge {
			// Music dir changed — hard-delete old tracks (don't pollute deletion UI)
			var purgeErr error
			orphaned, purgeErr = sc.store.PurgeMissingTracks(scanCtx, existingPaths)
			if purgeErr != nil {
				slog.Warn("purge orphaned tracks failed", "err", purgeErr)
			} else if orphaned > 0 {
				slog.Info("purged orphaned tracks from old directory", "count", orphaned)
			}
		} else {
			// Normal rescan — soft-delete missing tracks (user can restore)
			var deletedPaths []string
			var markErr error
			orphaned, deletedPaths, markErr = sc.store.MarkMissingAsDeleted(scanCtx, existingPaths, musicDir)
			if markErr != nil {
				slog.Warn("cleanup orphaned tracks failed", "err", markErr)
			} else if orphaned > 0 {
				slog.Info("marked orphaned tracks as deleted", "count", orphaned)
				for _, p := range deletedPaths {
					dir := filepath.Dir(filepath.Join(musicDir, filepath.FromSlash(p)))
					CleanupEmptyDirs(dir, musicDir)
				}
			}
		}
	}

	// Only mark scan as done AFTER cleanup is complete
	sc.mu.Lock()
	sc.status.Scanning = false
	sc.status.Scanned = count
	sc.initialScan = false
	sc.mu.Unlock()

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
			if IsSkippedDir(name) {
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
			MusicDir:   musicDir,
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
		if IsSkippedDir(name) {
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
