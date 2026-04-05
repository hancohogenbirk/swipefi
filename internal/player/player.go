package player

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"swipefi/internal/dlna"
	"swipefi/internal/store"
)

type State string

const (
	StateIdle    State = "idle"
	StatePlaying State = "playing"
	StatePaused  State = "paused"
)

// PlayerState is the full state broadcast to WebSocket clients.
type PlayerState struct {
	State         State        `json:"state"`
	Track         *store.Track `json:"track,omitempty"`
	PositionMs    int64        `json:"position_ms"`
	DurationMs    int64        `json:"duration_ms"`
	QueueLength   int          `json:"queue_length"`
	QueuePosition int          `json:"queue_position"`
}

// StateChangeFunc is called when player state changes.
type StateChangeFunc func(PlayerState)

type Player struct {
	mu sync.Mutex

	store     *store.Store
	musicDir  string
	deleteDir string
	port      string
	localIP   string

	transport *dlna.Transport
	queue     *Queue
	state     State

	// Play time tracking for the 60-second threshold
	playStartTime  time.Time
	accumulatedMs  int64
	playCounted    bool

	// Current position from renderer
	positionMs int64
	durationMs int64

	// Polling
	pollCancel context.CancelFunc

	// State change callback
	onChange StateChangeFunc
}

func New(s *store.Store, musicDir, deleteDir, port string) (*Player, error) {
	localIP, err := dlna.GetLocalIP()
	if err != nil {
		return nil, fmt.Errorf("get local ip: %w", err)
	}
	slog.Info("detected local IP", "ip", localIP)

	return &Player{
		store:     s,
		musicDir:  musicDir,
		deleteDir: deleteDir,
		port:      port,
		localIP:   localIP,
		state:     StateIdle,
	}, nil
}

func (p *Player) SetOnChange(fn StateChangeFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onChange = fn
}

func (p *Player) SetDirs(musicDir, deleteDir string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.musicDir = musicDir
	p.deleteDir = deleteDir
}

func (p *Player) SetTransport(t *dlna.Transport) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.transport = t
}

func (p *Player) GetState() PlayerState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stateLocked()
}

func (p *Player) stateLocked() PlayerState {
	ps := PlayerState{
		State:      p.state,
		PositionMs: p.positionMs,
		DurationMs: p.durationMs,
	}
	if p.queue != nil {
		ps.QueueLength = p.queue.Len()
		ps.QueuePosition = p.queue.Position()
		ps.Track = p.queue.Current()
	}
	return ps
}

func (p *Player) notify() {
	if p.onChange != nil {
		p.onChange(p.stateLocked())
	}
}

// PlayFolder builds a queue from the given folder and starts playback.
func (p *Player) PlayFolder(ctx context.Context, folder, sortBy, order string) error {
	p.mu.Lock()
	if p.transport == nil {
		p.mu.Unlock()
		return fmt.Errorf("no renderer selected")
	}
	p.mu.Unlock()

	tracks, err := p.store.ListTracks(ctx, folder, sortBy, order)
	if err != nil {
		return fmt.Errorf("list tracks: %w", err)
	}
	if len(tracks) == 0 {
		return fmt.Errorf("no tracks in folder")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.queue = NewQueue(tracks)
	return p.playCurrentLocked(ctx)
}

func (p *Player) playCurrentLocked(ctx context.Context) error {
	track := p.queue.Current()
	if track == nil {
		p.state = StateIdle
		p.stopPollingLocked()
		p.notify()
		return nil
	}

	streamURL := fmt.Sprintf("http://%s:%s/stream/%s", p.localIP, p.port, track.Path)
	slog.Info("playing track", "title", track.Title, "artist", track.Artist, "url", streamURL)

	if err := p.transport.SetURI(ctx, streamURL, ""); err != nil {
		return fmt.Errorf("set uri: %w", err)
	}
	if err := p.transport.Play(ctx); err != nil {
		return fmt.Errorf("play: %w", err)
	}

	p.state = StatePlaying
	p.positionMs = 0
	p.durationMs = 0
	p.playStartTime = time.Now()
	p.accumulatedMs = 0
	p.playCounted = false

	p.startPollingLocked(ctx)
	p.notify()
	return nil
}

func (p *Player) Pause(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.transport == nil || p.state != StatePlaying {
		return nil
	}

	if err := p.transport.Pause(ctx); err != nil {
		return fmt.Errorf("pause: %w", err)
	}

	// Accumulate play time
	p.accumulatedMs += time.Since(p.playStartTime).Milliseconds()
	p.state = StatePaused
	p.notify()
	return nil
}

func (p *Player) Resume(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.transport == nil || p.state != StatePaused {
		return nil
	}

	if err := p.transport.Play(ctx); err != nil {
		return fmt.Errorf("resume: %w", err)
	}

	p.playStartTime = time.Now()
	p.state = StatePlaying
	p.notify()
	return nil
}

func (p *Player) Next(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.queue == nil {
		return nil
	}

	p.checkPlayCountLocked(ctx)

	if p.queue.Next() == nil {
		// End of queue
		if p.transport != nil {
			p.transport.Stop(ctx)
		}
		p.state = StateIdle
		p.stopPollingLocked()
		p.notify()
		return nil
	}

	return p.playCurrentLocked(ctx)
}

func (p *Player) Prev(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.queue == nil {
		return nil
	}

	p.checkPlayCountLocked(ctx)

	if p.queue.Prev() == nil {
		return nil // Already at the beginning
	}

	return p.playCurrentLocked(ctx)
}

func (p *Player) Seek(ctx context.Context, positionMs int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.transport == nil {
		return nil
	}

	d := time.Duration(positionMs) * time.Millisecond
	return p.transport.Seek(ctx, d)
}

// Reject moves the current track to to_delete, removes it from the queue, and plays next.
func (p *Player) Reject(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.queue == nil {
		return nil
	}

	track := p.queue.Current()
	if track == nil {
		return nil
	}

	// Move file to to_delete (preserve directory structure)
	srcPath := filepath.Join(p.musicDir, filepath.FromSlash(track.Path))
	dstPath := filepath.Join(p.deleteDir, filepath.FromSlash(track.Path))

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("create delete dir: %w", err)
	}
	if err := os.Rename(srcPath, dstPath); err != nil {
		return fmt.Errorf("move file: %w", err)
	}

	slog.Info("rejected track", "path", track.Path)

	// Mark deleted in DB
	p.store.MarkDeleted(ctx, track.ID)

	// Remove from queue and play next
	p.queue.RemoveCurrent()

	if p.queue.Current() == nil {
		if p.transport != nil {
			p.transport.Stop(ctx)
		}
		p.state = StateIdle
		p.stopPollingLocked()
		p.notify()
		return nil
	}

	return p.playCurrentLocked(ctx)
}

// checkPlayCountLocked checks if we've listened long enough to count a play.
func (p *Player) checkPlayCountLocked(ctx context.Context) {
	if p.playCounted {
		return
	}

	total := p.accumulatedMs
	if p.state == StatePlaying {
		total += time.Since(p.playStartTime).Milliseconds()
	}

	if total >= 60_000 {
		track := p.queue.Current()
		if track != nil {
			p.store.IncrementPlayCount(ctx, track.ID)
			slog.Info("play count incremented", "track_id", track.ID, "title", track.Title)
		}
		p.playCounted = true
	}
}

// Polling goroutine to track position and detect track end.
func (p *Player) startPollingLocked(ctx context.Context) {
	p.stopPollingLocked()

	pollCtx, cancel := context.WithCancel(ctx)
	p.pollCancel = cancel

	go p.pollLoop(pollCtx)
}

func (p *Player) stopPollingLocked() {
	if p.pollCancel != nil {
		p.pollCancel()
		p.pollCancel = nil
	}
}

func (p *Player) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollOnce(ctx)
		}
	}
}

func (p *Player) pollOnce(ctx context.Context) {
	p.mu.Lock()
	transport := p.transport
	state := p.state
	p.mu.Unlock()

	if transport == nil || state == StateIdle {
		return
	}

	// Get position from renderer
	pos, err := transport.GetPosition(ctx)
	if err != nil {
		slog.Debug("poll position error", "err", err)
		return
	}

	// Get transport state
	tState, err := transport.GetState(ctx)
	if err != nil {
		slog.Debug("poll state error", "err", err)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.positionMs = pos.RelTime.Milliseconds()
	p.durationMs = pos.TrackDuration.Milliseconds()

	// Check if track ended naturally
	if tState == dlna.StateStopped && p.state == StatePlaying {
		slog.Info("track ended naturally")
		p.checkPlayCountLocked(ctx)

		if p.queue.Next() == nil {
			p.state = StateIdle
			p.stopPollingLocked()
			p.notify()
			return
		}
		p.playCurrentLocked(ctx)
		return
	}

	p.notify()
}
