package player

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"swipefi/internal/dlna"
	"swipefi/internal/store"
)

// mockTransport implements dlna.Transporter for testing.
type mockTransport struct {
	mu          sync.Mutex
	state       dlna.TransportState
	uri         string
	position    time.Duration
	duration    time.Duration
	playCalls   int
	stopCalls   int
	setURICalls int
	seekCalls   int
	seekTargets []time.Duration
	// callOrder records the sequence of transport calls (e.g. "Play", "Seek")
	// so tests can assert relative ordering.
	callOrder      []string
	playErr        error
	stopErr        error
	getStateErr    error
	getPositionErr error
	// When true, SetURI/Play/Stop check ctx.Err() first and return it if
	// non-nil. This catches bugs where a cancelled context is passed.
	checkCtx bool
}

func newMockTransport() *mockTransport {
	return &mockTransport{state: dlna.StateStopped}
}

func (m *mockTransport) SetURI(ctx context.Context, uri, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.checkCtx {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	m.setURICalls++
	m.uri = uri
	return nil
}

func (m *mockTransport) Play(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.checkCtx {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	m.playCalls++
	m.callOrder = append(m.callOrder, "Play")
	if m.playErr != nil {
		return m.playErr
	}
	m.state = dlna.StatePlaying
	return nil
}

func (m *mockTransport) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.checkCtx {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	m.stopCalls++
	m.callOrder = append(m.callOrder, "Stop")
	if m.stopErr != nil {
		return m.stopErr
	}
	m.state = dlna.StateStopped
	return nil
}

func (m *mockTransport) Pause(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = dlna.StatePaused
	return nil
}

func (m *mockTransport) Seek(_ context.Context, target time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seekCalls++
	m.seekTargets = append(m.seekTargets, target)
	m.callOrder = append(m.callOrder, "Seek")
	return nil
}

func (m *mockTransport) GetState(_ context.Context) (dlna.TransportState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getStateErr != nil {
		return "", m.getStateErr
	}
	return m.state, nil
}

func (m *mockTransport) GetPosition(_ context.Context) (*dlna.PositionInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getPositionErr != nil {
		return nil, m.getPositionErr
	}
	return &dlna.PositionInfo{
		TrackDuration: m.duration,
		RelTime:       m.position,
		TrackURI:      m.uri,
	}, nil
}

func (m *mockTransport) setState(s dlna.TransportState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = s
}

func (m *mockTransport) setPosition(pos, dur time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.position = pos
	m.duration = dur
}

// setupTestPlayer creates a Player with a mock transport and test queue.
func setupTestPlayer(t *testing.T, tracks []store.Track) (*Player, *mockTransport) {
	t.Helper()

	// Create a minimal test store (in-memory SQLite)
	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)

	if len(tracks) > 0 {
		p.mu.Lock()
		p.queue = NewQueue(tracks)
		p.mu.Unlock()
	}

	return p, mt
}

func testTracks() []store.Track {
	return []store.Track{
		{ID: 1, Path: "artist/album/01-song1.flac", Title: "Song 1", Artist: "Artist", Format: "flac"},
		{ID: 2, Path: "artist/album/02-song2.flac", Title: "Song 2", Artist: "Artist", Format: "flac"},
		{ID: 3, Path: "artist/album/03-song3.flac", Title: "Song 3", Artist: "Artist", Format: "flac"},
	}
}

func TestPollOnce_IgnoresStoppedDuringGracePeriod(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Simulate: playCurrentLocked just called, renderer still STOPPED
	p.mu.Lock()
	p.state = StateLoading
	p.playStartedAt = time.Now()
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.mu.Unlock()

	// Transport reports STOPPED (hasn't started yet) with zero duration
	mt.setState(dlna.StateStopped)
	mt.setPosition(0, 0)

	p.pollOnce(ctx)

	p.mu.Lock()
	state := p.state
	pos := p.queue.Position()
	p.mu.Unlock()

	// Should still be loading, not skipped to next track
	if state != StateLoading {
		t.Errorf("expected StateLoading, got %s", state)
	}
	if pos != 0 {
		t.Errorf("expected queue position 0, got %d", pos)
	}
}

func TestPollOnce_TransitionsLoadingToPlaying(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	p.mu.Lock()
	p.state = StateLoading
	p.playStartedAt = time.Now()
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.mu.Unlock()

	// Renderer starts playing
	mt.setState(dlna.StatePlaying)
	mt.setPosition(500*time.Millisecond, 3*time.Minute)

	p.pollOnce(ctx)

	p.mu.Lock()
	state := p.state
	p.mu.Unlock()

	if state != StatePlaying {
		t.Errorf("expected StatePlaying, got %s", state)
	}
}

func TestPollOnce_StoppedAfterGracePeriodAdvancesQueue(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Simulate: track has been playing, now naturally stopped
	p.mu.Lock()
	p.state = StatePlaying
	p.playStartedAt = time.Now().Add(-10 * time.Second) // well past grace period
	p.durationMs = 180000                                // 3 minutes
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.mu.Unlock()

	mt.setState(dlna.StateStopped)
	mt.setPosition(0, 3*time.Minute)

	p.pollOnce(ctx)

	p.mu.Lock()
	pos := p.queue.Position()
	p.mu.Unlock()

	// Should have advanced to next track
	if pos != 1 {
		t.Errorf("expected queue position 1, got %d", pos)
	}
}

func TestPlayCurrentLocked_StopsCurrentBeforeNewTrack(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Simulate: already playing track 1
	p.mu.Lock()
	p.state = StatePlaying
	p.currentStreamURL = "http://192.168.1.1:8080/stream/old"
	p.mu.Unlock()

	mt.setState(dlna.StatePlaying)

	// Play current (track 1) — should Stop first
	p.mu.Lock()
	err := p.playCurrentLocked(ctx)
	p.mu.Unlock()

	if err != nil {
		t.Fatalf("playCurrentLocked: %v", err)
	}

	mt.mu.Lock()
	stops := mt.stopCalls
	mt.mu.Unlock()

	if stops < 1 {
		t.Errorf("expected at least 1 Stop call, got %d", stops)
	}
}

func TestPollOnce_NaturalEndStartsNextTrack(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Enable context checking — transport methods will fail if they
	// receive a cancelled context. This catches the auto-advance
	// regression where playCurrentLocked was called with the poll
	// context which gets cancelled by stopPollingLocked.
	mt.mu.Lock()
	mt.checkCtx = true
	mt.mu.Unlock()

	// Simulate: track 1 has been playing for a while, now stopped naturally.
	p.mu.Lock()
	p.state = StatePlaying
	p.playStartedAt = time.Now().Add(-10 * time.Second) // well past grace period
	p.durationMs = 180000                                // 3 minutes
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.mu.Unlock()

	// Renderer reports STOPPED with position 0, duration 3 min — track ended.
	mt.setState(dlna.StateStopped)
	mt.setPosition(0, 3*time.Minute)

	// Record play calls before pollOnce.
	mt.mu.Lock()
	callsBefore := mt.playCalls
	mt.mu.Unlock()

	p.pollOnce(ctx)

	p.mu.Lock()
	pos := p.queue.Position()
	state := p.state
	p.mu.Unlock()

	// Queue should have advanced to track 2 (position 1).
	if pos != 1 {
		t.Errorf("expected queue position 1, got %d", pos)
	}

	// The new track should be loading or playing (not idle).
	if state == StateIdle {
		t.Errorf("expected non-idle state after auto-advance, got %s", state)
	}

	// Transport.Play should have been called for the new track (at least 1
	// call from tryPlayWithRetry). With checkCtx enabled, this would be 0
	// if a cancelled context was passed — the core regression being tested.
	mt.mu.Lock()
	newCalls := mt.playCalls - callsBefore
	mt.mu.Unlock()

	if newCalls < 1 {
		t.Errorf("expected at least 1 new Play call for next track, got %d", newCalls)
	}
}

func TestHeartbeatDetectsDisconnect(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)

	// Stop the background poll loop so we control timing
	p.mu.Lock()
	p.stopPollingLocked()
	p.state = StateIdle // idle but connected — heartbeat territory
	p.mu.Unlock()

	// Record state changes via onChange
	var lastState PlayerState
	var stateMu sync.Mutex
	p.SetOnChange(func(ps PlayerState) {
		stateMu.Lock()
		lastState = ps
		stateMu.Unlock()
	})

	// Make GetState fail
	mt.mu.Lock()
	mt.getStateErr = fmt.Errorf("connection refused")
	mt.mu.Unlock()

	// Simulate: first error happened 31 seconds ago (past the 30s threshold)
	p.mu.Lock()
	p.firstPollErrorAt = time.Now().Add(-31 * time.Second)
	p.mu.Unlock()

	// Single poll should trigger disconnect since we're past the threshold
	p.pollOnce(ctx)

	p.mu.Lock()
	transport := p.transport
	state := p.state
	p.mu.Unlock()

	if transport != nil {
		t.Error("expected transport to be nil after exceeding 30s error threshold")
	}
	if state != StateIdle {
		t.Errorf("expected StateIdle, got %s", state)
	}

	stateMu.Lock()
	connected := lastState.Connected
	stateMu.Unlock()

	if connected {
		t.Error("expected last broadcast to have connected=false")
	}
}

func TestHeartbeatDoesNotDisconnect_WithinTimeWindow(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)

	p.mu.Lock()
	p.stopPollingLocked()
	p.state = StateIdle
	p.mu.Unlock()

	// Make GetState fail
	mt.mu.Lock()
	mt.getStateErr = fmt.Errorf("connection refused")
	mt.mu.Unlock()

	// Poll many times rapidly — all within 30s window, should NOT disconnect
	for i := 0; i < 50; i++ {
		p.pollOnce(ctx)
	}

	p.mu.Lock()
	transport := p.transport
	state := p.state
	p.mu.Unlock()

	if transport == nil {
		t.Error("expected transport to still be set — errors within 30s window")
	}
	if state != StateIdle {
		t.Errorf("expected StateIdle, got %s", state)
	}
}

func TestHeartbeatResetsOnSuccess(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)

	// Stop background poll loop
	p.mu.Lock()
	p.stopPollingLocked()
	p.state = StateIdle
	p.mu.Unlock()

	// Fail twice
	mt.mu.Lock()
	mt.getStateErr = fmt.Errorf("timeout")
	mt.mu.Unlock()

	p.pollOnce(ctx)
	p.pollOnce(ctx)

	// Succeed — should reset error timer
	mt.mu.Lock()
	mt.getStateErr = nil
	mt.mu.Unlock()

	p.pollOnce(ctx)

	p.mu.Lock()
	errorReset := p.firstPollErrorAt.IsZero()
	transport := p.transport
	p.mu.Unlock()

	if !errorReset {
		t.Error("expected firstPollErrorAt to be zero after success")
	}
	if transport == nil {
		t.Error("expected transport to still be set after error recovery")
	}
}

func TestRecoverRendererState_PicksUpPlayingTrack(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetMusicDir("/tmp/music")

	// Insert a track into the DB
	if err := s.UpsertTrack(ctx, &store.Track{
		Path: "artist/album/01-song.flac", Title: "Song 1", Artist: "Artist",
		Album: "Album", DurationMs: 180000, Format: "flac", AddedAt: 1,
		MusicDir: "/tmp/music",
	}); err != nil {
		t.Fatal(err)
	}

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	// Simulate renderer playing a known track
	streamURL := fmt.Sprintf("http://%s:8080/stream/artist/album/01-song.flac", p.localIP)
	mt.mu.Lock()
	mt.uri = streamURL
	mt.state = dlna.StatePlaying
	mt.position = 45 * time.Second
	mt.duration = 3 * time.Minute
	mt.mu.Unlock()

	// SetTransport triggers recoverRendererState
	p.SetTransport(mt)

	// Give the async recovery a moment to complete
	time.Sleep(200 * time.Millisecond)

	p.mu.Lock()
	state := p.state
	track := p.queue.Current() // should be non-nil
	posMs := p.positionMs
	durMs := p.durationMs
	streamURLSet := p.currentStreamURL
	p.mu.Unlock()

	if state != StatePlaying {
		t.Errorf("expected StatePlaying, got %s", state)
	}
	if track == nil {
		t.Fatal("expected a track in the queue after recovery")
	}
	if track.Title != "Song 1" {
		t.Errorf("expected Song 1, got %s", track.Title)
	}
	if posMs != 45000 {
		t.Errorf("expected position 45000ms, got %d", posMs)
	}
	if durMs != 180000 {
		t.Errorf("expected duration 180000ms, got %d", durMs)
	}
	if streamURLSet != streamURL {
		t.Errorf("expected currentStreamURL to be set, got %q", streamURLSet)
	}
}

func TestRecoverRendererState_IgnoresUnknownTrack(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetMusicDir("/tmp/music")

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	// Renderer playing something not in our DB
	mt.mu.Lock()
	mt.uri = "http://someother.device/music/unknown.flac"
	mt.state = dlna.StatePlaying
	mt.position = 10 * time.Second
	mt.duration = 2 * time.Minute
	mt.mu.Unlock()

	p.SetTransport(mt)
	time.Sleep(200 * time.Millisecond)

	p.mu.Lock()
	state := p.state
	hasQueue := p.queue != nil
	p.mu.Unlock()

	// Should remain idle — can't recover an unknown track
	if state != StateIdle {
		t.Errorf("expected StateIdle for unknown track, got %s", state)
	}
	if hasQueue {
		t.Error("expected no queue for unknown track")
	}
}

func TestRecoverRendererState_PausedTrack(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetMusicDir("/tmp/music")

	if err := s.UpsertTrack(ctx, &store.Track{
		Path: "artist/album/02-song.flac", Title: "Song 2", Artist: "Artist",
		Album: "Album", DurationMs: 240000, Format: "flac", AddedAt: 1,
		MusicDir: "/tmp/music",
	}); err != nil {
		t.Fatal(err)
	}

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	streamURL := fmt.Sprintf("http://%s:8080/stream/artist/album/02-song.flac", p.localIP)
	mt.mu.Lock()
	mt.uri = streamURL
	mt.state = dlna.StatePaused
	mt.position = 90 * time.Second
	mt.duration = 4 * time.Minute
	mt.mu.Unlock()

	p.SetTransport(mt)
	time.Sleep(200 * time.Millisecond)

	p.mu.Lock()
	state := p.state
	track := p.queue.Current()
	p.mu.Unlock()

	if state != StatePaused {
		t.Errorf("expected StatePaused, got %s", state)
	}
	if track == nil || track.Title != "Song 2" {
		t.Errorf("expected Song 2, got %v", track)
	}
}

func TestRecoverRendererState_BuildsFolderQueue(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetMusicDir("/tmp/music")

	// Insert 3 tracks in the same folder
	for i, name := range []string{"01-song.flac", "02-song.flac", "03-song.flac"} {
		if err := s.UpsertTrack(ctx, &store.Track{
			Path: "artist/album/" + name, Title: fmt.Sprintf("Song %d", i+1),
			Artist: "Artist", Album: "Album", DurationMs: 180000, Format: "flac",
			AddedAt: int64(i + 1), MusicDir: "/tmp/music",
		}); err != nil {
			t.Fatal(err)
		}
	}

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	// Renderer is playing the second track in the folder
	streamURL := fmt.Sprintf("http://%s:8080/stream/artist/album/02-song.flac", p.localIP)
	mt.mu.Lock()
	mt.uri = streamURL
	mt.state = dlna.StatePlaying
	mt.position = 30 * time.Second
	mt.duration = 3 * time.Minute
	mt.mu.Unlock()

	p.SetTransport(mt)
	time.Sleep(200 * time.Millisecond)

	p.mu.Lock()
	queueLen := 0
	queuePos := 0
	var currentTitle string
	if p.queue != nil {
		queueLen = p.queue.Len()
		queuePos = p.queue.Position()
		if cur := p.queue.Current(); cur != nil {
			currentTitle = cur.Title
		}
	}
	p.mu.Unlock()

	// Should have all 3 tracks in the queue, positioned at track 2 (index 1)
	if queueLen != 3 {
		t.Errorf("expected queue length 3, got %d", queueLen)
	}
	if queuePos != 1 {
		t.Errorf("expected queue position 1 (second track), got %d", queuePos)
	}
	if currentTitle != "Song 2" {
		t.Errorf("expected current track 'Song 2', got %q", currentTitle)
	}
}

func TestRecoverRendererState_SkipsWhenQueueExists(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetMusicDir("/tmp/music")

	if err := s.UpsertTrack(ctx, &store.Track{
		Path: "artist/album/01-song.flac", Title: "Song 1", Artist: "Artist",
		Album: "Album", DurationMs: 180000, Format: "flac", AddedAt: 1,
		MusicDir: "/tmp/music",
	}); err != nil {
		t.Fatal(err)
	}

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	// Pre-set a queue (simulating the user was already playing)
	existingTracks := []store.Track{
		{ID: 99, Path: "other/track.flac", Title: "Existing Track"},
	}
	p.mu.Lock()
	p.queue = NewQueue(existingTracks)
	p.state = StatePlaying
	p.mu.Unlock()

	mt := newMockTransport()
	streamURL := fmt.Sprintf("http://%s:8080/stream/artist/album/01-song.flac", p.localIP)
	mt.mu.Lock()
	mt.uri = streamURL
	mt.state = dlna.StatePlaying
	mt.position = 45 * time.Second
	mt.duration = 3 * time.Minute
	mt.mu.Unlock()

	p.SetTransport(mt)
	time.Sleep(200 * time.Millisecond)

	p.mu.Lock()
	currentTitle := ""
	if p.queue != nil && p.queue.Current() != nil {
		currentTitle = p.queue.Current().Title
	}
	p.mu.Unlock()

	// Should NOT have replaced the existing queue
	if currentTitle != "Existing Track" {
		t.Errorf("expected existing queue to be preserved, got current track %q", currentTitle)
	}
}

func TestRecoverRendererState_StoppedNoRecovery(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetMusicDir("/tmp/music")

	if err := s.UpsertTrack(ctx, &store.Track{
		Path: "artist/album/01-song.flac", Title: "Song 1", Artist: "Artist",
		Album: "Album", DurationMs: 180000, Format: "flac", AddedAt: 1,
		MusicDir: "/tmp/music",
	}); err != nil {
		t.Fatal(err)
	}

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	// Renderer is STOPPED — nothing to recover
	mt.mu.Lock()
	mt.uri = ""
	mt.state = dlna.StateStopped
	mt.position = 0
	mt.duration = 0
	mt.mu.Unlock()

	p.SetTransport(mt)
	time.Sleep(200 * time.Millisecond)

	p.mu.Lock()
	state := p.state
	hasQueue := p.queue != nil
	p.mu.Unlock()

	if state != StateIdle {
		t.Errorf("expected StateIdle for stopped renderer, got %s", state)
	}
	if hasQueue {
		t.Error("expected no queue when renderer is stopped")
	}
}

func TestExtractTrackPath(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		localIP string
		port    string
		want    string
	}{
		{
			name:    "valid stream URL",
			uri:     "http://192.168.1.1:8080/stream/artist/album/01-song.flac",
			localIP: "192.168.1.1",
			port:    "8080",
			want:    "artist/album/01-song.flac",
		},
		{
			name:    "URL with spaces encoded",
			uri:     "http://10.0.0.5:9090/stream/My%20Artist/album/song.flac",
			localIP: "10.0.0.5",
			port:    "9090",
			want:    "My Artist/album/song.flac",
		},
		{
			name:    "external URL not matching our prefix",
			uri:     "http://other-server.local/music/song.mp3",
			localIP: "192.168.1.1",
			port:    "8080",
			want:    "",
		},
		{
			name:    "empty URI",
			uri:     "",
			localIP: "192.168.1.1",
			port:    "8080",
			want:    "",
		},
		{
			name:    "wrong port",
			uri:     "http://192.168.1.1:9999/stream/artist/song.flac",
			localIP: "192.168.1.1",
			port:    "8080",
			want:    "",
		},
		{
			name:    "wrong IP",
			uri:     "http://10.0.0.1:8080/stream/artist/song.flac",
			localIP: "192.168.1.1",
			port:    "8080",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTrackPath(tt.uri, tt.localIP, tt.port)
			if got != tt.want {
				t.Errorf("extractTrackPath(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}

func TestPollOnce_IgnoresPositionDuringLoading(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Simulate: loading state, renderer reports stale position from previous track
	p.mu.Lock()
	p.state = StateLoading
	p.playStartedAt = time.Now()
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.positionMs = 0
	p.durationMs = 0
	p.mu.Unlock()

	// Renderer still reports old track position (stale data during transition)
	mt.setState(dlna.StateStopped)
	mt.setPosition(45*time.Second, 3*time.Minute)

	p.pollOnce(ctx)

	p.mu.Lock()
	posMs := p.positionMs
	durMs := p.durationMs
	p.mu.Unlock()

	if posMs != 0 {
		t.Errorf("expected positionMs=0 during loading, got %d", posMs)
	}
	if durMs != 0 {
		t.Errorf("expected durationMs=0 during loading, got %d", durMs)
	}
}

func TestReject_CancelledContextStillPlaysNext(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())

	// Enable context checking
	mt.mu.Lock()
	mt.checkCtx = true
	mt.mu.Unlock()

	// Simulate playing track 1
	p.mu.Lock()
	p.state = StatePlaying
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.mu.Unlock()

	// Create a real music dir with the test file so Reject's os.Rename doesn't fail
	musicDir := t.TempDir()
	deleteDir := t.TempDir()
	p.mu.Lock()
	p.musicDir = musicDir
	p.deleteDir = deleteDir
	p.mu.Unlock()

	// Create the source file
	trackDir := filepath.Join(musicDir, "artist", "album")
	os.MkdirAll(trackDir, 0755)
	os.WriteFile(filepath.Join(trackDir, "01-song1.flac"), []byte("fake"), 0644)

	// Use a context that gets cancelled quickly (simulating HTTP timeout)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay (simulating request completion)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := p.Reject(ctx)
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}

	// Give transport calls time to complete
	time.Sleep(200 * time.Millisecond)

	p.mu.Lock()
	pos := p.queue.Position()
	state := p.state
	p.mu.Unlock()

	// Should have advanced to next track (not stuck)
	if state == StateIdle && pos == 0 {
		t.Error("expected non-idle state with advanced queue position after reject")
	}
}

func TestReject_CancelledContextStopsWhenQueueEmpty(t *testing.T) {
	// Single-track queue: after reject, queue is empty and Stop must succeed
	singleTrack := []store.Track{
		{ID: 1, Path: "artist/album/01-song1.flac", Title: "Song 1", Artist: "Artist", Format: "flac"},
	}
	p, mt := setupTestPlayer(t, singleTrack)

	// Enable context checking — Stop will fail if it receives a cancelled ctx
	mt.mu.Lock()
	mt.checkCtx = true
	mt.mu.Unlock()

	// Simulate playing track 1
	p.mu.Lock()
	p.state = StatePlaying
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.mu.Unlock()

	// Create a real music dir with the test file
	musicDir := t.TempDir()
	deleteDir := t.TempDir()
	p.mu.Lock()
	p.musicDir = musicDir
	p.deleteDir = deleteDir
	p.mu.Unlock()

	trackDir := filepath.Join(musicDir, "artist", "album")
	os.MkdirAll(trackDir, 0755)
	os.WriteFile(filepath.Join(trackDir, "01-song1.flac"), []byte("fake"), 0644)

	// Use an already-cancelled context (simulating HTTP timeout)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Reject(ctx)
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}

	// Stop should have been called successfully despite cancelled ctx
	mt.mu.Lock()
	stops := mt.stopCalls
	mt.mu.Unlock()

	if stops < 1 {
		t.Errorf("expected at least 1 Stop call, got %d", stops)
	}

	p.mu.Lock()
	state := p.state
	p.mu.Unlock()

	if state != StateIdle {
		t.Errorf("expected StateIdle after rejecting last track, got %s", state)
	}
}

func TestSetTransportStartsPolling(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	// Before SetTransport: no polling
	p.mu.Lock()
	hasCancel := p.pollCancel != nil
	p.mu.Unlock()

	if hasCancel {
		t.Error("expected no pollCancel before SetTransport")
	}

	// SetTransport with non-nil transport should start polling
	mt := newMockTransport()
	p.SetTransport(mt)

	p.mu.Lock()
	hasCancel = p.pollCancel != nil
	p.mu.Unlock()

	if !hasCancel {
		t.Error("expected pollCancel to be set after SetTransport")
	}

	// SetTransport(nil) should stop polling
	p.SetTransport(nil)

	p.mu.Lock()
	hasCancel = p.pollCancel != nil
	p.mu.Unlock()

	if hasCancel {
		t.Error("expected pollCancel to be nil after SetTransport(nil)")
	}
}

func TestPollOnce_IncrementsPlayCountAfter60Seconds(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Track state changes
	var lastState PlayerState
	var stateMu sync.Mutex
	p.SetOnChange(func(ps PlayerState) {
		stateMu.Lock()
		lastState = ps
		stateMu.Unlock()
	})

	// Simulate: playing for 61 seconds
	p.mu.Lock()
	p.state = StatePlaying
	p.playStartedAt = time.Now().Add(-10 * time.Second) // past grace period
	p.playStartTime = time.Now().Add(-61 * time.Second)  // 61s of play time
	p.accumulatedMs = 0
	p.playCounted = false
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	initialPlayCount := p.queue.Current().PlayCount
	p.mu.Unlock()

	// Renderer reports playing
	mt.setState(dlna.StatePlaying)
	mt.setPosition(61*time.Second, 3*time.Minute)

	p.pollOnce(ctx)

	p.mu.Lock()
	counted := p.playCounted
	currentTrack := p.queue.Current()
	p.mu.Unlock()

	if !counted {
		t.Error("expected playCounted=true after 61 seconds")
	}
	if currentTrack.PlayCount != initialPlayCount+1 {
		t.Errorf("expected play_count=%d, got %d", initialPlayCount+1, currentTrack.PlayCount)
	}

	// Verify notify was called (broadcast happened)
	stateMu.Lock()
	broadcastTrack := lastState.Track
	stateMu.Unlock()

	if broadcastTrack == nil {
		t.Error("expected broadcast with track after playcount increment")
	} else if broadcastTrack.PlayCount != initialPlayCount+1 {
		t.Errorf("expected broadcast play_count=%d, got %d", initialPlayCount+1, broadcastTrack.PlayCount)
	}
}

func TestPollOnce_DoesNotIncrementPlayCountBefore60Seconds(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Simulate: playing for only 30 seconds
	p.mu.Lock()
	p.state = StatePlaying
	p.playStartedAt = time.Now().Add(-10 * time.Second)
	p.playStartTime = time.Now().Add(-30 * time.Second) // only 30s
	p.accumulatedMs = 0
	p.playCounted = false
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	initialPlayCount := p.queue.Current().PlayCount
	p.mu.Unlock()

	mt.setState(dlna.StatePlaying)
	mt.setPosition(30*time.Second, 3*time.Minute)

	p.pollOnce(ctx)

	p.mu.Lock()
	counted := p.playCounted
	currentTrack := p.queue.Current()
	p.mu.Unlock()

	if counted {
		t.Error("expected playCounted=false before 60 seconds")
	}
	if currentTrack.PlayCount != initialPlayCount {
		t.Errorf("expected play_count=%d (unchanged), got %d", initialPlayCount, currentTrack.PlayCount)
	}
}

func TestPollOnce_PlayCountOnlyIncrementsOnce(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Simulate: playing for 120 seconds
	p.mu.Lock()
	p.state = StatePlaying
	p.playStartedAt = time.Now().Add(-10 * time.Second)
	p.playStartTime = time.Now().Add(-120 * time.Second)
	p.accumulatedMs = 0
	p.playCounted = false
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.mu.Unlock()

	mt.setState(dlna.StatePlaying)
	mt.setPosition(120*time.Second, 3*time.Minute)

	// Poll twice
	p.pollOnce(ctx)
	p.pollOnce(ctx)

	p.mu.Lock()
	currentTrack := p.queue.Current()
	p.mu.Unlock()

	// Should only have incremented once
	if currentTrack.PlayCount != 1 {
		t.Errorf("expected play_count=1 (incremented once), got %d", currentTrack.PlayCount)
	}
}

func TestPlayFolder_StoresQueueMetadata(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetMusicDir("/tmp/music")

	for i, name := range []string{"01-song.flac", "02-song.flac"} {
		if err := s.UpsertTrack(ctx, &store.Track{
			Path: "artist/album/" + name, Title: fmt.Sprintf("Song %d", i+1),
			Artist: "Artist", Album: "Album", DurationMs: 180000, Format: "flac",
			AddedAt: int64(i + 1), MusicDir: "/tmp/music",
		}); err != nil {
			t.Fatal(err)
		}
	}

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)

	// Stop polling so it doesn't interfere
	p.mu.Lock()
	p.stopPollingLocked()
	p.mu.Unlock()

	if err := p.PlayFolder(ctx, "artist/album", "title", "desc"); err != nil {
		t.Fatalf("PlayFolder: %v", err)
	}

	p.mu.Lock()
	folder := p.queueFolder
	sortBy := p.queueSortBy
	sortOrder := p.queueSortOrder
	p.mu.Unlock()

	if folder != "artist/album" {
		t.Errorf("expected queueFolder='artist/album', got %q", folder)
	}
	if sortBy != "title" {
		t.Errorf("expected queueSortBy='title', got %q", sortBy)
	}
	if sortOrder != "desc" {
		t.Errorf("expected queueSortOrder='desc', got %q", sortOrder)
	}
}

func TestPollDisconnect_PreservesQueueMetadata(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)

	// Set up queue metadata and a playing state
	p.mu.Lock()
	p.stopPollingLocked()
	p.state = StatePlaying
	p.queue = NewQueue(testTracks())
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.queueFolder = "artist/album"
	p.queueSortBy = "title"
	p.queueSortOrder = "desc"
	p.mu.Unlock()

	// Make GetPosition fail to trigger disconnect via poll errors
	mt.mu.Lock()
	mt.getPositionErr = fmt.Errorf("connection refused")
	mt.mu.Unlock()

	// Simulate: first error happened 31 seconds ago (past the 30s threshold)
	p.mu.Lock()
	p.firstPollErrorAt = time.Now().Add(-31 * time.Second)
	p.mu.Unlock()

	// Single poll should trigger disconnect since we're past the threshold
	p.pollOnce(ctx)

	p.mu.Lock()
	transport := p.transport
	queue := p.queue
	folder := p.queueFolder
	sortBy := p.queueSortBy
	sortOrder := p.queueSortOrder
	p.mu.Unlock()

	if transport != nil {
		t.Error("expected transport to be nil after disconnect")
	}
	if queue == nil {
		t.Error("expected queue to be preserved after auto-disconnect")
	}
	if folder != "artist/album" {
		t.Errorf("expected queueFolder preserved as 'artist/album', got %q", folder)
	}
	if sortBy != "title" {
		t.Errorf("expected queueSortBy preserved as 'title', got %q", sortBy)
	}
	if sortOrder != "desc" {
		t.Errorf("expected queueSortOrder preserved as 'desc', got %q", sortOrder)
	}
}

func TestExplicitDisconnect_ClearsQueueMetadata(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetMusicDir("/tmp/music")

	for i, name := range []string{"01-song.flac", "02-song.flac"} {
		if err := s.UpsertTrack(ctx, &store.Track{
			Path: "artist/album/" + name, Title: fmt.Sprintf("Song %d", i+1),
			Artist: "Artist", Album: "Album", DurationMs: 180000, Format: "flac",
			AddedAt: int64(i + 1), MusicDir: "/tmp/music",
		}); err != nil {
			t.Fatal(err)
		}
	}

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)

	p.mu.Lock()
	p.stopPollingLocked()
	p.mu.Unlock()

	if err := p.PlayFolder(ctx, "artist/album", "title", "desc"); err != nil {
		t.Fatalf("PlayFolder: %v", err)
	}

	// Explicit disconnect should clear metadata
	p.Disconnect()

	p.mu.Lock()
	folder := p.queueFolder
	sortBy := p.queueSortBy
	sortOrder := p.queueSortOrder
	p.mu.Unlock()

	if folder != "" {
		t.Errorf("expected queueFolder cleared, got %q", folder)
	}
	if sortBy != "" {
		t.Errorf("expected queueSortBy cleared, got %q", sortBy)
	}
	if sortOrder != "" {
		t.Errorf("expected queueSortOrder cleared, got %q", sortOrder)
	}
}

func TestRecoverRendererState_UsesSavedFolderMetadata(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetMusicDir("/tmp/music")

	// Insert 3 tracks in "artist/album"
	for i, name := range []string{"01-song.flac", "02-song.flac", "03-song.flac"} {
		if err := s.UpsertTrack(ctx, &store.Track{
			Path: "artist/album/" + name, Title: fmt.Sprintf("Album Song %d", i+1),
			Artist: "Artist", Album: "Album", DurationMs: 180000, Format: "flac",
			AddedAt: int64(i + 1), MusicDir: "/tmp/music",
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Insert 2 tracks in "artist/singles"
	for i, name := range []string{"single-a.flac", "single-b.flac"} {
		if err := s.UpsertTrack(ctx, &store.Track{
			Path: "artist/singles/" + name, Title: fmt.Sprintf("Single %d", i+1),
			Artist: "Artist", Album: "Singles", DurationMs: 120000, Format: "flac",
			AddedAt: int64(i + 10), MusicDir: "/tmp/music",
		}); err != nil {
			t.Fatal(err)
		}
	}

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	// Pre-set saved queue metadata pointing to "artist/album" with custom sort
	p.mu.Lock()
	p.queueFolder = "artist/album"
	p.queueSortBy = "title"
	p.queueSortOrder = "desc"
	p.mu.Unlock()

	mt := newMockTransport()
	// Renderer is playing a track from "artist/album"
	streamURL := fmt.Sprintf("http://%s:8080/stream/artist/album/02-song.flac", p.localIP)
	mt.mu.Lock()
	mt.uri = streamURL
	mt.state = dlna.StatePlaying
	mt.position = 30 * time.Second
	mt.duration = 3 * time.Minute
	mt.mu.Unlock()

	p.SetTransport(mt)
	time.Sleep(200 * time.Millisecond)

	p.mu.Lock()
	queueLen := 0
	var titles []string
	if p.queue != nil {
		queueLen = p.queue.Len()
		for _, t := range p.queue.Tracks() {
			titles = append(titles, t.Title)
		}
	}
	p.mu.Unlock()

	// Should have 3 tracks from "artist/album" (not 2 from "artist/singles")
	if queueLen != 3 {
		t.Errorf("expected queue length 3, got %d", queueLen)
	}

	// Verify all tracks are from the album folder
	for _, title := range titles {
		if !strings.HasPrefix(title, "Album Song") {
			t.Errorf("expected all tracks from 'artist/album', got title %q", title)
		}
	}
}

func TestPollOnce_TransitioningDoesNotResetGracePeriod(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Set player to StateLoading with playStartedAt 4 seconds ago
	startedAt := time.Now().Add(-4 * time.Second)
	p.mu.Lock()
	p.state = StateLoading
	p.playStartedAt = startedAt
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.mu.Unlock()

	// Mock renderer reporting TRANSITIONING
	mt.setState("TRANSITIONING")
	mt.setPosition(0, 0)

	p.pollOnce(ctx)

	p.mu.Lock()
	actualStartedAt := p.playStartedAt
	state := p.state
	p.mu.Unlock()

	// playStartedAt should NOT have been reset
	if !actualStartedAt.Equal(startedAt) {
		t.Errorf("expected playStartedAt to remain unchanged, was %v, now %v", startedAt, actualStartedAt)
	}
	// Should still be loading
	if state != StateLoading {
		t.Errorf("expected StateLoading, got %s", state)
	}
}

func TestTimeBasedDisconnect_NoDisconnectWithin30s(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatal(err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)
	p.mu.Lock()
	p.stopPollingLocked()
	p.state = StateIdle
	p.mu.Unlock()

	mt.mu.Lock()
	mt.getStateErr = fmt.Errorf("connection refused")
	mt.mu.Unlock()

	// Poll 50 times rapidly — all within 30s, should NOT disconnect
	for i := 0; i < 50; i++ {
		p.pollOnce(ctx)
	}

	p.mu.Lock()
	transport := p.transport
	p.mu.Unlock()

	if transport == nil {
		t.Error("expected transport to still be set — errors within 30s window")
	}
}

func TestTimeBasedDisconnect_DisconnectsAfter30s(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatal(err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)
	p.mu.Lock()
	p.stopPollingLocked()
	p.state = StateIdle
	p.mu.Unlock()

	mt.mu.Lock()
	mt.getStateErr = fmt.Errorf("connection refused")
	mt.mu.Unlock()

	// Simulate: first error happened 31 seconds ago
	p.mu.Lock()
	p.firstPollErrorAt = time.Now().Add(-31 * time.Second)
	p.mu.Unlock()

	p.pollOnce(ctx)

	p.mu.Lock()
	transport := p.transport
	p.mu.Unlock()

	if transport != nil {
		t.Error("expected transport nil — errors exceeded 30s threshold")
	}
}

func TestTimeBasedDisconnect_ResetsOnSuccess(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatal(err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)
	p.mu.Lock()
	p.stopPollingLocked()
	p.state = StateIdle
	p.mu.Unlock()

	// Fail a few times
	mt.mu.Lock()
	mt.getStateErr = fmt.Errorf("timeout")
	mt.mu.Unlock()
	p.pollOnce(ctx)
	p.pollOnce(ctx)

	p.mu.Lock()
	hasError := !p.firstPollErrorAt.IsZero()
	p.mu.Unlock()
	if !hasError {
		t.Error("expected firstPollErrorAt to be set after failures")
	}

	// Succeed — should reset
	mt.mu.Lock()
	mt.getStateErr = nil
	mt.mu.Unlock()
	p.pollOnce(ctx)

	p.mu.Lock()
	reset := p.firstPollErrorAt.IsZero()
	transport := p.transport
	p.mu.Unlock()

	if !reset {
		t.Error("expected firstPollErrorAt to reset after success")
	}
	if transport == nil {
		t.Error("expected transport to survive after error recovery")
	}
}

func TestAutoDisconnect_PreservesQueue(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatal(err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)
	tracks := testTracks()
	p.mu.Lock()
	p.stopPollingLocked()
	p.state = StatePlaying
	p.queue = NewQueue(tracks)
	p.queueFolder = "artist/album"
	p.currentStreamURL = "http://192.168.1.1:8080/stream/test"
	p.firstPollErrorAt = time.Now().Add(-31 * time.Second)
	p.mu.Unlock()

	mt.mu.Lock()
	mt.getPositionErr = fmt.Errorf("unreachable")
	mt.mu.Unlock()

	p.pollOnce(ctx)

	p.mu.Lock()
	queue := p.queue
	folder := p.queueFolder
	transport := p.transport
	p.mu.Unlock()

	if transport != nil {
		t.Error("expected transport nil after auto-disconnect")
	}
	if queue == nil {
		t.Error("expected queue preserved after auto-disconnect")
	}
	if folder != "artist/album" {
		t.Errorf("expected queueFolder preserved, got %q", folder)
	}
}

func TestAutoDisconnect_SetsReconnecting(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatal(err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)
	p.mu.Lock()
	p.stopPollingLocked()
	p.state = StateIdle
	p.firstPollErrorAt = time.Now().Add(-31 * time.Second)
	p.mu.Unlock()

	var lastState PlayerState
	var mu sync.Mutex
	p.SetOnChange(func(ps PlayerState) { mu.Lock(); lastState = ps; mu.Unlock() })

	mt.mu.Lock()
	mt.getStateErr = fmt.Errorf("unreachable")
	mt.mu.Unlock()

	p.pollOnce(ctx)

	mu.Lock()
	reconnecting := lastState.Reconnecting
	connected := lastState.Connected
	mu.Unlock()

	if !reconnecting {
		t.Error("expected Reconnecting=true after auto-disconnect")
	}
	if connected {
		t.Error("expected Connected=false after auto-disconnect")
	}
}

func TestAutoDisconnect_CallsOnDisconnect(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatal(err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)
	p.mu.Lock()
	p.stopPollingLocked()
	p.state = StateIdle
	p.firstPollErrorAt = time.Now().Add(-31 * time.Second)
	p.mu.Unlock()

	var called int32
	p.SetOnDisconnect(func() { atomic.AddInt32(&called, 1) })

	mt.mu.Lock()
	mt.getStateErr = fmt.Errorf("unreachable")
	mt.mu.Unlock()

	p.pollOnce(ctx)
	time.Sleep(50 * time.Millisecond) // callback is async

	if atomic.LoadInt32(&called) == 0 {
		t.Error("expected onDisconnect callback to fire")
	}
}

func TestSetTransport_ClearsReconnecting(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatal(err)
	}

	p.mu.Lock()
	p.reconnecting = true
	p.mu.Unlock()

	mt := newMockTransport()
	p.SetTransport(mt)

	p.mu.Lock()
	reconnecting := p.reconnecting
	p.mu.Unlock()

	if reconnecting {
		t.Error("expected reconnecting=false after SetTransport")
	}
}

func TestPlayCurrentLocked_ImmediateStateTransition(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Mock: renderer transitions to PLAYING immediately on Play()
	// (mockTransport.Play already sets state to PLAYING)
	mt.setPosition(0, 3*time.Minute)

	p.mu.Lock()
	err := p.playCurrentLocked(ctx)
	state := p.state
	p.mu.Unlock()

	if err != nil {
		t.Fatalf("playCurrentLocked: %v", err)
	}

	if state != StatePlaying {
		t.Errorf("expected immediate StatePlaying, got %s", state)
	}
}

func TestClearReconnecting_ClearsState(t *testing.T) {
	p, _ := setupTestPlayer(t, testTracks())

	p.mu.Lock()
	p.reconnecting = true
	p.transport = nil
	p.mu.Unlock()

	var lastState PlayerState
	p.SetOnChange(func(ps PlayerState) { lastState = ps })

	p.ClearReconnecting()

	p.mu.Lock()
	reconnecting := p.reconnecting
	queue := p.queue
	p.mu.Unlock()

	if reconnecting {
		t.Error("expected reconnecting=false")
	}
	if queue != nil {
		t.Error("expected queue cleared")
	}
	if lastState.Reconnecting {
		t.Error("expected broadcast with Reconnecting=false")
	}
}

func TestDisconnect_AlwaysCallsStop(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())

	p.mu.Lock()
	p.state = StateIdle
	p.mu.Unlock()

	mt.setState(dlna.StateStopped)

	p.Disconnect()

	mt.mu.Lock()
	stops := mt.stopCalls
	mt.mu.Unlock()

	if stops < 1 {
		t.Errorf("expected Stop called even when Idle, got %d calls", stops)
	}
}

func TestDisconnect_HandlesStopError(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())

	p.mu.Lock()
	p.state = StatePlaying
	p.mu.Unlock()

	mt.mu.Lock()
	mt.stopErr = fmt.Errorf("network timeout")
	mt.mu.Unlock()

	p.Disconnect()

	p.mu.Lock()
	transport := p.transport
	state := p.state
	p.mu.Unlock()

	if transport != nil {
		t.Error("expected transport cleared even after Stop error")
	}
	if state != StateIdle {
		t.Errorf("expected StateIdle, got %s", state)
	}
}

func TestPlayCurrentLocked_SetsAggressivePollUntil(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetMusicDir("/tmp/music")

	for i, name := range []string{"01-song.flac", "02-song.flac"} {
		if err := s.UpsertTrack(ctx, &store.Track{
			Path: "artist/album/" + name, Title: fmt.Sprintf("Song %d", i+1),
			Artist: "Artist", Album: "Album", DurationMs: 180000, Format: "flac",
			AddedAt: int64(i + 1), MusicDir: "/tmp/music",
		}); err != nil {
			t.Fatal(err)
		}
	}

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)

	p.mu.Lock()
	p.stopPollingLocked()
	p.mu.Unlock()

	before := time.Now()
	if err := p.PlayFolder(ctx, "artist/album", "added_at", "asc"); err != nil {
		t.Fatalf("PlayFolder: %v", err)
	}

	p.mu.Lock()
	aggressiveUntil := p.aggressivePollUntil
	p.stopPollingLocked()
	p.mu.Unlock()

	// aggressivePollUntil should be ~10 seconds in the future
	expected := before.Add(10 * time.Second)
	if aggressiveUntil.Before(expected.Add(-1 * time.Second)) {
		t.Errorf("aggressivePollUntil too early: %v, expected around %v", aggressiveUntil, expected)
	}
	if aggressiveUntil.After(expected.Add(1 * time.Second)) {
		t.Errorf("aggressivePollUntil too late: %v, expected around %v", aggressiveUntil, expected)
	}
}

func TestRemoveFromQueue_NonCurrentTrack(t *testing.T) {
	p, _ := setupTestPlayer(t, testTracks())
	p.mu.Lock()
	p.state = StatePlaying
	p.mu.Unlock()

	err := p.RemoveFromQueue(context.Background(), 3)
	if err != nil {
		t.Fatalf("RemoveFromQueue: %v", err)
	}

	p.mu.Lock()
	qLen := p.queue.Len()
	curID := p.queue.Current().ID
	state := p.state
	p.mu.Unlock()

	if qLen != 2 {
		t.Errorf("want len 2, got %d", qLen)
	}
	if curID != 1 {
		t.Errorf("want current 1 (unchanged), got %d", curID)
	}
	if state != StatePlaying {
		t.Errorf("want StatePlaying, got %s", state)
	}
}

func TestRemoveFromQueue_CurrentTrackSkipsToNext(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	mt.mu.Lock()
	mt.checkCtx = true
	mt.mu.Unlock()

	p.mu.Lock()
	p.state = StatePlaying
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.mu.Unlock()

	err := p.RemoveFromQueue(context.Background(), 1)
	if err != nil {
		t.Fatalf("RemoveFromQueue: %v", err)
	}

	p.mu.Lock()
	qLen := p.queue.Len()
	curID := p.queue.Current().ID
	p.mu.Unlock()

	if qLen != 2 {
		t.Errorf("want len 2, got %d", qLen)
	}
	if curID != 2 {
		t.Errorf("want current 2 (next), got %d", curID)
	}
}

func TestRemoveFromQueue_LastTrackGoesIdle(t *testing.T) {
	single := []store.Track{{ID: 1, Path: "a.flac", Title: "A"}}
	p, _ := setupTestPlayer(t, single)
	p.mu.Lock()
	p.state = StatePlaying
	p.mu.Unlock()

	err := p.RemoveFromQueue(context.Background(), 1)
	if err != nil {
		t.Fatalf("RemoveFromQueue: %v", err)
	}

	p.mu.Lock()
	state := p.state
	p.mu.Unlock()

	if state != StateIdle {
		t.Errorf("want StateIdle, got %s", state)
	}
}

func TestRemoveFromQueue_NotInQueue(t *testing.T) {
	p, _ := setupTestPlayer(t, testTracks())
	err := p.RemoveFromQueue(context.Background(), 99)
	if err == nil {
		t.Error("expected error for non-existent track")
	}
}

func TestRejectFromQueue_NonCurrentTrack(t *testing.T) {
	p, _ := setupTestPlayer(t, testTracks())

	musicDir := t.TempDir()
	deleteDir := t.TempDir()
	p.mu.Lock()
	p.musicDir = musicDir
	p.deleteDir = deleteDir
	p.state = StatePlaying
	p.mu.Unlock()

	os.MkdirAll(filepath.Join(musicDir, "artist", "album"), 0755)
	os.WriteFile(filepath.Join(musicDir, "artist", "album", "02-song2.flac"), []byte("fake"), 0644)

	err := p.RejectFromQueue(context.Background(), 2)
	if err != nil {
		t.Fatalf("RejectFromQueue: %v", err)
	}

	// File moved to deleteDir
	if _, err := os.Stat(filepath.Join(deleteDir, "artist", "album", "02-song2.flac")); os.IsNotExist(err) {
		t.Error("expected file in deleteDir")
	}

	p.mu.Lock()
	qLen := p.queue.Len()
	curID := p.queue.Current().ID
	state := p.state
	p.mu.Unlock()

	if qLen != 2 {
		t.Errorf("want len 2, got %d", qLen)
	}
	if curID != 1 {
		t.Errorf("want current 1, got %d", curID)
	}
	if state != StatePlaying {
		t.Errorf("want StatePlaying, got %s", state)
	}
}

func TestRejectFromQueue_CurrentTrack(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	mt.mu.Lock()
	mt.checkCtx = true
	mt.mu.Unlock()

	musicDir := t.TempDir()
	deleteDir := t.TempDir()
	p.mu.Lock()
	p.musicDir = musicDir
	p.deleteDir = deleteDir
	p.state = StatePlaying
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.mu.Unlock()

	os.MkdirAll(filepath.Join(musicDir, "artist", "album"), 0755)
	os.WriteFile(filepath.Join(musicDir, "artist", "album", "01-song1.flac"), []byte("fake"), 0644)

	err := p.RejectFromQueue(context.Background(), 1)
	if err != nil {
		t.Fatalf("RejectFromQueue: %v", err)
	}

	if _, err := os.Stat(filepath.Join(deleteDir, "artist", "album", "01-song1.flac")); os.IsNotExist(err) {
		t.Error("expected file in deleteDir")
	}

	p.mu.Lock()
	qLen := p.queue.Len()
	curID := p.queue.Current().ID
	p.mu.Unlock()

	if qLen != 2 {
		t.Errorf("want len 2, got %d", qLen)
	}
	if curID != 2 {
		t.Errorf("want current 2, got %d", curID)
	}
}

func TestGetQueueContext(t *testing.T) {
	p, _ := setupTestPlayer(t, testTracks())

	p.mu.Lock()
	p.queueFolder = "Jazz/Bebop"
	p.queueSortBy = "added_at"
	p.queueSortOrder = "desc"
	p.mu.Unlock()

	folder, sortBy, sortOrder := p.GetQueueContext()
	if folder != "Jazz/Bebop" {
		t.Errorf("folder: want 'Jazz/Bebop', got %q", folder)
	}
	if sortBy != "added_at" {
		t.Errorf("sortBy: want 'added_at', got %q", sortBy)
	}
	if sortOrder != "desc" {
		t.Errorf("sortOrder: want 'desc', got %q", sortOrder)
	}
}

func TestPollOnce_NoNotifyDuringLoadingTransitioning(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Put player in loading state
	p.mu.Lock()
	p.state = StateLoading
	p.playStartedAt = time.Now()
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.mu.Unlock()

	// Count notifications
	var notifyCount int
	var mu sync.Mutex
	p.SetOnChange(func(ps PlayerState) {
		mu.Lock()
		notifyCount++
		mu.Unlock()
	})

	// Poll with TRANSITIONING — should NOT notify
	mt.setState("TRANSITIONING")
	mt.setPosition(0, 0)
	p.pollOnce(ctx)

	mu.Lock()
	count1 := notifyCount
	mu.Unlock()
	if count1 != 0 {
		t.Errorf("expected 0 notifications during TRANSITIONING, got %d", count1)
	}

	// Poll while still loading (STOPPED within grace period) — should NOT notify
	mt.setState(dlna.StateStopped)
	p.pollOnce(ctx)

	mu.Lock()
	count2 := notifyCount
	mu.Unlock()
	if count2 != 0 {
		t.Errorf("expected 0 notifications during loading/stopped, got %d", count2)
	}

	// Poll with PLAYING — SHOULD notify (state change)
	mt.setState(dlna.StatePlaying)
	p.pollOnce(ctx)

	mu.Lock()
	count3 := notifyCount
	mu.Unlock()
	if count3 != 1 {
		t.Errorf("expected 1 notification on loading→playing transition, got %d", count3)
	}
}

func TestPollOnce_NotifiesOnPlayingStateUpdate(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Put player in playing state (playCounted=true to avoid play count notify)
	p.mu.Lock()
	p.state = StatePlaying
	p.playStartedAt = time.Now().Add(-10 * time.Second) // past grace period
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac"
	p.playCounted = true
	p.mu.Unlock()

	var notifyCount int
	var mu sync.Mutex
	p.SetOnChange(func(ps PlayerState) {
		mu.Lock()
		notifyCount++
		mu.Unlock()
	})

	// Normal playing poll — should still notify (for position updates)
	mt.setState(dlna.StatePlaying)
	mt.setPosition(5000, 180000)
	p.pollOnce(ctx)

	mu.Lock()
	count := notifyCount
	mu.Unlock()
	if count != 1 {
		t.Errorf("expected 1 notification during playing poll, got %d", count)
	}
}

func TestRefreshTrack_UpdatesCurrentAndNotifies(t *testing.T) {
	// Insert a track into DB, then set its transcode score
	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	s.SetMusicDir("/tmp/music")
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	dbTrack := &store.Track{
		Path: "a/b.flac", Title: "Song", Format: "flac",
		AddedAt: 1000, MusicDir: "/tmp/music",
	}
	if err := s.UpsertTrack(ctx, dbTrack); err != nil {
		t.Fatal(err)
	}
	allTracks, _ := s.ListTracks(ctx, "", "added_at", "asc")
	trackID := allTracks[0].ID

	// Simulate analyzer writing transcode data
	if err := s.UpdateTranscodeAnalysis(ctx, trackID, 0.85, "MP3"); err != nil {
		t.Fatal(err)
	}

	// Create player with a queue containing a stale track (no transcode data)
	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatal(err)
	}
	mt := newMockTransport()
	p.SetTransport(mt)
	p.mu.Lock()
	p.queue = NewQueue([]store.Track{{ID: trackID, Path: "a/b.flac", Title: "Song"}})
	p.mu.Unlock()

	var notified atomic.Bool
	p.SetOnChange(func(ps PlayerState) {
		if ps.Track != nil && ps.Track.TranscodeScore == 0.85 {
			notified.Store(true)
		}
	})

	p.RefreshTrack(ctx, trackID)

	if !notified.Load() {
		t.Error("expected onChange called with updated transcode score")
	}

	p.mu.Lock()
	cur := p.queue.Current()
	p.mu.Unlock()
	if cur.TranscodeScore != 0.85 {
		t.Errorf("expected TranscodeScore=0.85, got %f", cur.TranscodeScore)
	}
}

func TestDisconnectNotFiredTwiceWithoutReconnect(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)

	p.mu.Lock()
	p.state = StateIdle
	p.stopPollingLocked()
	p.mu.Unlock()

	var disconnectCount atomic.Int32
	p.SetOnDisconnect(func() {
		disconnectCount.Add(1)
	})

	// Make heartbeat fail
	mt.mu.Lock()
	mt.getStateErr = fmt.Errorf("connection refused")
	mt.mu.Unlock()

	// Backdate error to exceed timeout
	p.mu.Lock()
	p.firstPollErrorAt = time.Now().Add(-31 * time.Second)
	p.mu.Unlock()

	// First poll triggers disconnect
	p.pollOnce(ctx)
	time.Sleep(50 * time.Millisecond)

	if got := disconnectCount.Load(); got != 1 {
		t.Fatalf("expected 1 disconnect, got %d", got)
	}

	// Now simulate: transport is set back (by reconnect) but immediately fails again
	mt2 := newMockTransport()
	mt2.mu.Lock()
	mt2.getStateErr = fmt.Errorf("connection refused again")
	mt2.mu.Unlock()
	p.SetTransport(mt2) // This resets the reconnecting flag

	p.mu.Lock()
	p.state = StateIdle
	p.stopPollingLocked()
	p.firstPollErrorAt = time.Now().Add(-31 * time.Second)
	p.mu.Unlock()

	// Second disconnect should fire (new transport = new disconnect cycle)
	p.pollOnce(ctx)
	time.Sleep(50 * time.Millisecond)

	if got := disconnectCount.Load(); got != 2 {
		t.Errorf("expected 2 total disconnects (reset after SetTransport), got %d", got)
	}
}

func TestDisconnectNotFiredWhenAlreadyReconnecting(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	s, err := store.New(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	p, err := New(ctx, s, "/tmp/music", "/tmp/delete", "8080")
	if err != nil {
		t.Fatalf("create player: %v", err)
	}

	mt := newMockTransport()
	p.SetTransport(mt)

	p.mu.Lock()
	p.state = StateIdle
	p.stopPollingLocked()
	// Pre-set reconnecting to true (simulating a reconnect already in progress)
	p.reconnecting = true
	p.mu.Unlock()

	var disconnectCount atomic.Int32
	p.SetOnDisconnect(func() {
		disconnectCount.Add(1)
	})

	mt.mu.Lock()
	mt.getStateErr = fmt.Errorf("connection refused")
	mt.mu.Unlock()

	p.mu.Lock()
	p.firstPollErrorAt = time.Now().Add(-31 * time.Second)
	p.mu.Unlock()

	p.pollOnce(ctx)
	time.Sleep(50 * time.Millisecond)

	// Should NOT fire onDisconnect because we're already reconnecting
	if got := disconnectCount.Load(); got != 0 {
		t.Errorf("expected 0 disconnects while already reconnecting, got %d", got)
	}
}

func TestRefreshTrack_IgnoresNonCurrentTrack(t *testing.T) {
	tracks := makeTracks(3)
	p, _ := setupTestPlayer(t, tracks)

	var notified atomic.Bool
	p.SetOnChange(func(ps PlayerState) {
		notified.Store(true)
	})

	p.RefreshTrack(context.Background(), 2)

	if notified.Load() {
		t.Error("expected no notification for non-current track")
	}
}

// TestResume_FromIdleWithQueue_RestartsPlayback verifies that when the renderer
// has silently lost its transport URI (e.g. after phone sleep) and the player
// state has drifted to Idle while a queue is still loaded, calling Resume
// recovers by re-sending SetURI + Play rather than silently returning nil.
func TestResume_FromIdleWithQueue_RestartsPlayback(t *testing.T) {
	tracks := makeTracks(3)
	p, mt := setupTestPlayer(t, tracks)

	// Force the player into the "post-sleep amnesia" scenario: queue is still
	// populated but state drifted to Idle.
	p.mu.Lock()
	p.state = StateIdle
	p.mu.Unlock()

	// User presses play.
	if err := p.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	mt.mu.Lock()
	playCalls := mt.playCalls
	setURICalls := mt.setURICalls
	mt.mu.Unlock()

	// Expected: playCurrentLocked path took over — URI was re-set AND Play was called.
	if setURICalls == 0 {
		t.Errorf("expected SetURI to be called to recover, got %d", setURICalls)
	}
	if playCalls == 0 {
		t.Errorf("expected Play to be called, got %d", playCalls)
	}
}

// TestResume_FromIdleWithoutQueue_NoOp verifies that the recovery path does
// not fire when there is no queue — Resume should remain a no-op.
func TestResume_FromIdleWithoutQueue_NoOp(t *testing.T) {
	p, mt := setupTestPlayer(t, nil)

	p.mu.Lock()
	p.state = StateIdle
	p.mu.Unlock()

	if err := p.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()
	if mt.playCalls != 0 || mt.setURICalls != 0 {
		t.Errorf("expected no renderer calls when queue is empty, got play=%d setURI=%d",
			mt.playCalls, mt.setURICalls)
	}
}

// TestResume_FromPaused_SendsPlay verifies the normal paused→playing path:
// Play is sent but SetURI is not (the renderer still has the URI).
func TestResume_FromPaused_SendsPlay(t *testing.T) {
	tracks := makeTracks(3)
	p, mt := setupTestPlayer(t, tracks)

	p.mu.Lock()
	p.state = StatePaused
	p.mu.Unlock()

	if err := p.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	p.mu.Lock()
	state := p.state
	p.mu.Unlock()
	if state != StatePlaying {
		t.Errorf("state = %q, want %q", state, StatePlaying)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()
	if mt.playCalls != 1 {
		t.Errorf("expected 1 Play call, got %d", mt.playCalls)
	}
	if mt.setURICalls != 0 {
		t.Errorf("expected 0 SetURI calls (paused path should not re-SetURI), got %d",
			mt.setURICalls)
	}
}

// TestNext_SeeksRendererToZero verifies that on a track change, the player
// explicitly seeks the renderer to position 0 after Play. Some DLNA renderers
// (e.g. Wiim Ultra) carry the seek offset from the previous track across
// SetAVTransportURI, causing the new track to audibly start at the old
// position. The fix forces Seek(0) on every track change.
func TestNext_SeeksRendererToZero(t *testing.T) {
	tracks := makeTracks(3)
	p, mt := setupTestPlayer(t, tracks)

	// Clear any calls from setup.
	mt.mu.Lock()
	mt.callOrder = nil
	mt.seekCalls = 0
	mt.seekTargets = nil
	mt.mu.Unlock()

	// Simulate that track 1 is currently playing — Next will advance to track 2.
	p.mu.Lock()
	p.state = StatePlaying
	p.currentStreamURL = "http://192.168.1.1:8080/stream/0-song-0.flac"
	p.mu.Unlock()

	if err := p.Next(context.Background()); err != nil {
		t.Fatalf("Next: %v", err)
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()

	if mt.seekCalls < 1 {
		t.Fatalf("expected at least 1 Seek call on track change, got %d", mt.seekCalls)
	}

	// The first Seek after the track change must target zero.
	gotZero := false
	for _, d := range mt.seekTargets {
		if d == 0 {
			gotZero = true
			break
		}
	}
	if !gotZero {
		t.Errorf("expected a Seek(0) call, got targets=%v", mt.seekTargets)
	}

	// Seek must come AFTER Play — otherwise the renderer won't honour it.
	lastPlayIdx := -1
	firstSeekAfterPlay := -1
	for i, c := range mt.callOrder {
		if c == "Play" {
			lastPlayIdx = i
		}
		if c == "Seek" && lastPlayIdx >= 0 && firstSeekAfterPlay < 0 {
			firstSeekAfterPlay = i
		}
	}
	if lastPlayIdx < 0 {
		t.Fatalf("expected at least one Play call, callOrder=%v", mt.callOrder)
	}
	if firstSeekAfterPlay < 0 {
		t.Errorf("expected a Seek call after Play, callOrder=%v", mt.callOrder)
	}
}

// TestPollOnce_IgnoresStaleURIDuringTrackTransition verifies that when the
// renderer is still reporting the *previous* track's URI and position
// immediately after a track change, the poll goroutine does NOT overwrite
// positionMs with the stale value. Without the guard, Track B can flash
// Track A's position (e.g. 1:30) in the UI right after pressing Next.
func TestPollOnce_IgnoresStaleURIDuringTrackTransition(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	// Simulate: track change just happened, player is already in Playing
	// state (skipping Loading), but the renderer is still reporting the
	// previous track's URI and position.
	p.mu.Lock()
	p.state = StatePlaying
	p.playStartedAt = time.Now()       // within 2s grace window
	p.playStartTime = time.Now()
	p.currentStreamURL = "http://192.168.1.1:8080/stream/artist/album/02-song2.flac"
	p.positionMs = 0
	p.mu.Unlock()

	// Renderer still reports the OLD track URI with a 90s position.
	mt.mu.Lock()
	mt.state = dlna.StatePlaying
	mt.uri = "http://192.168.1.1:8080/stream/artist/album/01-song1.flac" // stale
	mt.position = 90 * time.Second
	mt.duration = 3 * time.Minute
	mt.mu.Unlock()

	p.pollOnce(ctx)

	p.mu.Lock()
	positionMs := p.positionMs
	state := p.state
	streamURL := p.currentStreamURL
	p.mu.Unlock()

	// Position must NOT be overwritten with the stale 90s value.
	if positionMs == 90_000 {
		t.Errorf("positionMs was overwritten with stale URI's position (90000ms); want unchanged")
	}

	// Must NOT go idle within the grace window — the URI mismatch is
	// transitional, not an external takeover.
	if state == StateIdle {
		t.Errorf("expected state to remain %q within grace window, got %q", StatePlaying, state)
	}
	if streamURL == "" {
		t.Errorf("currentStreamURL was cleared within grace window; want preserved")
	}
}

// TestPollOnce_ExternalTakeoverAfterGracePeriod verifies that a URI mismatch
// is still treated as external takeover once the 2-second grace window has
// elapsed. Without this, the player would never detect a genuine takeover.
func TestPollOnce_ExternalTakeoverAfterGracePeriod(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())
	ctx := context.Background()

	p.mu.Lock()
	p.state = StatePlaying
	p.playStartedAt = time.Now().Add(-10 * time.Second) // well past grace window
	p.playStartTime = time.Now().Add(-10 * time.Second)
	p.currentStreamURL = "http://192.168.1.1:8080/stream/ours.flac"
	p.mu.Unlock()

	mt.mu.Lock()
	mt.state = dlna.StatePlaying
	mt.uri = "http://elsewhere/stream.flac"
	mt.position = 30 * time.Second
	mt.duration = 3 * time.Minute
	mt.mu.Unlock()

	p.pollOnce(ctx)

	p.mu.Lock()
	state := p.state
	p.mu.Unlock()

	if state != StateIdle {
		t.Errorf("expected StateIdle after external takeover past grace window, got %q", state)
	}
}

func TestPollOnce_RetryAfterGracePeriodIssuesStopFirst(t *testing.T) {
	p, mt := setupTestPlayer(t, testTracks())

	// Wedged renderer: never reports Playing.
	mt.setState(dlna.StateStopped)
	mt.setPosition(0, 0)

	// Force player into Loading well past grace period.
	p.mu.Lock()
	p.state = StateLoading
	p.currentStreamURL = "http://test/stream/1"
	p.playStartedAt = time.Now().Add(-10 * time.Second)
	p.mu.Unlock()

	initialStops := mt.stopCalls
	initialPlays := mt.playCalls

	p.pollOnce(context.Background())

	if mt.stopCalls <= initialStops {
		t.Fatalf("expected Stop during retry, got stopCalls=%d (was %d)", mt.stopCalls, initialStops)
	}
	if mt.playCalls <= initialPlays {
		t.Fatalf("expected Play during retry, got playCalls=%d (was %d)", mt.playCalls, initialPlays)
	}

	// Find indices of the LAST Stop and LAST Play; Stop must come before Play.
	lastStop, lastPlay := -1, -1
	for i, c := range mt.callOrder {
		if c == "Stop" {
			lastStop = i
		}
		if c == "Play" {
			lastPlay = i
		}
	}
	if lastStop == -1 || lastPlay == -1 || lastStop > lastPlay {
		t.Fatalf("expected Stop before Play in retry, got order=%v", mt.callOrder)
	}
}
