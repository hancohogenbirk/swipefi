package library

import (
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
