package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"swipefi/internal/store"
)

const testMusicDir = "/test/music"

func setupTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.SetMusicDir(testMusicDir)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNew_BinaryNotFound(t *testing.T) {
	s := setupTestStore(t)

	// When flacalyzer is not in PATH, analyzer should be created but not available
	orig := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir()) // empty dir — no binaries
	defer os.Setenv("PATH", orig)

	az := New(s)
	if az.Available() {
		t.Error("expected Available()=false when binary not found")
	}
}

func TestRun_DisabledWhenBinaryNotFound(t *testing.T) {
	s := setupTestStore(t)

	orig := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", orig)

	az := New(s)

	// Run should return nil silently (no error, no panic)
	if err := az.Run(context.Background(), testMusicDir); err != nil {
		t.Errorf("expected nil error when disabled, got %v", err)
	}
}

func TestRun_SkipsWhenNoTracksNeedAnalysis(t *testing.T) {
	s := setupTestStore(t)

	// Create a mock flacalyzer script that would fail if called
	mockBin := writeMockScript(t, `#!/bin/sh
echo "SHOULD NOT BE CALLED" >&2
exit 1
`)

	az := &Analyzer{binPath: mockBin, store: s}

	// No tracks in DB — Run should return nil without calling the binary
	if err := az.Run(context.Background(), testMusicDir); err != nil {
		t.Errorf("expected nil when no tracks need analysis, got %v", err)
	}
}

func TestRun_ParsesNDJSON(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Insert test tracks
	for _, path := range []string{"artist/album/clean.flac", "artist/album/bad.flac", "artist/album/suspect.flac"} {
		tr := &store.Track{
			Path:     path,
			Title:    filepath.Base(path),
			Format:   "flac",
			AddedAt:  1000,
			MusicDir: testMusicDir,
		}
		if err := s.UpsertTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	// Create mock flacalyzer that outputs NDJSON
	mockBin := writeMockScript(t, `#!/bin/sh
cat <<'NDJSON'
{"type":"file","path":"`+testMusicDir+`/artist/album/clean.flac","verdict":"lossless","confidence":0.0,"source_codec":null,"cutoff_khz":22.05,"sample_rate_hz":44100,"bit_depth":16}
{"type":"file","path":"`+testMusicDir+`/artist/album/bad.flac","verdict":"definitely_transcoded","confidence":0.92,"source_codec":"MP3 128kbps","cutoff_khz":16.0,"sample_rate_hz":44100,"bit_depth":16}
{"type":"file","path":"`+testMusicDir+`/artist/album/suspect.flac","verdict":"likely_transcoded","confidence":0.55,"source_codec":"Unknown lossy","cutoff_khz":19.5,"sample_rate_hz":44100,"bit_depth":16}
{"type":"summary","total_files":3,"valid_lossless":1,"definitely_transcoded":1,"likely_transcoded":1,"invalid":0,"elapsed_seconds":0.1}
NDJSON
`)

	az := &Analyzer{binPath: mockBin, store: s}
	if err := az.Run(ctx, testMusicDir); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify results stored in DB
	tracks, err := s.ListTracks(ctx, "", "added_at", "asc")
	if err != nil {
		t.Fatal(err)
	}

	results := make(map[string]store.Track)
	for _, tr := range tracks {
		results[tr.Path] = tr
	}

	// Clean track: score should be 0
	if clean := results["artist/album/clean.flac"]; clean.TranscodeScore != 0 {
		t.Errorf("clean track: expected score=0, got %f", clean.TranscodeScore)
	}

	// Definitely transcoded
	if bad := results["artist/album/bad.flac"]; bad.TranscodeScore != 0.92 {
		t.Errorf("bad track: expected score=0.92, got %f", bad.TranscodeScore)
	}
	if bad := results["artist/album/bad.flac"]; bad.TranscodeSource != "MP3 128kbps" {
		t.Errorf("bad track: expected source='MP3 128kbps', got %q", bad.TranscodeSource)
	}

	// Likely transcoded
	if suspect := results["artist/album/suspect.flac"]; suspect.TranscodeScore != 0.55 {
		t.Errorf("suspect track: expected score=0.55, got %f", suspect.TranscodeScore)
	}

	// No tracks should need analysis anymore
	needing, err := s.ListTracksNeedingAnalysis(ctx, testMusicDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(needing) != 0 {
		t.Errorf("expected 0 tracks needing analysis after Run, got %d", len(needing))
	}
}

func TestRun_CancelsOnContext(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Insert a track so Run doesn't skip
	tr := &store.Track{
		Path: "music/song.flac", Title: "Song", Format: "flac",
		AddedAt: 1000, MusicDir: testMusicDir,
	}
	if err := s.UpsertTrack(ctx, tr); err != nil {
		t.Fatal(err)
	}

	// Mock script that sleeps — we'll cancel before it finishes
	mockBin := writeMockScript(t, `#!/bin/sh
sleep 30
`)

	az := &Analyzer{binPath: mockBin, store: s}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // cancel immediately

	// Should return without error (context cancelled)
	err := az.Run(cancelCtx, testMusicDir)
	if err != nil {
		t.Errorf("expected nil on cancelled context, got %v", err)
	}
}

func TestMarkPending_SetsRunningState(t *testing.T) {
	s := setupTestStore(t)
	az := &Analyzer{store: s}

	st := az.GetStatus()
	if st.Running {
		t.Error("expected not running initially")
	}

	az.MarkPending()
	st = az.GetStatus()
	if !st.Running {
		t.Error("expected running after MarkPending")
	}
	if st.Analyzed != 0 || st.Total != 0 {
		t.Errorf("expected analyzed=0, total=0 after MarkPending, got %d/%d", st.Analyzed, st.Total)
	}
	if st.Error != "" {
		t.Errorf("expected empty error after MarkPending, got %q", st.Error)
	}
}

func TestCancel_ClearsRunningState(t *testing.T) {
	s := setupTestStore(t)
	az := &Analyzer{store: s}

	az.MarkPending()
	if !az.GetStatus().Running {
		t.Fatal("precondition: should be running")
	}

	az.Cancel()
	if az.GetStatus().Running {
		t.Error("expected not running after Cancel")
	}
}

func TestRun_ClearsRunningOnNoTracks(t *testing.T) {
	s := setupTestStore(t)
	mockBin := writeMockScript(t, "#!/bin/sh\nexit 1")
	az := &Analyzer{binPath: mockBin, store: s}

	// MarkPending then Run with no tracks — should clear running
	az.MarkPending()
	if err := az.Run(context.Background(), testMusicDir); err != nil {
		t.Fatal(err)
	}
	st := az.GetStatus()
	if st.Running {
		t.Error("expected not running after Run with 0 tracks")
	}
}

func TestRun_CapturesErrorOnFailure(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	tr := &store.Track{
		Path: "music/song.flac", Title: "Song", Format: "flac",
		AddedAt: 1000, MusicDir: testMusicDir,
	}
	if err := s.UpsertTrack(ctx, tr); err != nil {
		t.Fatal(err)
	}

	// Script that prints stderr and exits with error
	mockBin := writeMockScript(t, `#!/bin/sh
echo "something went wrong" >&2
exit 1
`)

	az := &Analyzer{binPath: mockBin, store: s}
	az.Run(ctx, testMusicDir)

	st := az.GetStatus()
	if st.Running {
		t.Error("expected not running after failed Run")
	}
	if st.Error == "" {
		t.Error("expected error message after failed Run")
	}
	if st.Error != "something went wrong" {
		t.Errorf("expected stderr in error, got %q", st.Error)
	}
}

func TestRun_TracksProgress(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Insert 2 tracks
	for _, path := range []string{"music/a.flac", "music/b.flac"} {
		tr := &store.Track{
			Path: path, Title: filepath.Base(path), Format: "flac",
			AddedAt: 1000, MusicDir: testMusicDir,
		}
		if err := s.UpsertTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	mockBin := writeMockScript(t, `#!/bin/sh
cat <<'NDJSON'
{"type":"file","path":"`+testMusicDir+`/music/a.flac","verdict":"lossless","confidence":0.0,"source_codec":null}
{"type":"file","path":"`+testMusicDir+`/music/b.flac","verdict":"definitely_transcoded","confidence":0.9,"source_codec":"MP3"}
NDJSON
`)

	az := &Analyzer{binPath: mockBin, store: s}
	if err := az.Run(ctx, testMusicDir); err != nil {
		t.Fatal(err)
	}

	st := az.GetStatus()
	if st.Running {
		t.Error("expected not running after completion")
	}
	if st.Total != 2 {
		t.Errorf("expected total=2, got %d", st.Total)
	}
	if st.Analyzed != 2 {
		t.Errorf("expected analyzed=2, got %d", st.Analyzed)
	}
	if st.Error != "" {
		t.Errorf("expected no error, got %q", st.Error)
	}
}

func TestCancel_StopsRunningAnalysis(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	tr := &store.Track{
		Path: "music/song.flac", Title: "Song", Format: "flac",
		AddedAt: 1000, MusicDir: testMusicDir,
	}
	if err := s.UpsertTrack(ctx, tr); err != nil {
		t.Fatal(err)
	}

	// Script that sleeps forever
	mockBin := writeMockScript(t, "#!/bin/sh\nsleep 60")
	az := &Analyzer{binPath: mockBin, store: s}

	done := make(chan struct{})
	go func() {
		az.Run(ctx, testMusicDir)
		close(done)
	}()

	// Give Run time to start the process
	for i := 0; i < 50; i++ {
		if az.GetStatus().Total > 0 {
			break
		}
		<-time.After(10 * time.Millisecond)
	}

	az.Cancel()
	<-done // Run should return promptly after Cancel

	st := az.GetStatus()
	if st.Running {
		t.Error("expected not running after Cancel")
	}
}

func TestRun_CallsOnTrackAnalyzed(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for _, path := range []string{"artist/album/track1.flac", "artist/album/track2.flac"} {
		tr := &store.Track{
			Path: path, Title: filepath.Base(path), Format: "flac",
			AddedAt: 1000, MusicDir: testMusicDir,
		}
		if err := s.UpsertTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}

	mockBin := writeMockScript(t, `#!/bin/sh
cat <<'NDJSON'
{"type":"file","path":"`+testMusicDir+`/artist/album/track1.flac","verdict":"definitely_transcoded","confidence":0.9,"source_codec":"MP3"}
{"type":"file","path":"`+testMusicDir+`/artist/album/track2.flac","verdict":"lossless","confidence":0.0,"source_codec":null}
{"type":"summary","total_files":2,"valid_lossless":1,"definitely_transcoded":1,"likely_transcoded":0,"invalid":0,"elapsed_seconds":0.1}
NDJSON
`)

	var called []int64
	var mu sync.Mutex

	az := &Analyzer{binPath: mockBin, store: s}
	az.OnTrackAnalyzed = func(trackID int64) {
		mu.Lock()
		called = append(called, trackID)
		mu.Unlock()
	}

	if err := az.Run(ctx, testMusicDir); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(called) != 2 {
		t.Fatalf("expected OnTrackAnalyzed called 2 times, got %d", len(called))
	}
}

// writeMockScript creates an executable shell script in a temp dir and returns its path.
func writeMockScript(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "flacalyzer")
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
