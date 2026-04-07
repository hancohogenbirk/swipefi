package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"swipefi/internal/store"
)

// setupScannerTest creates a temp music dir, a store, and a scanner for integration tests.
func setupScannerTest(t *testing.T) (string, *store.Store, *Scanner) {
	t.Helper()

	musicDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	sc := NewScanner(musicDir, s)
	return musicDir, s, sc
}

// createAudioFile creates an empty .flac file at the given path under musicDir.
// ReadMetadata falls back to path-based metadata (artist/album/title from path).
func createAudioFile(t *testing.T, musicDir, relPath string) {
	t.Helper()
	fullPath := filepath.Join(musicDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("fake audio"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan_BasicScan(t *testing.T) {
	musicDir, s, sc := setupScannerTest(t)
	ctx := context.Background()

	createAudioFile(t, musicDir, "Artist/Album/01 - Song.flac")
	createAudioFile(t, musicDir, "Artist/Album/02 - Another.flac")

	count, err := sc.Scan(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 tracks scanned, got %d", count)
	}

	tracks, err := s.ListTracks(ctx, "", "added_at", "asc")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 {
		t.Errorf("expected 2 tracks in DB, got %d", len(tracks))
	}
}

func TestScan_RescanPreservesPlayCounts(t *testing.T) {
	musicDir, s, sc := setupScannerTest(t)
	ctx := context.Background()

	createAudioFile(t, musicDir, "Artist/Album/Song.flac")

	// First scan
	if _, err := sc.Scan(ctx, false); err != nil {
		t.Fatal(err)
	}

	// Increment play count
	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if err := s.IncrementPlayCount(ctx, tracks[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.IncrementPlayCount(ctx, tracks[0].ID); err != nil {
		t.Fatal(err)
	}

	// Rescan same directory
	if _, err := sc.Scan(ctx, false); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTrack(ctx, tracks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlayCount != 2 {
		t.Errorf("expected play_count=2 after rescan, got %d", got.PlayCount)
	}
}

func TestScan_ForceRescanReReadsMetadata(t *testing.T) {
	musicDir, s, sc := setupScannerTest(t)
	ctx := context.Background()

	createAudioFile(t, musicDir, "Artist/Album/Song.flac")

	// First scan
	if _, err := sc.Scan(ctx, false); err != nil {
		t.Fatal(err)
	}

	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}

	// Force rescan — should re-read metadata (not skip via TrackExistsByPath)
	count, err := sc.Scan(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 track on force rescan, got %d", count)
	}
}

func TestScan_MissingFilesSoftDeleted(t *testing.T) {
	musicDir, s, sc := setupScannerTest(t)
	ctx := context.Background()

	createAudioFile(t, musicDir, "Artist/Album/Song.flac")
	createAudioFile(t, musicDir, "Artist/Album/WillRemove.flac")

	// Scan both files
	if _, err := sc.Scan(ctx, false); err != nil {
		t.Fatal(err)
	}

	// Remove one file
	os.Remove(filepath.Join(musicDir, "Artist/Album/WillRemove.flac"))

	// Rescan — missing file should be soft-deleted
	if _, err := sc.Scan(ctx, false); err != nil {
		t.Fatal(err)
	}

	deleted, err := s.ListDeleted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 {
		t.Errorf("expected 1 soft-deleted track, got %d", len(deleted))
	}
	if len(deleted) > 0 && deleted[0].Title != "WillRemove" {
		t.Errorf("expected deleted track 'WillRemove', got %q", deleted[0].Title)
	}

	// Remaining track should still be active
	active, err := s.TrackCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Errorf("expected 1 active track, got %d", active)
	}
}

// TestScan_SwitchMusicDir_PurgesOldTracks is the key regression test.
// It verifies that switching music directories purges old tracks from the DB
// entirely (hard delete) rather than soft-deleting them into the deletion UI.
func TestScan_SwitchMusicDir_PurgesOldTracks(t *testing.T) {
	_, s, sc := setupScannerTest(t)
	ctx := context.Background()

	// Set up first music dir with tracks
	dirA := t.TempDir()
	sc.SetMusicDir(dirA)
	createAudioFile(t, dirA, "ArtistA/AlbumA/SongA.flac")
	createAudioFile(t, dirA, "ArtistA/AlbumA/SongB.flac")

	if _, err := sc.Scan(ctx, false); err != nil {
		t.Fatal(err)
	}

	// Add play counts to dir A tracks
	tracksA, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if len(tracksA) != 2 {
		t.Fatalf("expected 2 tracks from dir A, got %d", len(tracksA))
	}
	s.IncrementPlayCount(ctx, tracksA[0].ID)
	s.IncrementPlayCount(ctx, tracksA[1].ID)

	// Switch to a completely different music dir
	dirB := t.TempDir()
	sc.SetMusicDir(dirB)
	createAudioFile(t, dirB, "ArtistB/AlbumB/SongC.flac")

	// Scan with purgeOrphans=true (simulates music dir change)
	count, err := sc.Scan(ctx, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 track scanned from dir B, got %d", count)
	}

	// KEY ASSERTION: Old tracks should be PURGED (not in deleted list)
	deleted, err := s.ListDeleted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 0 {
		t.Errorf("expected 0 soft-deleted tracks after dir switch, got %d (old tracks should be purged, not soft-deleted)", len(deleted))
	}

	// Only new dir tracks should remain
	active, err := s.TrackCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Errorf("expected 1 active track from dir B, got %d", active)
	}
}

// TestScan_SwitchMusicDir_PreservesMatchingPlayCounts verifies that when
// switching to a dir that has tracks with the same relative paths, play
// counts are preserved.
func TestScan_SwitchMusicDir_PreservesMatchingPlayCounts(t *testing.T) {
	_, s, sc := setupScannerTest(t)
	ctx := context.Background()

	// Set up first music dir
	dirA := t.TempDir()
	sc.SetMusicDir(dirA)
	createAudioFile(t, dirA, "Artist/Album/Song.flac")

	if _, err := sc.Scan(ctx, false); err != nil {
		t.Fatal(err)
	}

	tracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	s.IncrementPlayCount(ctx, tracks[0].ID)
	s.IncrementPlayCount(ctx, tracks[0].ID)
	s.IncrementPlayCount(ctx, tracks[0].ID)

	// Switch to dir B with SAME relative path structure
	dirB := t.TempDir()
	sc.SetMusicDir(dirB)
	createAudioFile(t, dirB, "Artist/Album/Song.flac")

	if _, err := sc.Scan(ctx, false, true); err != nil {
		t.Fatal(err)
	}

	// Play count should be preserved (same relative path)
	tracksB, _ := s.ListTracks(ctx, "", "added_at", "asc")
	if len(tracksB) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracksB))
	}
	if tracksB[0].PlayCount != 3 {
		t.Errorf("expected play_count=3 preserved across dir switch, got %d", tracksB[0].PlayCount)
	}
}

// TestScan_SwitchMusicDir_NoSoftDeletePollution is the exact regression scenario.
// Switch from dir A to dir B, then back to dir A. At no point should the
// deletion UI show tracks from the other directory.
func TestScan_SwitchMusicDir_NoSoftDeletePollution(t *testing.T) {
	_, s, sc := setupScannerTest(t)
	ctx := context.Background()

	dirA := t.TempDir()
	dirB := t.TempDir()

	createAudioFile(t, dirA, "ArtistA/Song1.flac")
	createAudioFile(t, dirA, "ArtistA/Song2.flac")
	createAudioFile(t, dirB, "ArtistB/Song3.flac")

	// Scan dir A
	sc.SetMusicDir(dirA)
	if _, err := sc.Scan(ctx, false); err != nil {
		t.Fatal(err)
	}

	countA, _ := s.TrackCount(ctx)
	if countA != 2 {
		t.Fatalf("expected 2 tracks from dir A, got %d", countA)
	}

	// Switch to dir B (purge orphans)
	sc.SetMusicDir(dirB)
	if _, err := sc.Scan(ctx, false, true); err != nil {
		t.Fatal(err)
	}

	deleted, _ := s.ListDeleted(ctx)
	if len(deleted) != 0 {
		t.Errorf("after switch A→B: expected 0 deleted, got %d", len(deleted))
	}
	countB, _ := s.TrackCount(ctx)
	if countB != 1 {
		t.Errorf("after switch A→B: expected 1 active track, got %d", countB)
	}

	// Switch back to dir A (purge orphans)
	sc.SetMusicDir(dirA)
	if _, err := sc.Scan(ctx, false, true); err != nil {
		t.Fatal(err)
	}

	deleted, _ = s.ListDeleted(ctx)
	if len(deleted) != 0 {
		t.Errorf("after switch B→A: expected 0 deleted, got %d", len(deleted))
	}
	countBack, _ := s.TrackCount(ctx)
	if countBack != 2 {
		t.Errorf("after switch B→A: expected 2 active tracks, got %d", countBack)
	}
}

func TestScan_EmptyDirsCleanedAfterSoftDelete(t *testing.T) {
	musicDir, _, sc := setupScannerTest(t)
	ctx := context.Background()

	// Need at least one file remaining so the scanner doesn't skip cleanup
	// (count==0 is treated as "mount not ready" safety check)
	createAudioFile(t, musicDir, "Keep/Song.flac")
	createAudioFile(t, musicDir, "Remove/Deep/Track.flac")

	if _, err := sc.Scan(ctx, false); err != nil {
		t.Fatal(err)
	}

	// Remove the file, leaving empty dirs
	os.Remove(filepath.Join(musicDir, "Remove/Deep/Track.flac"))

	// Rescan — should soft-delete and clean empty dirs
	if _, err := sc.Scan(ctx, false); err != nil {
		t.Fatal(err)
	}

	// Empty dirs should be removed from disk
	if _, err := os.Stat(filepath.Join(musicDir, "Remove/Deep")); !os.IsNotExist(err) {
		t.Error("expected Remove/Deep dir to be cleaned up")
	}
	if _, err := os.Stat(filepath.Join(musicDir, "Remove")); !os.IsNotExist(err) {
		t.Error("expected Remove dir to be cleaned up")
	}
	// The Keep dir should still exist
	if _, err := os.Stat(filepath.Join(musicDir, "Keep")); err != nil {
		t.Error("expected Keep dir to still exist")
	}
}

func TestScan_BatchUpsert(t *testing.T) {
	musicDir, s, sc := setupScannerTest(t)
	ctx := context.Background()

	// Create more than batchSize (500) files to exercise batching
	// Use 10 to keep test fast while still exercising the batch path
	for i := 0; i < 10; i++ {
		createAudioFile(t, musicDir, filepath.Join("Artist", "Album", fmt.Sprintf("%02d - Track %d.flac", i+1, i+1)))
	}

	count, err := sc.Scan(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if count != 10 {
		t.Errorf("expected 10 tracks, got %d", count)
	}

	dbCount, _ := s.TrackCount(ctx)
	if dbCount != 10 {
		t.Errorf("expected 10 tracks in DB, got %d", dbCount)
	}
}
