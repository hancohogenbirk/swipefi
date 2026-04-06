package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Track struct {
	ID         int64  `json:"id"`
	Path       string `json:"path"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	DurationMs int64  `json:"duration_ms"`
	Format     string `json:"format"`
	PlayCount  int    `json:"play_count"`
	AddedAt    int64  `json:"added_at"`
	LastPlayed *int64 `json:"last_played,omitempty"`
	Deleted    bool   `json:"-"`
}

var ErrTrackNotFound = errors.New("track not found")

func (s *Store) UpsertTrack(ctx context.Context, t *Track) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tracks (path, title, artist, album, duration_ms, format, added_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			title = excluded.title,
			artist = excluded.artist,
			album = excluded.album,
			duration_ms = excluded.duration_ms,
			format = excluded.format,
			deleted = 0
	`, t.Path, t.Title, t.Artist, t.Album, t.DurationMs, t.Format, t.AddedAt, now)
	if err != nil {
		return fmt.Errorf("upsert track: %w", err)
	}
	return nil
}

// MarkMissingAsDeleted marks tracks as deleted if their path is not in the provided set.
func (s *Store) MarkMissingAsDeleted(ctx context.Context, existingPaths map[string]bool) (int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, path FROM tracks WHERE deleted = 0")
	if err != nil {
		return 0, fmt.Errorf("query tracks: %w", err)
	}
	defer rows.Close()

	var toDelete []int64
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return 0, fmt.Errorf("scan: %w", err)
		}
		if !existingPaths[path] {
			toDelete = append(toDelete, id)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, id := range toDelete {
		s.db.ExecContext(ctx, "UPDATE tracks SET deleted = 1 WHERE id = ?", id)
	}

	return len(toDelete), nil
}

func (s *Store) GetTrack(ctx context.Context, id int64) (*Track, error) {
	var t Track
	var lastPlayed sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, path, title, artist, album, duration_ms, format, play_count, added_at, last_played, deleted
		FROM tracks WHERE id = ?
	`, id).Scan(&t.ID, &t.Path, &t.Title, &t.Artist, &t.Album, &t.DurationMs, &t.Format,
		&t.PlayCount, &t.AddedAt, &lastPlayed, &t.Deleted)
	if err == sql.ErrNoRows {
		return nil, ErrTrackNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get track %d: %w", id, err)
	}
	if lastPlayed.Valid {
		t.LastPlayed = &lastPlayed.Int64
	}
	return &t, nil
}

func (s *Store) ListTracks(ctx context.Context, folder, sortBy, order string) ([]Track, error) {
	query := `SELECT id, path, title, artist, album, duration_ms, format, play_count, added_at, last_played, deleted
		FROM tracks WHERE deleted = 0`

	var args []any
	if folder != "" {
		// Only match direct children: path starts with folder/ but has no further /
		query += " AND path LIKE ? AND path NOT LIKE ?"
		args = append(args, folder+"/%", folder+"/%/%")
	} else {
		// Root: only files with no / in path (direct children of music dir)
		query += " AND path NOT LIKE '%/%'"
	}

	sortCol := "added_at"
	if sortBy == "play_count" {
		sortCol = "play_count"
	}

	orderDir := "ASC"
	if order == "desc" {
		orderDir = "DESC"
	}

	query += fmt.Sprintf(" ORDER BY %s %s", sortCol, orderDir)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tracks: %w", err)
	}
	defer rows.Close()

	var tracks []Track
	for rows.Next() {
		var t Track
		var lastPlayed sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Path, &t.Title, &t.Artist, &t.Album, &t.DurationMs,
			&t.Format, &t.PlayCount, &t.AddedAt, &lastPlayed, &t.Deleted); err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}
		if lastPlayed.Valid {
			t.LastPlayed = &lastPlayed.Int64
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

func (s *Store) IncrementPlayCount(ctx context.Context, id int64) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		UPDATE tracks SET play_count = play_count + 1, last_played = ? WHERE id = ?
	`, now, id)
	if err != nil {
		return fmt.Errorf("increment play count %d: %w", id, err)
	}
	return nil
}

func (s *Store) MarkDeleted(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tracks SET deleted = 1 WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("mark deleted %d: %w", id, err)
	}
	return nil
}

// HasTracksInFolder returns true if there are any non-deleted tracks under the given folder.
func (s *Store) HasTracksInFolder(folder string) bool {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM tracks WHERE deleted = 0 AND path LIKE ?",
		folder+"/%",
	).Scan(&count)
	return err == nil && count > 0
}

func (s *Store) TrackCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tracks WHERE deleted = 0").Scan(&count)
	return count, err
}

// TrackExistsByPath checks if a non-deleted track with the given path exists.
func (s *Store) TrackExistsByPath(ctx context.Context, path string) bool {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tracks WHERE path = ? AND deleted = 0", path).Scan(&count)
	return err == nil && count > 0
}

// ListDeleted returns all tracks marked as deleted.
func (s *Store) ListDeleted(ctx context.Context) ([]Track, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, path, title, artist, album, duration_ms, format, play_count, added_at, last_played, deleted
		FROM tracks WHERE deleted = 1 ORDER BY title ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list deleted: %w", err)
	}
	defer rows.Close()

	var tracks []Track
	for rows.Next() {
		var t Track
		var lastPlayed sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Path, &t.Title, &t.Artist, &t.Album, &t.DurationMs,
			&t.Format, &t.PlayCount, &t.AddedAt, &lastPlayed, &t.Deleted); err != nil {
			return nil, fmt.Errorf("scan deleted track: %w", err)
		}
		if lastPlayed.Valid {
			t.LastPlayed = &lastPlayed.Int64
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

// UnmarkDeleted sets deleted = 0 for the given track ID.
func (s *Store) UnmarkDeleted(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "UPDATE tracks SET deleted = 0 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("unmark deleted %d: %w", id, err)
	}
	return nil
}

// PurgeTrack permanently removes a track from the database.
func (s *Store) PurgeTrack(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM tracks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("purge track %d: %w", id, err)
	}
	return nil
}

// DeletedCount returns the number of tracks marked as deleted.
func (s *Store) DeletedCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tracks WHERE deleted = 1").Scan(&count)
	return count, err
}
