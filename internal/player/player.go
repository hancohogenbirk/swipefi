package player

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"swipefi/internal/dlna"
	"swipefi/internal/library"
	"swipefi/internal/store"
)

type State string

const (
	StateIdle    State = "idle"
	StateLoading State = "loading"
	StatePlaying State = "playing"
	StatePaused  State = "paused"
)

const (
	playCountThresholdMs = 60_000
	dlnaRetryDelay       = 500 * time.Millisecond
	disconnectTimeout    = 30 * time.Second
)

// PlayerState is the full state broadcast to WebSocket clients.
type PlayerState struct {
	State         State        `json:"state"`
	Connected     bool         `json:"connected"`
	Reconnecting  bool         `json:"reconnecting"`
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

	transport dlna.Transporter
	queue     *Queue
	state     State

	// Play time tracking for the 60-second threshold
	playStartTime  time.Time
	accumulatedMs  int64
	playCounted    bool

	// Timestamp when playCurrentLocked last called Play() on the renderer.
	// Used to suppress false "track ended" detections during renderer startup.
	playStartedAt time.Time

	// Expected stream URL for the current track
	currentStreamURL string

	// Current position from renderer
	positionMs int64
	durationMs int64

	// Time of first consecutive poll error — disconnect after disconnectTimeout
	firstPollErrorAt time.Time

	// Queue metadata for recovery after disconnect
	queueFolder    string
	queueSortBy    string
	queueSortOrder string

	// Fast polling after track change to detect playback start quickly
	aggressivePollUntil time.Time

	// Polling
	pollCancel context.CancelFunc

	// Long-lived app context for background work (polling, etc.)
	appCtx context.Context

	// Reconnecting flag — set on auto-disconnect, cleared on SetTransport
	reconnecting bool

	// Callback fired on auto-disconnect (for reconnect loop)
	onDisconnect func()

	// State change callback
	onChange StateChangeFunc
}

func New(ctx context.Context, s *store.Store, musicDir, deleteDir, port string) (*Player, error) {
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
		appCtx:    ctx,
	}, nil
}

func (p *Player) SetOnChange(fn StateChangeFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onChange = fn
}

func (p *Player) SetOnDisconnect(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onDisconnect = fn
}

func (p *Player) SetDirs(musicDir, deleteDir string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.musicDir = musicDir
	p.deleteDir = deleteDir
}

func (p *Player) SetTransport(t dlna.Transporter) {
	p.mu.Lock()
	p.transport = t
	p.firstPollErrorAt = time.Time{}
	if t != nil {
		p.reconnecting = false
		p.startPollingLocked(p.appCtx)
		// Try to recover the renderer's current playback state
		appCtx := p.appCtx
		p.mu.Unlock()
		p.recoverRendererState(appCtx, t)
	} else {
		p.stopPollingLocked()
		p.mu.Unlock()
	}
}

// recoverRendererState checks what the renderer is currently playing and
// rebuilds the player queue to match. This is called on (re)connect so the
// UI reflects the actual renderer state.
func (p *Player) recoverRendererState(ctx context.Context, transport dlna.Transporter) {
	// Don't clobber an existing queue — only recover when idle
	p.mu.Lock()
	hasQueue := p.queue != nil
	savedFolder := p.queueFolder
	savedSortBy := p.queueSortBy
	savedSortOrder := p.queueSortOrder
	p.mu.Unlock()
	if hasQueue {
		return
	}

	tState, err := transport.GetState(ctx)
	if err != nil {
		slog.Debug("recoverRendererState: cannot get state", "err", err)
		return
	}
	// Only recover if the renderer is actively playing or paused
	if tState != dlna.StatePlaying && tState != dlna.StatePaused {
		return
	}

	pos, err := transport.GetPosition(ctx)
	if err != nil {
		slog.Debug("recoverRendererState: cannot get position", "err", err)
		return
	}
	if pos.TrackURI == "" {
		return
	}

	// Extract track path from stream URL: http://<ip>:<port>/stream/<path>
	trackPath := extractTrackPath(pos.TrackURI, p.localIP, p.port)
	if trackPath == "" {
		slog.Debug("recoverRendererState: cannot parse track path from URI", "uri", pos.TrackURI)
		return
	}

	track, err := p.store.GetTrackByPath(ctx, trackPath)
	if err != nil {
		slog.Debug("recoverRendererState: track not found in DB", "path", trackPath, "err", err)
		return
	}

	// Build the queue from the saved folder metadata if available,
	// otherwise fall back to the track's parent folder.
	folder := savedFolder
	sortBy := savedSortBy
	sortOrder := savedSortOrder
	if folder == "" {
		folder = filepath.Dir(trackPath)
		sortBy = "added_at"
		sortOrder = "asc"
	}
	folderTracks, err := p.store.ListTracks(ctx, folder, sortBy, sortOrder)
	if err != nil || len(folderTracks) == 0 {
		// Fallback: single-track queue
		folderTracks = []store.Track{*track}
	}

	// Find the position of the current track in the folder queue
	trackPos := 0
	for i, t := range folderTracks {
		if t.ID == track.ID {
			trackPos = i
			break
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.queue = NewQueue(folderTracks)
	// Advance the queue to the current track position
	for i := 0; i < trackPos; i++ {
		p.queue.Next()
	}
	p.currentStreamURL = pos.TrackURI
	p.positionMs = pos.RelTime.Milliseconds()
	p.durationMs = pos.TrackDuration.Milliseconds()
	p.playStartedAt = time.Now().Add(-pos.RelTime) // approximate

	if tState == dlna.StatePlaying {
		p.state = StatePlaying
		p.playStartTime = time.Now()
	} else {
		p.state = StatePaused
	}

	slog.Info("recovered renderer state", "track", track.Title, "state", tState,
		"position", pos.RelTime, "duration", pos.TrackDuration,
		"queue_len", len(folderTracks), "queue_pos", trackPos)
	p.notify()
}

// extractTrackPath extracts the relative track path from a SwipeFi stream URL.
// Returns empty string if the URL doesn't match the expected pattern.
func extractTrackPath(uri, localIP, port string) string {
	prefix := fmt.Sprintf("http://%s:%s/stream/", localIP, port)
	if strings.HasPrefix(uri, prefix) {
		path := strings.TrimPrefix(uri, prefix)
		// DLNA renderers may percent-encode the URI when reporting it back
		if decoded, err := url.PathUnescape(path); err == nil {
			return decoded
		}
		return path
	}
	return ""
}

// HasTransport returns true if a DLNA transport is currently set.
func (p *Player) HasTransport() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.transport != nil
}

// Disconnect stops playback and clears the transport.
func (p *Player) Disconnect() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.transport != nil {
		stopCtx, cancel := context.WithTimeout(p.appCtx, 5*time.Second)
		defer cancel()
		if err := p.transport.Stop(stopCtx); err != nil {
			slog.Warn("disconnect stop failed", "err", err)
		}
	}
	p.stopPollingLocked()
	p.state = StateIdle
	p.transport = nil
	p.queue = nil
	p.currentStreamURL = ""
	p.queueFolder = ""
	p.queueSortBy = ""
	p.queueSortOrder = ""
	p.notify()
}

// ClearReconnecting clears the reconnecting flag and residual queue state
// after an auto-reconnect attempt fails completely.
func (p *Player) ClearReconnecting() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reconnecting = false
	p.queue = nil
	p.queueFolder = ""
	p.queueSortBy = ""
	p.queueSortOrder = ""
	p.currentStreamURL = ""
	p.notify()
}

func (p *Player) GetState() PlayerState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stateLocked()
}

func (p *Player) stateLocked() PlayerState {
	ps := PlayerState{
		State:        p.state,
		Connected:    p.transport != nil,
		Reconnecting: p.reconnecting,
		PositionMs:   p.positionMs,
		DurationMs:   p.durationMs,
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

	p.queueFolder = folder
	p.queueSortBy = sortBy
	p.queueSortOrder = order
	p.queue = NewQueue(tracks)
	return p.playCurrentLocked(ctx)
}

func (p *Player) playCurrentLocked(ctx context.Context) error {
	track := p.queue.Current()
	if track == nil {
		p.state = StateIdle
		p.currentStreamURL = ""
		p.stopPollingLocked()
		p.notify()
		return nil
	}

	// Stop current playback first — DLNA renderers can get confused
	// when receiving a new URI while still playing/buffering another.
	// Use p.appCtx (not ctx) because ctx may be a poll context that
	// gets cancelled by stopPollingLocked below.
	if p.transport != nil && (p.state == StatePlaying || p.state == StatePaused || p.state == StateLoading) {
		p.stopPollingLocked()
		p.transport.Stop(p.appCtx)
		time.Sleep(200 * time.Millisecond)
	}

	streamURL := fmt.Sprintf("http://%s:%s/stream/%s", p.localIP, p.port, track.Path)
	p.currentStreamURL = streamURL
	slog.Info("playing track", "title", track.Title, "artist", track.Artist, "url", streamURL)

	// Pre-warm art cache so renderer and frontend see consistent art
	artURL := fmt.Sprintf("http://%s:%s/api/tracks/%d/art", p.localIP, p.port, track.ID)
	artResp, artErr := http.Get(artURL)
	if artErr == nil {
		artResp.Body.Close()
	}

	// Build DIDL-Lite metadata so the renderer shows track info and art
	metadata := buildDIDLMetadata(track, streamURL, artURL)

	// Set state to loading BEFORE sending Play command
	p.state = StateLoading
	p.positionMs = 0
	p.durationMs = 0
	p.playStartTime = time.Now()
	p.accumulatedMs = 0
	p.playCounted = false
	p.notify()

	// Try play with one retry — DLNA renderers sometimes return EOF on first attempt.
	// Use p.appCtx so transport calls survive poll-context cancellation.
	if err := p.tryPlayWithRetry(p.appCtx, streamURL, metadata); err != nil {
		slog.Error("play failed after retry", "track", track.Title, "err", err)
		return err
	}

	p.playStartedAt = time.Now()
	slog.Debug("playCurrentLocked: reset playCounted", "track_id", track.ID, "title", track.Title)

	// Check if renderer already started — skip the loading state entirely for fast renderers
	if tState, err := p.transport.GetState(p.appCtx); err == nil && tState == dlna.StatePlaying {
		p.state = StatePlaying
		p.playStartTime = time.Now()
	}

	p.startPollingLocked(p.appCtx)
	p.aggressivePollUntil = time.Now().Add(10 * time.Second)
	p.notify()
	return nil
}

// tryPlayWithRetry attempts SetURI + Play with one retry on failure.
func (p *Player) tryPlayWithRetry(ctx context.Context, streamURL, metadata string) error {
	for attempt := 0; attempt < 2; attempt++ {
		err := p.transport.SetURI(ctx, streamURL, metadata)
		if err != nil {
			slog.Debug("SetURI failed", "err", err, "attempt", attempt+1)
		}
		if err == nil {
			err = p.transport.Play(ctx)
			if err != nil {
				slog.Debug("Play failed", "err", err, "attempt", attempt+1)
			}
		}
		if err == nil {
			return nil
		}
		if attempt == 0 {
			slog.Warn("play failed, retrying", "err", err, "attempt", attempt+1)
			time.Sleep(dlnaRetryDelay)
			continue
		}
		return fmt.Errorf("play: %w", err)
	}
	return nil
}

// buildDIDLMetadata creates DIDL-Lite XML for UPnP renderers to display track info.
func buildDIDLMetadata(track *store.Track, streamURL, artURL string) string {
	escape := func(s string) string {
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		s = strings.ReplaceAll(s, "\"", "&quot;")
		return s
	}

	title := escape(track.Title)
	artist := escape(track.Artist)
	album := escape(track.Album)

	mime := "audio/flac"
	switch strings.ToLower(track.Format) {
	case "mp3":
		mime = "audio/mpeg"
	case "wav":
		mime = "audio/wav"
	case "aiff":
		mime = "audio/aiff"
	case "ogg":
		mime = "audio/ogg"
	case "aac", "m4a":
		mime = "audio/mp4"
	case "dsf":
		mime = "audio/dsf"
	case "dff":
		mime = "audio/dff"
	}

	return `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">` +
		`<item id="0" parentID="-1" restricted="1">` +
		`<dc:title>` + title + `</dc:title>` +
		`<dc:creator>` + artist + `</dc:creator>` +
		`<upnp:artist>` + artist + `</upnp:artist>` +
		`<upnp:album>` + album + `</upnp:album>` +
		`<upnp:albumArtURI>` + escape(artURL) + `</upnp:albumArtURI>` +
		`<res protocolInfo="http-get:*:` + mime + `:*">` + escape(streamURL) + `</res>` +
		`</item></DIDL-Lite>`
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

	if cur := p.queue.Current(); cur != nil {
		slog.Debug("Next called", "current_track", cur.Title, "playCounted", p.playCounted)
	}
	p.checkPlayCountLocked(ctx, true) // user explicitly skipped → always count

	if p.queue.Next() == nil {
		// End of queue
		if p.transport != nil {
			p.transport.Stop(ctx)
		}
		p.state = StateIdle
		p.currentStreamURL = ""
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

	p.checkPlayCountLocked(ctx, false) // going back → use 60s threshold

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

	// Clean up empty source directory
	library.CleanupEmptyDirs(filepath.Dir(srcPath), p.musicDir)

	slog.Info("rejected track", "path", track.Path)

	// Mark deleted in DB
	if err := p.store.MarkDeleted(ctx, track.ID); err != nil {
		slog.Warn("failed to mark track deleted in db", "track_id", track.ID, "err", err)
	}

	// Remove from queue and play next
	p.queue.RemoveCurrent()

	if p.queue.Current() == nil {
		if p.transport != nil {
			p.transport.Stop(p.appCtx) // Use appCtx to survive HTTP context cancellation
		}
		p.state = StateIdle
		p.currentStreamURL = ""
		p.stopPollingLocked()
		p.notify()
		return nil
	}

	return p.playCurrentLocked(ctx)
}

// GetQueue returns the current queue tracks and position.
func (p *Player) GetQueue() ([]store.Track, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.queue == nil {
		return nil, 0
	}
	return p.queue.Tracks(), p.queue.Position()
}

// ReorderQueue sets a new track order by IDs.
func (p *Player) ReorderQueue(ids []int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.queue == nil {
		return
	}
	p.queue.Reorder(ids)
	p.notify()
}

// SkipToTrack jumps to a specific track in the queue, removing all before it.
func (p *Player) SkipToTrack(ctx context.Context, trackID int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.queue == nil {
		return fmt.Errorf("no queue")
	}

	slog.Debug("SkipToTrack called", "target_id", trackID, "playCounted", p.playCounted)
	p.checkPlayCountLocked(ctx, true)

	if !p.queue.SkipTo(trackID) {
		return fmt.Errorf("track not in queue")
	}

	return p.playCurrentLocked(ctx)
}

// checkPlayCountLocked increments play count if not already counted.
// force=true always counts (user skipped/swiped). force=false only counts after 60s.
func (p *Player) checkPlayCountLocked(ctx context.Context, force bool) {
	slog.Debug("checkPlayCountLocked", "playCounted", p.playCounted, "force", force)
	if p.playCounted {
		return
	}

	shouldCount := force
	if !shouldCount {
		total := p.accumulatedMs
		if p.state == StatePlaying {
			total += time.Since(p.playStartTime).Milliseconds()
		}
		shouldCount = total >= playCountThresholdMs
	}

	if shouldCount {
		track := p.queue.Current()
		if track != nil {
			if err := p.store.IncrementPlayCount(ctx, track.ID); err != nil {
				slog.Warn("failed to increment play count", "track_id", track.ID, "err", err)
			}
			track.PlayCount++
			if p.queue != nil {
				p.queue.UpdateCurrentPlayCount(track.PlayCount)
			}
			slog.Info("play count incremented", "track_id", track.ID, "title", track.Title, "new_count", track.PlayCount)
		}
		p.playCounted = true
		p.notify()
	}
}

// Polling goroutine to track position and detect track end.
// Uses the long-lived appCtx, not the HTTP request context.
func (p *Player) startPollingLocked(_ context.Context) {
	p.stopPollingLocked()

	pollCtx, cancel := context.WithCancel(p.appCtx)
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
	for {
		p.mu.Lock()
		interval := 1 * time.Second
		if time.Now().Before(p.aggressivePollUntil) {
			interval = 200 * time.Millisecond
		}
		state := p.state
		p.mu.Unlock()

		// Idle state polls less frequently
		if state == StateIdle {
			interval = 5 * time.Second
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			p.pollOnce(ctx)
		}
	}
}

func (p *Player) heartbeatCheck(ctx context.Context, transport dlna.Transporter) {
	_, err := transport.GetState(ctx)
	if err != nil {
		slog.Debug("heartbeat error", "err", err)
		p.mu.Lock()
		if p.firstPollErrorAt.IsZero() {
			p.firstPollErrorAt = time.Now()
		} else if time.Since(p.firstPollErrorAt) > disconnectTimeout {
			slog.Warn("device unreachable (idle heartbeat), disconnecting", "error_duration", time.Since(p.firstPollErrorAt))
			p.stopPollingLocked()
			p.state = StateIdle
			p.transport = nil
			p.firstPollErrorAt = time.Time{}
			p.reconnecting = true
			p.notify()
			onDisconnect := p.onDisconnect
			p.mu.Unlock()
			if onDisconnect != nil {
				go onDisconnect()
			}
			return
		}
		p.mu.Unlock()
		return
	}
	p.mu.Lock()
	p.firstPollErrorAt = time.Time{}
	p.mu.Unlock()
}

func (p *Player) pollOnce(ctx context.Context) {
	p.mu.Lock()
	transport := p.transport
	state := p.state
	p.mu.Unlock()

	if transport == nil {
		return
	}
	if state == StateIdle {
		p.heartbeatCheck(ctx, transport)
		return
	}

	// Get position from renderer
	pos, err := transport.GetPosition(ctx)
	if err != nil {
		slog.Debug("poll position error", "err", err)
		p.mu.Lock()
		if p.firstPollErrorAt.IsZero() {
			p.firstPollErrorAt = time.Now()
		} else if time.Since(p.firstPollErrorAt) > disconnectTimeout {
			slog.Warn("device unreachable, disconnecting", "error_duration", time.Since(p.firstPollErrorAt))
			p.stopPollingLocked()
			p.state = StateIdle
			p.transport = nil
			p.firstPollErrorAt = time.Time{}
			p.reconnecting = true
			p.notify()
			onDisconnect := p.onDisconnect
			p.mu.Unlock()
			if onDisconnect != nil {
				go onDisconnect()
			}
			return
		}
		p.mu.Unlock()
		return
	}

	// Get transport state
	tState, err := transport.GetState(ctx)
	if err != nil {
		slog.Debug("poll state error", "err", err)
		p.mu.Lock()
		if p.firstPollErrorAt.IsZero() {
			p.firstPollErrorAt = time.Now()
		} else if time.Since(p.firstPollErrorAt) > disconnectTimeout {
			slog.Warn("device unreachable, disconnecting", "error_duration", time.Since(p.firstPollErrorAt))
			p.stopPollingLocked()
			p.state = StateIdle
			p.transport = nil
			p.firstPollErrorAt = time.Time{}
			p.reconnecting = true
			p.notify()
			onDisconnect := p.onDisconnect
			p.mu.Unlock()
			if onDisconnect != nil {
				go onDisconnect()
			}
			return
		}
		p.mu.Unlock()
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.firstPollErrorAt = time.Time{}

	// Don't update position during loading — renderer may report stale data
	// from the previous track while transitioning. Also skip the external
	// source check since the URI may be stale during loading.
	if p.state != StateLoading {
		p.positionMs = pos.RelTime.Milliseconds()
		p.durationMs = pos.TrackDuration.Milliseconds()

		// Check if another source took over the device
		if pos.TrackURI != "" && p.currentStreamURL != "" && pos.TrackURI != p.currentStreamURL {
			slog.Info("external source took over device", "expected", p.currentStreamURL, "actual", pos.TrackURI)
			p.state = StateIdle
			p.currentStreamURL = ""
			p.stopPollingLocked()
			p.notify()
			return
		}
	}

	gracePeriod := 5 * time.Second

	// Handle StateLoading → StatePlaying transition
	if p.state == StateLoading {
		if tState == dlna.StatePlaying {
			slog.Info("renderer started playing", "track", p.queue.Current().Title)
			p.state = StatePlaying
			p.playStartTime = time.Now()
			p.notify()
			return
		}
		// Renderer is transitioning — don't reset the grace period timer
		if tState == "TRANSITIONING" {
			slog.Debug("renderer transitioning")
			p.notify()
			return
		}
		// Still loading — check if grace period expired
		if time.Since(p.playStartedAt) > gracePeriod {
			track := p.queue.Current()
			slog.Warn("track failed to start within grace period",
				"track", track.Title, "path", track.Path, "format", track.Format,
				"renderer_state", tState)
			// One more retry before giving up
			slog.Info("retrying play after grace period timeout")
			streamURL := p.currentStreamURL
			artURL := fmt.Sprintf("http://%s:%s/api/tracks/%d/art", p.localIP, p.port, track.ID)
			metadata := buildDIDLMetadata(track, streamURL, artURL)
			// Use p.appCtx so transport calls survive poll-context cancellation.
			if err := p.tryPlayWithRetry(p.appCtx, streamURL, metadata); err != nil {
				slog.Error("retry also failed, skipping track", "track", track.Title, "err", err)
				if p.queue.Next() == nil {
					p.state = StateIdle
					p.currentStreamURL = ""
					p.stopPollingLocked()
					p.notify()
					return
				}
				p.playCurrentLocked(p.appCtx)
				return
			}
			p.playStartedAt = time.Now()
		}
		p.notify()
		return
	}

	// Check if track ended naturally (only when actually playing, not loading)
	if tState == dlna.StateStopped && p.state == StatePlaying {
		// Grace period: ignore STOPPED shortly after play started
		if time.Since(p.playStartedAt) < gracePeriod {
			slog.Debug("ignoring STOPPED during grace period", "elapsed", time.Since(p.playStartedAt))
			return
		}
		// Require that we've received a real duration from the renderer
		if p.durationMs == 0 {
			slog.Debug("ignoring STOPPED with zero duration")
			return
		}

		slog.Info("track ended naturally")
		p.checkPlayCountLocked(p.appCtx, true) // finished naturally → count

		if p.queue.Next() == nil {
			p.state = StateIdle
			p.currentStreamURL = ""
			p.stopPollingLocked()
			p.notify()
			return
		}
		// Use p.appCtx so transport calls survive poll-context cancellation.
		p.playCurrentLocked(p.appCtx)
		return
	}

	// Check playcount threshold during active playback
	if p.state == StatePlaying && !p.playCounted {
		p.checkPlayCountLocked(p.appCtx, false)
	}

	p.notify()
}
