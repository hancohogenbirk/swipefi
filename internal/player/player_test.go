package player

import (
	"context"
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
