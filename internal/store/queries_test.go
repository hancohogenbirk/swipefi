package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testMusicDir = "/test/music"

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.SetMusicDir(testMusicDir)
	t.Cleanup(func() { s.Close() })
	return s
}

func newTrack(path, title, artist, album string) *Track {
	return &Track{
		Path:       path,
		Title:      title,
		Artist:     artist,
		Album:      album,
		DurationMs: 180000,
		Format:     "flac",
		AddedAt:    time.Now().Unix(),
		MusicDir:   testMusicDir,
	}
}

func TestUpsertTrack_InsertAndUpdate(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	track := newTrack("music/song.flac", "Song", "Artist", "Album")
	if err := s.UpsertTrack(ctx, track); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Verify it was inserted.
	count, err := s.TrackCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 track, got %d", count)
	}

	// Update on conflict: same path, different metadata.
	updated := newTrack("music/song.flac", "New Title", "New Artist", "New Album")
	if err := s.UpsertTrack(ctx, updated); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	// Should still be exactly 1 track.
	count, err = s.TrackCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 track after update, got %d", count)
	}
}

func TestUpsertTrack_ClearsDeletedFlag(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	track := newTrack("music/song.flac", "Song", "Artist", "Album")
	if err := s.UpsertTrack(ctx, track); err != nil {
		t.Fatal(err)
	}

	// Fetch the ID.
	tracks, err := s.ListTracks(ctx, "", "added_at", "asc")
	if err != nil || len(tracks) == 0 {
		t.Fatalf("list tracks: %v, len=%d", err, len(tracks))
	}
	id := tracks[0].ID

	// Mark deleted.
	if err := s.MarkDeleted(ctx, id); err != nil {
		t.Fatal(err)
	}

	// Upsert the same path again — should clear deleted.
	if err := s.UpsertTrack(ctx, track); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTrack(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Deleted {
		t.Error("expected deleted=false after upsert, got true")
	}
}

func TestGetTrack_Found(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	track := newTrack("music/track.flac", "Title", "Artist", "Album")
	if err := s.UpsertTrack(ctx, track); err != nil {
		t.Fatal(err)
	}

	tracks, err := s.ListTracks(ctx, "", "added_at", "asc")
	if err != nil || len(tracks) == 0 {
		t.Fatalf("list tracks: %v", err)
	}

	got, err := s.GetTrack(ctx, tracks[0].ID)
	if err != nil {
		t.Fatalf("get track: %v", err)
	}
	if got.Title != "Title" || got.Artist != "Artist" || got.Album != "Album" {
		t.Errorf("unexpected track fields: %+v", got)
	}
}

func TestGetTrack_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	_, err := s.GetTrack(ctx, 9999)
	if !errors.Is(err, ErrTrackNotFound) {
		t.Errorf("expected ErrTrackNotFound, got %v", err)
	}
}

func TestListTracks_Recursive(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	tracks := []*Track{
		newTrack("music/a.flac", "A", "", ""),
		newTrack("music/sub/b.flac", "B", "", ""),
		newTrack("music/sub/deep/c.flac", "C", "", ""),
		newTrack("other/d.flac", "D", "", ""),
	}
	for _, tr := range tracks {
		if err := s.UpsertTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListTracks(ctx, "music", "added_at", "asc")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 tracks under music/, got %d", len(got))
	}
}

func TestListTracksDirectOnly(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	tracks := []*Track{
		newTrack("music/a.flac", "A", "", ""),
		newTrack("music/sub/b.flac", "B", "", ""),
		newTrack("music/sub/deep/c.flac", "C", "", ""),
	}
	for _, tr := range tracks {
		if err := s.UpsertTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListTracksDirectOnly(ctx, "music", "added_at", "asc")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 direct track under music/, got %d", len(got))
	}
	if got[0].Title != "A" {
		t.Errorf("expected track A, got %s", got[0].Title)
	}
}

func TestIncrementPlayCount(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	if err := s.UpsertTrack(ctx, newTrack("music/a.flac", "A", "", "")); err != nil {
		t.Fatal(err)
	}
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	id := tracks[0].ID

	before := time.Now().Unix()
	if err := s.IncrementPlayCount(ctx, id); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTrack(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlayCount != 1 {
		t.Errorf("expected play_count=1, got %d", got.PlayCount)
	}
	if got.LastPlayed == nil || *got.LastPlayed < before {
		t.Error("last_played not set correctly after IncrementPlayCount")
	}
}

func TestMarkDeleted(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	if err := s.UpsertTrack(ctx, newTrack("music/a.flac", "A", "", "")); err != nil {
		t.Fatal(err)
	}
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	id := tracks[0].ID

	if err := s.MarkDeleted(ctx, id); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTrack(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Deleted {
		t.Error("expected deleted=true after MarkDeleted")
	}
}

func TestUnmarkDeleted(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	if err := s.UpsertTrack(ctx, newTrack("music/a.flac", "A", "", "")); err != nil {
		t.Fatal(err)
	}
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	id := tracks[0].ID

	if err := s.MarkDeleted(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.UnmarkDeleted(ctx, id); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTrack(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Deleted {
		t.Error("expected deleted=false after UnmarkDeleted")
	}
}

func TestPurgeTrack(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	if err := s.UpsertTrack(ctx, newTrack("music/a.flac", "A", "", "")); err != nil {
		t.Fatal(err)
	}
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	id := tracks[0].ID

	if err := s.PurgeTrack(ctx, id); err != nil {
		t.Fatal(err)
	}

	_, err := s.GetTrack(ctx, id)
	if !errors.Is(err, ErrTrackNotFound) {
		t.Errorf("expected ErrTrackNotFound after purge, got %v", err)
	}
}

func TestTrackCount(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for _, path := range []string{"music/a.flac", "music/b.flac", "music/c.flac"} {
		if err := s.UpsertTrack(ctx, newTrack(path, path, "", "")); err != nil {
			t.Fatal(err)
		}
	}

	// Mark one deleted.
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if err := s.MarkDeleted(ctx, tracks[0].ID); err != nil {
		t.Fatal(err)
	}

	count, err := s.TrackCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected TrackCount=2, got %d", count)
	}
}

func TestDeletedCount(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for _, path := range []string{"music/a.flac", "music/b.flac", "music/c.flac"} {
		if err := s.UpsertTrack(ctx, newTrack(path, path, "", "")); err != nil {
			t.Fatal(err)
		}
	}

	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if err := s.MarkDeleted(ctx, tracks[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkDeleted(ctx, tracks[1].ID); err != nil {
		t.Fatal(err)
	}

	count, err := s.DeletedCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected DeletedCount=2, got %d", count)
	}
}

func TestListDeleted(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for _, path := range []string{"music/a.flac", "music/b.flac", "music/c.flac"} {
		if err := s.UpsertTrack(ctx, newTrack(path, path, "", "")); err != nil {
			t.Fatal(err)
		}
	}

	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if err := s.MarkDeleted(ctx, tracks[0].ID); err != nil {
		t.Fatal(err)
	}

	deleted, err := s.ListDeleted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 {
		t.Errorf("expected 1 deleted track, got %d", len(deleted))
	}
	if !deleted[0].Deleted {
		t.Error("track in ListDeleted should have Deleted=true")
	}
}

func TestTrackExistsByPath(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	if err := s.UpsertTrack(ctx, newTrack("music/a.flac", "A", "", "")); err != nil {
		t.Fatal(err)
	}

	if !s.TrackExistsByPath(ctx, "music/a.flac") {
		t.Error("expected TrackExistsByPath=true for existing track")
	}

	// Non-existent path.
	if s.TrackExistsByPath(ctx, "music/nonexistent.flac") {
		t.Error("expected TrackExistsByPath=false for missing path")
	}

	// Deleted track should not be found.
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if err := s.MarkDeleted(ctx, tracks[0].ID); err != nil {
		t.Fatal(err)
	}
	if s.TrackExistsByPath(ctx, "music/a.flac") {
		t.Error("expected TrackExistsByPath=false for deleted track")
	}
}

func TestHasTracksInFolder(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	if err := s.UpsertTrack(ctx, newTrack("music/sub/a.flac", "A", "", "")); err != nil {
		t.Fatal(err)
	}

	if !s.HasTracksInFolder("music/sub") {
		t.Error("expected HasTracksInFolder=true when folder has tracks")
	}
	if s.HasTracksInFolder("music/empty") {
		t.Error("expected HasTracksInFolder=false for empty folder")
	}

	// Mark the track deleted — folder should now report false.
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if err := s.MarkDeleted(ctx, tracks[0].ID); err != nil {
		t.Fatal(err)
	}
	if s.HasTracksInFolder("music/sub") {
		t.Error("expected HasTracksInFolder=false after all tracks deleted")
	}
}

func TestConfig_SetAndGet(t *testing.T) {
	s := setupTestStore(t)

	if err := s.SetConfig("theme", "dark"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	val, err := s.GetConfig("theme")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "dark" {
		t.Errorf("expected 'dark', got %q", val)
	}
}

func TestConfig_UpdateExisting(t *testing.T) {
	s := setupTestStore(t)

	if err := s.SetConfig("key", "first"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetConfig("key", "second"); err != nil {
		t.Fatal(err)
	}

	val, err := s.GetConfig("key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "second" {
		t.Errorf("expected 'second' after update, got %q", val)
	}
}

func TestConfig_MissingKeyReturnsEmpty(t *testing.T) {
	s := setupTestStore(t)

	val, err := s.GetConfig("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error for missing key: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing key, got %q", val)
	}
}

func TestUpsertTrack_PreservesPlayCount(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	track := newTrack("music/song.flac", "Song", "Artist", "Album")
	if err := s.UpsertTrack(ctx, track); err != nil {
		t.Fatal(err)
	}

	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	id := tracks[0].ID

	if err := s.IncrementPlayCount(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.IncrementPlayCount(ctx, id); err != nil {
		t.Fatal(err)
	}

	updated := newTrack("music/song.flac", "New Title", "New Artist", "New Album")
	if err := s.UpsertTrack(ctx, updated); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTrack(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlayCount != 2 {
		t.Errorf("expected play_count=2 after re-upsert, got %d", got.PlayCount)
	}
	if got.Title != "New Title" {
		t.Errorf("expected title updated, got %q", got.Title)
	}
}

func TestUpsertTrackBatch(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	batch := []*Track{
		newTrack("music/a.flac", "A", "Artist", "Album"),
		newTrack("music/b.flac", "B", "Artist", "Album"),
		newTrack("music/c.flac", "C", "Artist", "Album"),
	}

	if err := s.UpsertTrackBatch(ctx, batch); err != nil {
		t.Fatalf("batch upsert: %v", err)
	}

	count, err := s.TrackCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 tracks, got %d", count)
	}

	// Re-upsert with updated metadata — play count should survive
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if err := s.IncrementPlayCount(ctx, tracks[0].ID); err != nil {
		t.Fatal(err)
	}

	updatedBatch := []*Track{
		newTrack("music/a.flac", "A Updated", "Artist", "Album"),
	}
	if err := s.UpsertTrackBatch(ctx, updatedBatch); err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetTrack(ctx, tracks[0].ID)
	if got.PlayCount != 1 {
		t.Errorf("expected play_count=1 after batch re-upsert, got %d", got.PlayCount)
	}
	if got.Title != "A Updated" {
		t.Errorf("expected title updated, got %q", got.Title)
	}
}

func TestCleanupMissingTracks(t *testing.T) {
	t.Run("externally removed file is purged from DB", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		dir := t.TempDir()
		musicDir := filepath.Join(dir, "music")
		deleteDir := filepath.Join(musicDir, "to_delete")
		if err := os.MkdirAll(filepath.Join(musicDir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}

		s.SetMusicDir(musicDir)

		realFile := filepath.Join(musicDir, "sub", "exists.flac")
		if err := os.WriteFile(realFile, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}

		existsTrack := newTrack("sub/exists.flac", "Exists", "", "")
		existsTrack.MusicDir = musicDir
		if err := s.UpsertTrack(ctx, existsTrack); err != nil {
			t.Fatal(err)
		}
		goneTrack := newTrack("sub/gone.flac", "Gone", "", "")
		goneTrack.MusicDir = musicDir
		if err := s.UpsertTrack(ctx, goneTrack); err != nil {
			t.Fatal(err)
		}

		// Get the gone track's ID before cleanup
		tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
		var goneID int64
		for _, tr := range tracks {
			if tr.Path == "sub/gone.flac" {
				goneID = tr.ID
			}
		}

		existing := map[string]bool{"sub/exists.flac": true}
		// File not in to_delete either — should be purged
		softDeleted, purged, deletedPaths, purgedPaths, err := s.CleanupMissingTracks(ctx, existing, musicDir, deleteDir)
		if err != nil {
			t.Fatal(err)
		}
		if softDeleted != 0 {
			t.Errorf("expected 0 soft-deleted, got %d", softDeleted)
		}
		if purged != 1 {
			t.Errorf("expected 1 purged, got %d", purged)
		}
		if len(deletedPaths) != 0 {
			t.Errorf("expected no deleted paths, got %v", deletedPaths)
		}
		if len(purgedPaths) != 1 || purgedPaths[0] != "sub/gone.flac" {
			t.Errorf("expected purgedPaths=[sub/gone.flac], got %v", purgedPaths)
		}

		// Track should be completely gone from DB
		_, err = s.GetTrack(ctx, goneID)
		if !errors.Is(err, ErrTrackNotFound) {
			t.Errorf("expected ErrTrackNotFound for purged track, got %v", err)
		}
	})

	t.Run("user-rejected file in to_delete is soft-deleted", func(t *testing.T) {
		s := setupTestStore(t)
		ctx := context.Background()

		dir := t.TempDir()
		musicDir := filepath.Join(dir, "music")
		deleteDir := filepath.Join(musicDir, "to_delete")
		if err := os.MkdirAll(filepath.Join(musicDir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		// Create the file in to_delete (simulating user rejection)
		if err := os.MkdirAll(filepath.Join(deleteDir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(deleteDir, "sub", "gone.flac"), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}

		s.SetMusicDir(musicDir)

		realFile := filepath.Join(musicDir, "sub", "exists.flac")
		if err := os.WriteFile(realFile, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}

		existsTrack := newTrack("sub/exists.flac", "Exists", "", "")
		existsTrack.MusicDir = musicDir
		if err := s.UpsertTrack(ctx, existsTrack); err != nil {
			t.Fatal(err)
		}
		goneTrack := newTrack("sub/gone.flac", "Gone", "", "")
		goneTrack.MusicDir = musicDir
		if err := s.UpsertTrack(ctx, goneTrack); err != nil {
			t.Fatal(err)
		}

		// Get the gone track's ID before cleanup
		tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
		var goneID int64
		for _, tr := range tracks {
			if tr.Path == "sub/gone.flac" {
				goneID = tr.ID
			}
		}

		existing := map[string]bool{"sub/exists.flac": true}
		// File exists in to_delete — should be soft-deleted
		softDeleted, purged, deletedPaths, purgedPaths, err := s.CleanupMissingTracks(ctx, existing, musicDir, deleteDir)
		if err != nil {
			t.Fatal(err)
		}
		if softDeleted != 1 {
			t.Errorf("expected 1 soft-deleted, got %d", softDeleted)
		}
		if purged != 0 {
			t.Errorf("expected 0 purged, got %d", purged)
		}
		if len(deletedPaths) != 1 || deletedPaths[0] != "sub/gone.flac" {
			t.Errorf("expected deletedPaths=[sub/gone.flac], got %v", deletedPaths)
		}
		if len(purgedPaths) != 0 {
			t.Errorf("expected no purged paths, got %v", purgedPaths)
		}

		// Track should still exist in DB with deleted=1
		got, err := s.GetTrack(ctx, goneID)
		if err != nil {
			t.Fatalf("expected track to still exist in DB, got %v", err)
		}
		if !got.Deleted {
			t.Error("expected deleted=true for soft-deleted track")
		}
	})
}

