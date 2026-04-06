package library

import (
	"os"
	"path/filepath"
)

// CleanupEmptyDirs removes dir if empty, then walks up deleting empty parents.
// Stops at stopAt (never deletes stopAt itself).
func CleanupEmptyDirs(dir string, stopAt string) {
	dir = filepath.Clean(dir)
	stopAt = filepath.Clean(stopAt)

	for dir != stopAt && dir != "." && dir != "/" {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}
