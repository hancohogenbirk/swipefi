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

func TestGetTrackByPath_Found(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	track := newTrack("artist/album/01-song.flac", "Song", "Artist", "Album")
	if err := s.UpsertTrack(ctx, track); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTrackByPath(ctx, "artist/album/01-song.flac")
	if err != nil {
		t.Fatalf("GetTrackByPath: %v", err)
	}
	if got.Title != "Song" || got.Artist != "Artist" || got.Album != "Album" {
		t.Errorf("unexpected track: %+v", got)
	}
}

func TestGetTrackByPath_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	_, err := s.GetTrackByPath(ctx, "nonexistent/path.flac")
	if !errors.Is(err, ErrTrackNotFound) {
		t.Errorf("expected ErrTrackNotFound, got %v", err)
	}
}

func TestGetTrackByPath_IgnoresDeleted(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	track := newTrack("artist/album/01-song.flac", "Song", "Artist", "Album")
	if err := s.UpsertTrack(ctx, track); err != nil {
		t.Fatal(err)
	}

	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if err := s.MarkDeleted(ctx, tracks[0].ID); err != nil {
		t.Fatal(err)
	}

	_, err := s.GetTrackByPath(ctx, "artist/album/01-song.flac")
	if !errors.Is(err, ErrTrackNotFound) {
		t.Errorf("expected ErrTrackNotFound for deleted track, got %v", err)
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

func TestTranscodeAnalysis_DefaultScore(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	if err := s.UpsertTrack(ctx, newTrack("music/song.flac", "Song", "Artist", "Album")); err != nil {
		t.Fatal(err)
	}

	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if len(tracks) == 0 {
		t.Fatal("expected 1 track")
	}

	// New tracks should have transcode_score = -1 (not yet analyzed)
	if tracks[0].TranscodeScore != -1 {
		t.Errorf("expected default transcode_score=-1, got %f", tracks[0].TranscodeScore)
	}
	if tracks[0].TranscodeSource != "" {
		t.Errorf("expected empty transcode_source, got %q", tracks[0].TranscodeSource)
	}
}

func TestUpdateTranscodeAnalysis(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	if err := s.UpsertTrack(ctx, newTrack("music/song.flac", "Song", "Artist", "Album")); err != nil {
		t.Fatal(err)
	}

	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	id := tracks[0].ID

	// Update transcode analysis results
	if err := s.UpdateTranscodeAnalysis(ctx, id, 0.85, "MP3 128kbps"); err != nil {
		t.Fatalf("UpdateTranscodeAnalysis: %v", err)
	}

	got, err := s.GetTrack(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.TranscodeScore != 0.85 {
		t.Errorf("expected transcode_score=0.85, got %f", got.TranscodeScore)
	}
	if got.TranscodeSource != "MP3 128kbps" {
		t.Errorf("expected transcode_source='MP3 128kbps', got %q", got.TranscodeSource)
	}
}

func TestListTracksNeedingAnalysis(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Insert 3 FLAC tracks and 1 MP3 track
	flac1 := newTrack("music/a.flac", "A", "", "")
	flac2 := newTrack("music/b.flac", "B", "", "")
	flac3 := newTrack("music/c.flac", "C", "", "")
	mp3 := newTrack("music/d.mp3", "D", "", "")
	mp3.Format = "mp3"

	for _, tr := range []*Track{flac1, flac2, flac3, mp3} {
		if err := s.UpsertTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	// All 3 FLACs should need analysis (score = -1)
	needing, err := s.ListTracksNeedingAnalysis(ctx, testMusicDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(needing) != 3 {
		t.Errorf("expected 3 tracks needing analysis, got %d", len(needing))
	}

	// Analyze one track
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	for _, tr := range tracks {
		if tr.Path == "music/a.flac" {
			if err := s.UpdateTranscodeAnalysis(ctx, tr.ID, 0.0, ""); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Now only 2 should need analysis
	needing, err = s.ListTracksNeedingAnalysis(ctx, testMusicDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(needing) != 2 {
		t.Errorf("expected 2 tracks needing analysis after one analyzed, got %d", len(needing))
	}
}

func TestUpsertTrack_PreservesTranscodeAnalysis(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	track := newTrack("music/song.flac", "Song", "Artist", "Album")
	if err := s.UpsertTrack(ctx, track); err != nil {
		t.Fatal(err)
	}

	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	id := tracks[0].ID

	// Set transcode analysis
	if err := s.UpdateTranscodeAnalysis(ctx, id, 0.92, "MP3 320kbps"); err != nil {
		t.Fatal(err)
	}

	// Re-upsert the same track with different metadata
	updated := newTrack("music/song.flac", "New Title", "New Artist", "New Album")
	if err := s.UpsertTrack(ctx, updated); err != nil {
		t.Fatal(err)
	}

	// Transcode analysis should be preserved
	got, err := s.GetTrack(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.TranscodeScore != 0.92 {
		t.Errorf("expected transcode_score=0.92 preserved after upsert, got %f", got.TranscodeScore)
	}
	if got.TranscodeSource != "MP3 320kbps" {
		t.Errorf("expected transcode_source preserved after upsert, got %q", got.TranscodeSource)
	}
	if got.Title != "New Title" {
		t.Errorf("expected title updated, got %q", got.Title)
	}
}

func TestTranscodeAnalysis_SchemaVersion(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	version, err := s.getSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version < 3 {
		t.Errorf("expected schema version >= 3 after migration, got %d", version)
	}
}

func TestListTracks_SortByLastPlayed(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for _, tr := range []*Track{
		newTrack("music/a.flac", "Never Played", "", ""),
		newTrack("music/b.flac", "Played Yesterday", "", ""),
		newTrack("music/c.flac", "Played Today", "", ""),
	} {
		if err := s.UpsertTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	now := time.Now().Unix()
	s.db.ExecContext(ctx, "UPDATE tracks SET last_played = ? WHERE id = ?", now-86400, tracks[1].ID)
	s.db.ExecContext(ctx, "UPDATE tracks SET last_played = ? WHERE id = ?", now, tracks[2].ID)

	// ASC: NULL first (never played), then oldest, then newest
	got, err := s.ListTracks(ctx, "", "last_played", "asc")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(got))
	}
	if got[0].Title != "Never Played" {
		t.Errorf("asc[0]: want 'Never Played' (NULL), got %q", got[0].Title)
	}
	if got[1].Title != "Played Yesterday" {
		t.Errorf("asc[1]: want 'Played Yesterday', got %q", got[1].Title)
	}
	if got[2].Title != "Played Today" {
		t.Errorf("asc[2]: want 'Played Today', got %q", got[2].Title)
	}

	// DESC: newest first, then oldest, then NULL last
	got, err = s.ListTracks(ctx, "", "last_played", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Title != "Played Today" {
		t.Errorf("desc[0]: want 'Played Today', got %q", got[0].Title)
	}
	if got[1].Title != "Played Yesterday" {
		t.Errorf("desc[1]: want 'Played Yesterday', got %q", got[1].Title)
	}
	if got[2].Title != "Never Played" {
		t.Errorf("desc[2]: want 'Never Played' (NULL), got %q", got[2].Title)
	}
}

func TestListTracksDirectOnly_SortByLastPlayed(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for _, tr := range []*Track{
		newTrack("music/a.flac", "A", "", ""),
		newTrack("music/b.flac", "B", "", ""),
	} {
		if err := s.UpsertTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	tracks, _ := s.ListTracksDirectOnly(ctx, "music", "added_at", "asc")
	now := time.Now().Unix()
	s.db.ExecContext(ctx, "UPDATE tracks SET last_played = ? WHERE id = ?", now, tracks[0].ID)

	got, err := s.ListTracksDirectOnly(ctx, "music", "last_played", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(got))
	}
	if got[0].Title != "A" {
		t.Errorf("expected 'A' first (most recently played), got %q", got[0].Title)
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

func TestResetTranscodeScores(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Insert FLAC tracks and an MP3
	flac1 := newTrack("music/a.flac", "A", "", "")
	flac2 := newTrack("music/b.flac", "B", "", "")
	mp3 := newTrack("music/c.mp3", "C", "", "")
	mp3.Format = "mp3"

	for _, tr := range []*Track{flac1, flac2, mp3} {
		if err := s.UpsertTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	// Analyze all tracks
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	for _, tr := range tracks {
		if err := s.UpdateTranscodeAnalysis(ctx, tr.ID, 0.85, "MP3 128kbps"); err != nil {
			t.Fatal(err)
		}
	}

	// Verify none need analysis
	needing, _ := s.ListTracksNeedingAnalysis(ctx, testMusicDir)
	if len(needing) != 0 {
		t.Fatalf("expected 0 tracks needing analysis, got %d", len(needing))
	}

	// Reset scores
	if err := s.ResetTranscodeScores(ctx, testMusicDir); err != nil {
		t.Fatalf("ResetTranscodeScores: %v", err)
	}

	// Only FLAC tracks should need analysis again (not MP3)
	needing, _ = s.ListTracksNeedingAnalysis(ctx, testMusicDir)
	if len(needing) != 2 {
		t.Errorf("expected 2 FLAC tracks needing analysis after reset, got %d", len(needing))
	}

	// Verify MP3 score was NOT reset
	tracks, _ = s.ListTracks(ctx, "", "added_at", "asc")
	for _, tr := range tracks {
		if tr.Format == "mp3" && tr.TranscodeScore != 0.85 {
			t.Errorf("MP3 score should be unchanged, got %f", tr.TranscodeScore)
		}
	}
}

func TestResetTranscodeScores_FiltersByMusicDir(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Insert a track with the default music dir
	flac := newTrack("music/a.flac", "A", "", "")
	if err := s.UpsertTrack(ctx, flac); err != nil {
		t.Fatal(err)
	}
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if err := s.UpdateTranscodeAnalysis(ctx, tracks[0].ID, 0.9, "MP3"); err != nil {
		t.Fatal(err)
	}

	// Reset with a DIFFERENT music dir — should not affect our track
	if err := s.ResetTranscodeScores(ctx, "/other/dir"); err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetTrack(ctx, tracks[0].ID)
	if got.TranscodeScore != 0.9 {
		t.Errorf("score should be unchanged for different musicDir, got %f", got.TranscodeScore)
	}
}

func TestResetTranscodeScores_SkipsDeleted(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	flac := newTrack("music/a.flac", "A", "", "")
	if err := s.UpsertTrack(ctx, flac); err != nil {
		t.Fatal(err)
	}
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	id := tracks[0].ID

	if err := s.UpdateTranscodeAnalysis(ctx, id, 0.9, "MP3"); err != nil {
		t.Fatal(err)
	}
	// Soft-delete the track
	if err := s.MarkDeleted(ctx, id); err != nil {
		t.Fatal(err)
	}

	// Reset should not affect deleted tracks
	if err := s.ResetTranscodeScores(ctx, testMusicDir); err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetTrack(ctx, id)
	if got.TranscodeScore != 0.9 {
		t.Errorf("deleted track score should be unchanged, got %f", got.TranscodeScore)
	}
}

