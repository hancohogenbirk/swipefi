package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type Track struct {
	ID           int64  `json:"id"`
	Path         string `json:"path"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	DurationMs   int64  `json:"duration_ms"`
	Format       string `json:"format"`
	PlayCount    int    `json:"play_count"`
	AddedAt      int64  `json:"added_at"`
	LastPlayed   *int64 `json:"last_played,omitempty"`
	Deleted      bool   `json:"-"`
	MusicDir     string `json:"-"`
	SampleRateHz    int     `json:"sample_rate_hz,omitempty"`
	BitDepth        int     `json:"bit_depth,omitempty"`
	BitrateKbps     int     `json:"bitrate_kbps,omitempty"`
	TranscodeScore  float64 `json:"transcode_score,omitempty"`
	TranscodeSource string  `json:"transcode_source,omitempty"`
}

var ErrTrackNotFound = errors.New("track not found")

// validSortColumns and validOrderDirs whitelist allowed ORDER BY values
// to prevent any possibility of SQL injection via sort parameters.
var validSortColumns = map[string]string{
	"added_at":    "added_at",
	"play_count":  "play_count",
	"last_played": "last_played",
}

var validOrderDirs = map[string]string{
	"asc":  "ASC",
	"desc": "DESC",
}

func (s *Store) UpsertTrack(ctx context.Context, t *Track) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tracks (path, title, artist, album, duration_ms, format, added_at, created_at, music_dir, sample_rate_hz, bit_depth, bitrate_kbps)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path, music_dir) DO UPDATE SET
			title = excluded.title,
			artist = excluded.artist,
			album = excluded.album,
			duration_ms = excluded.duration_ms,
			format = excluded.format,
			deleted = 0,
			music_dir = excluded.music_dir,
			sample_rate_hz = excluded.sample_rate_hz,
			bit_depth = excluded.bit_depth,
			bitrate_kbps = excluded.bitrate_kbps
	`, t.Path, t.Title, t.Artist, t.Album, t.DurationMs, t.Format, t.AddedAt, now, t.MusicDir,
		t.SampleRateHz, t.BitDepth, t.BitrateKbps)
	if err != nil {
		return fmt.Errorf("upsert track: %w", err)
	}
	return nil
}

// CleanupMissingTracks handles tracks whose files no longer exist at their original location.
// User-rejected tracks (found in deleteDir) are soft-deleted (deleted=1, restorable).
// Externally removed tracks (not in deleteDir either) are purged from the DB entirely.
func (s *Store) CleanupMissingTracks(ctx context.Context, existingPaths map[string]bool, musicDir, deleteDir string) (softDeleted int, purged int, deletedPaths []string, purgedPaths []string, err error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, path FROM tracks WHERE deleted = 0 AND music_dir = ?", musicDir)
	if err != nil {
		return 0, 0, nil, nil, fmt.Errorf("query tracks: %w", err)
	}
	defer rows.Close()

	var toSoftDelete []int64
	var toPurge []int64
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return 0, 0, nil, nil, fmt.Errorf("scan: %w", err)
		}
		if existingPaths[path] {
			continue
		}
		// Walk didn't find it — double-check with stat before taking action
		fullPath := filepath.Join(musicDir, filepath.FromSlash(path))
		if _, err := os.Stat(fullPath); err == nil {
			continue // file exists, walk just missed it
		}
		// Check if file exists in to_delete directory (user-rejected)
		deletePath := filepath.Join(deleteDir, filepath.FromSlash(path))
		if _, statErr := os.Stat(deletePath); statErr == nil {
			// File is in to_delete — user rejected it, soft-delete
			toSoftDelete = append(toSoftDelete, id)
			deletedPaths = append(deletedPaths, path)
		} else {
			// File is nowhere — externally removed, purge from DB
			toPurge = append(toPurge, id)
			purgedPaths = append(purgedPaths, path)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, nil, nil, err
	}

	if len(toSoftDelete)+len(toPurge) > 0 {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return 0, 0, nil, nil, fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback()

		if len(toSoftDelete) > 0 {
			stmt, err := tx.PrepareContext(ctx, "UPDATE tracks SET deleted = 1 WHERE id = ?")
			if err != nil {
				return 0, 0, nil, nil, fmt.Errorf("prepare mark deleted: %w", err)
			}
			defer stmt.Close()
			for _, id := range toSoftDelete {
				if _, err := stmt.ExecContext(ctx, id); err != nil {
					slog.Warn("failed to mark track deleted", "id", id, "err", err)
				}
			}
		}

		if len(toPurge) > 0 {
			stmt, err := tx.PrepareContext(ctx, "DELETE FROM tracks WHERE id = ?")
			if err != nil {
				return 0, 0, nil, nil, fmt.Errorf("prepare purge: %w", err)
			}
			defer stmt.Close()
			for _, id := range toPurge {
				if _, err := stmt.ExecContext(ctx, id); err != nil {
					slog.Warn("failed to purge track", "id", id, "err", err)
				}
			}
		}

		if err := tx.Commit(); err != nil {
			return 0, 0, nil, nil, fmt.Errorf("commit cleanup: %w", err)
		}
	}

	return len(toSoftDelete), len(toPurge), deletedPaths, purgedPaths, nil
}

// UpsertTrackBatch inserts or updates multiple tracks in a single transaction.
func (s *Store) UpsertTrackBatch(ctx context.Context, tracks []*Track) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO tracks (path, title, artist, album, duration_ms, format, added_at, created_at, music_dir, sample_rate_hz, bit_depth, bitrate_kbps)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path, music_dir) DO UPDATE SET
			title = excluded.title,
			artist = excluded.artist,
			album = excluded.album,
			duration_ms = excluded.duration_ms,
			format = excluded.format,
			deleted = 0,
			music_dir = excluded.music_dir,
			sample_rate_hz = excluded.sample_rate_hz,
			bit_depth = excluded.bit_depth,
			bitrate_kbps = excluded.bitrate_kbps
	`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, t := range tracks {
		if _, err := stmt.ExecContext(ctx, t.Path, t.Title, t.Artist, t.Album, t.DurationMs, t.Format, t.AddedAt, now, t.MusicDir,
			t.SampleRateHz, t.BitDepth, t.BitrateKbps); err != nil {
			return fmt.Errorf("exec %s: %w", t.Path, err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetTrack(ctx context.Context, id int64) (*Track, error) {
	var t Track
	var lastPlayed sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, path, title, artist, album, duration_ms, format, play_count, added_at, last_played, deleted, sample_rate_hz, bit_depth, bitrate_kbps, transcode_score, transcode_source
		FROM tracks WHERE id = ?
	`, id).Scan(&t.ID, &t.Path, &t.Title, &t.Artist, &t.Album, &t.DurationMs, &t.Format,
		&t.PlayCount, &t.AddedAt, &lastPlayed, &t.Deleted, &t.SampleRateHz, &t.BitDepth, &t.BitrateKbps,
		&t.TranscodeScore, &t.TranscodeSource)
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

// GetTrackByPath returns a non-deleted track by its relative path.
func (s *Store) GetTrackByPath(ctx context.Context, path string) (*Track, error) {
	var t Track
	var lastPlayed sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, path, title, artist, album, duration_ms, format, play_count, added_at, last_played, deleted, sample_rate_hz, bit_depth, bitrate_kbps, transcode_score, transcode_source
		FROM tracks WHERE path = ? AND deleted = 0 AND music_dir = ?
	`, path, s.getMusicDir()).Scan(&t.ID, &t.Path, &t.Title, &t.Artist, &t.Album, &t.DurationMs, &t.Format,
		&t.PlayCount, &t.AddedAt, &lastPlayed, &t.Deleted, &t.SampleRateHz, &t.BitDepth, &t.BitrateKbps,
		&t.TranscodeScore, &t.TranscodeSource)
	if err == sql.ErrNoRows {
		return nil, ErrTrackNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get track by path %s: %w", path, err)
	}
	if lastPlayed.Valid {
		t.LastPlayed = &lastPlayed.Int64
	}
	return &t, nil
}

// ListTracks returns tracks in a folder recursively (for playback queue building).
func (s *Store) ListTracks(ctx context.Context, folder, sortBy, order string) ([]Track, error) {
	query := `SELECT id, path, title, artist, album, duration_ms, format, play_count, added_at, last_played, deleted, sample_rate_hz, bit_depth, bitrate_kbps, transcode_score, transcode_source
		FROM tracks WHERE deleted = 0 AND music_dir = ?`

	args := []any{s.getMusicDir()}
	if folder != "" {
		query += " AND path LIKE ?"
		args = append(args, folder+"/%")
	}

	sortCol := validSortColumns[sortBy]
	if sortCol == "" {
		sortCol = "added_at"
	}

	orderDir := validOrderDirs[order]
	if orderDir == "" {
		orderDir = "ASC"
	}

	query += " ORDER BY " + sortCol + " " + orderDir

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
			&t.Format, &t.PlayCount, &t.AddedAt, &lastPlayed, &t.Deleted,
			&t.SampleRateHz, &t.BitDepth, &t.BitrateKbps,
				&t.TranscodeScore, &t.TranscodeSource); err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}
		if lastPlayed.Valid {
			t.LastPlayed = &lastPlayed.Int64
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

// ListTracksDirectOnly returns only tracks that are direct children of a folder (not in subfolders).
func (s *Store) ListTracksDirectOnly(ctx context.Context, folder, sortBy, order string) ([]Track, error) {
	query := `SELECT id, path, title, artist, album, duration_ms, format, play_count, added_at, last_played, deleted, sample_rate_hz, bit_depth, bitrate_kbps, transcode_score, transcode_source
		FROM tracks WHERE deleted = 0 AND music_dir = ?`

	args := []any{s.getMusicDir()}
	if folder != "" {
		query += " AND path LIKE ? AND path NOT LIKE ?"
		args = append(args, folder+"/%", folder+"/%/%")
	} else {
		query += " AND path NOT LIKE '%/%'"
	}

	sortCol := validSortColumns[sortBy]
	if sortCol == "" {
		sortCol = "added_at"
	}

	orderDir := validOrderDirs[order]
	if orderDir == "" {
		orderDir = "ASC"
	}

	query += " ORDER BY " + sortCol + " " + orderDir

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tracks direct: %w", err)
	}
	defer rows.Close()

	var tracks []Track
	for rows.Next() {
		var t Track
		var lastPlayed sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Path, &t.Title, &t.Artist, &t.Album, &t.DurationMs,
			&t.Format, &t.PlayCount, &t.AddedAt, &lastPlayed, &t.Deleted,
			&t.SampleRateHz, &t.BitDepth, &t.BitrateKbps,
				&t.TranscodeScore, &t.TranscodeSource); err != nil {
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
		"SELECT COUNT(*) FROM tracks WHERE deleted = 0 AND path LIKE ? AND music_dir = ?",
		folder+"/%", s.getMusicDir(),
	).Scan(&count)
	return err == nil && count > 0
}

func (s *Store) TrackCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tracks WHERE deleted = 0 AND music_dir = ?", s.getMusicDir()).Scan(&count)
	return count, err
}

// TrackExistsByPath checks if a non-deleted track with the given path exists.
func (s *Store) TrackExistsByPath(ctx context.Context, path string) bool {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tracks WHERE path = ? AND deleted = 0 AND music_dir = ?", path, s.getMusicDir()).Scan(&count)
	return err == nil && count > 0
}

// ListDeleted returns all tracks marked as deleted.
func (s *Store) ListDeleted(ctx context.Context) ([]Track, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, path, title, artist, album, duration_ms, format, play_count, added_at, last_played, deleted, sample_rate_hz, bit_depth, bitrate_kbps, transcode_score, transcode_source
		FROM tracks WHERE deleted = 1 AND music_dir = ? ORDER BY title ASC
	`, s.getMusicDir())
	if err != nil {
		return nil, fmt.Errorf("list deleted: %w", err)
	}
	defer rows.Close()

	var tracks []Track
	for rows.Next() {
		var t Track
		var lastPlayed sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Path, &t.Title, &t.Artist, &t.Album, &t.DurationMs,
			&t.Format, &t.PlayCount, &t.AddedAt, &lastPlayed, &t.Deleted,
			&t.SampleRateHz, &t.BitDepth, &t.BitrateKbps,
				&t.TranscodeScore, &t.TranscodeSource); err != nil {
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
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tracks WHERE deleted = 1 AND music_dir = ?", s.getMusicDir()).Scan(&count)
	return count, err
}

// UpdateTranscodeAnalysis sets the transcode analysis results for a track.
func (s *Store) UpdateTranscodeAnalysis(ctx context.Context, id int64, score float64, source string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tracks SET transcode_score = ?, transcode_source = ? WHERE id = ?
	`, score, source, id)
	if err != nil {
		return fmt.Errorf("update transcode analysis %d: %w", id, err)
	}
	return nil
}

// ResetTranscodeScores resets all transcode scores to -1 so they get re-analyzed.
func (s *Store) ResetTranscodeScores(ctx context.Context, musicDir string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tracks SET transcode_score = -1, transcode_source = ''
		WHERE format = 'flac' AND deleted = 0 AND music_dir = ?
	`, musicDir)
	return err
}

// ListTracksNeedingAnalysis returns FLAC tracks that haven't been analyzed for transcoding.
func (s *Store) ListTracksNeedingAnalysis(ctx context.Context, musicDir string) ([]Track, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, path FROM tracks
		WHERE transcode_score = -1 AND format = 'flac' AND deleted = 0 AND music_dir = ?
	`, musicDir)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []Track
	for rows.Next() {
		var t Track
		if err := rows.Scan(&t.ID, &t.Path); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

// UpdateTrackAudioInfo sets the audio format fields for a track.
func (s *Store) UpdateTrackAudioInfo(ctx context.Context, id int64, sampleRate, bitDepth, bitrateKbps int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tracks SET sample_rate_hz = ?, bit_depth = ?, bitrate_kbps = ? WHERE id = ?
	`, sampleRate, bitDepth, bitrateKbps, id)
	if err != nil {
		return fmt.Errorf("update audio info %d: %w", id, err)
	}
	return nil
}

// ListTracksNeedingAudioInfo returns FLAC tracks where audio info hasn't been populated.
func (s *Store) ListTracksNeedingAudioInfo(ctx context.Context, musicDir string) ([]Track, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, path, format FROM tracks
		WHERE sample_rate_hz = 0 AND format = 'flac' AND deleted = 0 AND music_dir = ?
	`, musicDir)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []Track
	for rows.Next() {
		var t Track
		if err := rows.Scan(&t.ID, &t.Path, &t.Format); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}
