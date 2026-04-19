package library

import (
	"io/fs"
	"os"
	"path/filepath"
)

// ignoredFiles are OS-generated files that don't count as "real content"
// when deciding whether a directory is empty.
var ignoredFiles = map[string]bool{
	".DS_Store":   true, // macOS
	"Thumbs.db":   true, // Windows
	"desktop.ini": true, // Windows
}

func isIgnoredEntry(name string) bool {
	// Reuse the directory skip list (covers @eaDir, #recycle, dot-prefixed, etc.)
	if IsSkippedDir(name) {
		return true
	}
	// Also ignore OS-generated files
	return ignoredFiles[name]
}

// CleanupEmptyDirs removes dir if effectively empty (only contains ignored entries),
// then walks up deleting empty parents. Stops at stopAt (never deletes stopAt itself).
func CleanupEmptyDirs(dir string, stopAt string) {
	dir = filepath.Clean(dir)
	stopAt = filepath.Clean(stopAt)

	for dir != stopAt && dir != "." && dir != "/" {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}

		hasRealContent := false
		for _, e := range entries {
			if !isIgnoredEntry(e.Name()) {
				hasRealContent = true
				break
			}
		}
		if hasRealContent {
			return
		}

		// Remove ignored entries first, then the dir itself
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			os.RemoveAll(p)
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

// CleanupOrphanedAudioDir removes dir (and empty parents up to stopAt) when it
// contains no audio files anywhere in its subtree. Unlike CleanupEmptyDirs,
// this will delete non-audio residuals (covers, .cue, .log, etc.) that remain
// after the last audio file is purged. Use this in purge flows; use
// CleanupEmptyDirs in restore flows where non-audio files must not be touched.
func CleanupOrphanedAudioDir(dir string, stopAt string) {
	dir = filepath.Clean(dir)
	stopAt = filepath.Clean(stopAt)

	for dir != stopAt && dir != "." && dir != "/" {
		if _, err := os.Stat(dir); err != nil {
			return
		}
		if containsAudioFile(dir) {
			return
		}
		if err := os.RemoveAll(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// containsAudioFile reports whether dir or any of its subdirectories
// contains at least one audio file (as detected by IsAudioFile).
func containsAudioFile(dir string) bool {
	found := false
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && IsAudioFile(d.Name()) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
