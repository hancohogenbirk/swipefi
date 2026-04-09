package store

import (
	"path/filepath"
	"testing"
)

func TestSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	version, err := s.getSchemaVersion()
	if err != nil {
		t.Fatalf("getSchemaVersion: %v", err)
	}
	if version < 1 {
		t.Errorf("expected schema version >= 1 after New, got %d", version)
	}
}

func TestMigrationIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s1, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	v1, err := s1.getSchemaVersion()
	if err != nil {
		t.Fatalf("getSchemaVersion (first open): %v", err)
	}
	s1.Close()

	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	defer s2.Close()

	v2, err := s2.getSchemaVersion()
	if err != nil {
		t.Fatalf("getSchemaVersion (second open): %v", err)
	}

	if v1 != v2 {
		t.Errorf("schema version changed between opens: %d -> %d", v1, v2)
	}
	if v2 < 1 {
		t.Errorf("expected schema version >= 1 after second open, got %d", v2)
	}
}
