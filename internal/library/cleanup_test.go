package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupEmptyDirs(t *testing.T) {
	t.Run("removes empty dir", func(t *testing.T) {
		root, err := os.MkdirTemp("", "cleanup-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(root)

		empty := filepath.Join(root, "empty")
		if err := os.MkdirAll(empty, 0o755); err != nil {
			t.Fatal(err)
		}

		CleanupEmptyDirs(empty, root)

		if _, err := os.Stat(empty); !os.IsNotExist(err) {
			t.Errorf("expected empty dir to be removed, but it still exists")
		}
	})

	t.Run("does not remove non-empty dir", func(t *testing.T) {
		root, err := os.MkdirTemp("", "cleanup-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(root)

		nonempty := filepath.Join(root, "nonempty")
		if err := os.MkdirAll(nonempty, 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(nonempty, "file.txt"))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		CleanupEmptyDirs(nonempty, root)

		if _, err := os.Stat(nonempty); err != nil {
			t.Errorf("expected non-empty dir to remain, got error: %v", err)
		}
	})

	t.Run("walks up and removes empty parents", func(t *testing.T) {
		root, err := os.MkdirTemp("", "cleanup-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(root)

		// root/a/b/c — all empty
		leaf := filepath.Join(root, "a", "b", "c")
		if err := os.MkdirAll(leaf, 0o755); err != nil {
			t.Fatal(err)
		}

		CleanupEmptyDirs(leaf, root)

		for _, dir := range []string{
			filepath.Join(root, "a", "b", "c"),
			filepath.Join(root, "a", "b"),
			filepath.Join(root, "a"),
		} {
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Errorf("expected %s to be removed, but it still exists", dir)
			}
		}
	})

	t.Run("stops at stopAt and never removes it", func(t *testing.T) {
		root, err := os.MkdirTemp("", "cleanup-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(root)

		leaf := filepath.Join(root, "sub")
		if err := os.MkdirAll(leaf, 0o755); err != nil {
			t.Fatal(err)
		}

		CleanupEmptyDirs(leaf, root)

		// stopAt (root) must still exist
		if _, err := os.Stat(root); err != nil {
			t.Errorf("stopAt dir was removed, but it should not be: %v", err)
		}

		// the empty child should be gone
		if _, err := os.Stat(leaf); !os.IsNotExist(err) {
			t.Errorf("expected child dir to be removed, but it still exists")
		}
	})

	t.Run("handles non-existent dir gracefully", func(t *testing.T) {
		root, err := os.MkdirTemp("", "cleanup-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(root)

		nonexistent := filepath.Join(root, "does", "not", "exist")

		// must not panic or crash
		CleanupEmptyDirs(nonexistent, root)
	})
}
