package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

type Store struct {
	db       *sql.DB
	mu       sync.RWMutex
	musicDir string
}

func (s *Store) SetMusicDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.musicDir = dir
}

func (s *Store) getMusicDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.musicDir
}

func (s *Store) MusicDir() string {
	return s.getMusicDir()
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := s.migrateConfig(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate config: %w", err)
	}

	version, _ := s.getSchemaVersion()

	if version < 1 {
		if err := s.migrateMusicDir(); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate music_dir: %w", err)
		}
		if err := s.setSchemaVersion(1); err != nil {
			db.Close()
			return nil, fmt.Errorf("set schema version: %w", err)
		}
	}

	return s, nil
}

func (s *Store) getSchemaVersion() (int, error) {
	var v int
	err := s.db.QueryRow("SELECT value FROM config WHERE key = 'schema_version'").Scan(&v)
	if err != nil {
		return 0, nil // no row = version 0
	}
	return v, nil
}

func (s *Store) setSchemaVersion(v int) error {
	_, err := s.db.Exec(`
		INSERT INTO config (key, value) VALUES ('schema_version', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, v)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrateMusicDir() error {
	// Step 1: Add music_dir column if it doesn't exist
	_, err := s.db.Exec(`ALTER TABLE tracks ADD COLUMN music_dir TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		var dummy string
		checkErr := s.db.QueryRow("SELECT music_dir FROM tracks LIMIT 1").Scan(&dummy)
		if checkErr != nil && checkErr != sql.ErrNoRows {
			return fmt.Errorf("add music_dir column: %w", err)
		}
	}

	// Step 2: Migrate UNIQUE constraint from (path) to (path, music_dir)
	// Check if migration is needed by looking at the table schema
	var tableSql string
	err = s.db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='tracks'").Scan(&tableSql)
	if err != nil {
		return fmt.Errorf("read table schema: %w", err)
	}

	// If the table does not yet have the composite UNIQUE(path, music_dir) constraint,
	// recreate it. This covers both the fresh-table and already-altered-table cases.
	if !strings.Contains(tableSql, "UNIQUE(path, music_dir)") {
		if _, err = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS tracks_new (
				id          INTEGER PRIMARY KEY AUTOINCREMENT,
				path        TEXT NOT NULL,
				title       TEXT NOT NULL DEFAULT '',
				artist      TEXT NOT NULL DEFAULT '',
				album       TEXT NOT NULL DEFAULT '',
				duration_ms INTEGER NOT NULL DEFAULT 0,
				format      TEXT NOT NULL DEFAULT '',
				play_count  INTEGER NOT NULL DEFAULT 0,
				added_at    INTEGER NOT NULL,
				last_played INTEGER,
				deleted     INTEGER NOT NULL DEFAULT 0,
				created_at  INTEGER NOT NULL,
				music_dir   TEXT NOT NULL DEFAULT '',
				UNIQUE(path, music_dir)
			)`); err != nil {
			return fmt.Errorf("create tracks_new: %w", err)
		}
		if _, err = s.db.Exec(`INSERT OR IGNORE INTO tracks_new SELECT * FROM tracks`); err != nil {
			return fmt.Errorf("copy tracks: %w", err)
		}
		if _, err = s.db.Exec(`DROP TABLE tracks`); err != nil {
			return fmt.Errorf("drop old tracks: %w", err)
		}
		if _, err = s.db.Exec(`ALTER TABLE tracks_new RENAME TO tracks`); err != nil {
			return fmt.Errorf("rename tracks_new: %w", err)
		}
	}

	// Recreate indexes (may have been dropped with the table)
	for _, idx := range []string{
		"CREATE INDEX IF NOT EXISTS idx_tracks_music_dir ON tracks(music_dir)",
		"CREATE INDEX IF NOT EXISTS idx_tracks_play_count ON tracks(play_count)",
		"CREATE INDEX IF NOT EXISTS idx_tracks_added_at ON tracks(added_at)",
		"CREATE INDEX IF NOT EXISTS idx_tracks_path ON tracks(path)",
	} {
		if _, err := s.db.Exec(idx); err != nil {
			slog.Warn("failed to create index", "sql", idx, "err", err)
		}
	}

	return nil
}

func (s *Store) BackfillMusicDir(musicDir string) error {
	if musicDir == "" {
		return nil
	}
	_, err := s.db.Exec("UPDATE tracks SET music_dir = ? WHERE music_dir = ''", musicDir)
	if err != nil {
		return fmt.Errorf("backfill music_dir: %w", err)
	}
	return nil
}

func (s *Store) migrate() error {
	slog.Info("running database migrations")
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tracks (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			path        TEXT UNIQUE NOT NULL,
			title       TEXT NOT NULL DEFAULT '',
			artist      TEXT NOT NULL DEFAULT '',
			album       TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			format      TEXT NOT NULL DEFAULT '',
			play_count  INTEGER NOT NULL DEFAULT 0,
			added_at    INTEGER NOT NULL,
			last_played INTEGER,
			deleted     INTEGER NOT NULL DEFAULT 0,
			created_at  INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_tracks_play_count ON tracks(play_count);
		CREATE INDEX IF NOT EXISTS idx_tracks_added_at ON tracks(added_at);
		CREATE INDEX IF NOT EXISTS idx_tracks_path ON tracks(path);
	`)
	return err
}
