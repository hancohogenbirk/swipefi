package player

import (
	"context"
	"fmt"
	"sync"
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
	playErr     error
	getStateErr error
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
	m.state = dlna.StateStopped
	return nil
}

func (m *mockTransport) Pause(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = dlna.StatePaused
	return nil
}

func (m *mockTransport) Seek(_ context.Context, _ time.Duration) error {
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

	// Poll 3 times — should trigger disconnect after 3 consecutive errors
	for i := 0; i < 3; i++ {
		p.pollOnce(ctx)
	}

	p.mu.Lock()
	transport := p.transport
	state := p.state
	p.mu.Unlock()

	if transport != nil {
		t.Error("expected transport to be nil after 3 heartbeat failures")
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

	// Succeed — should reset error counter
	mt.mu.Lock()
	mt.getStateErr = nil
	mt.mu.Unlock()

	p.pollOnce(ctx)

	p.mu.Lock()
	errors := p.pollErrors
	transport := p.transport
	p.mu.Unlock()

	if errors != 0 {
		t.Errorf("expected pollErrors to reset to 0, got %d", errors)
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
			want:    "My%20Artist/album/song.flac",
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
