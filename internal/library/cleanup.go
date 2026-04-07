package library

import (
	"os"
	"path/filepath"
	"strings"
)

// ignoredEntries are files/dirs created by OS or NAS indexing services
// that should not prevent a directory from being considered "empty".
var ignoredEntries = map[string]bool{
	"@eaDir":     true, // Synology extended attributes
	"#recycle":   true, // Synology recycle bin
	".DS_Store":  true, // macOS
	"Thumbs.db":  true, // Windows
	"desktop.ini": true, // Windows
}

func isIgnoredEntry(name string) bool {
	if ignoredEntries[name] {
		return true
	}
	// Also ignore any dot-prefixed entries (hidden files/dirs)
	return strings.HasPrefix(name, ".")
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
