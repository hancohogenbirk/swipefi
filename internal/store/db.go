package store

import (
	"database/sql"
	"fmt"
	"log/slog"
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
	if err := s.migrateMusicDir(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate music_dir: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrateMusicDir() error {
	_, err := s.db.Exec(`
		ALTER TABLE tracks ADD COLUMN music_dir TEXT NOT NULL DEFAULT ''
	`)
	if err != nil {
		var dummy string
		checkErr := s.db.QueryRow("SELECT music_dir FROM tracks LIMIT 1").Scan(&dummy)
		if checkErr != nil && checkErr != sql.ErrNoRows {
			return fmt.Errorf("add music_dir column: %w", err)
		}
	}
	s.db.Exec("CREATE INDEX IF NOT EXISTS idx_tracks_music_dir ON tracks(music_dir)")
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
