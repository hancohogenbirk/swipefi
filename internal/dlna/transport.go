package dlna

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/huin/goupnp/dcps/av1"
)

// TransportState represents the UPnP AVTransport state.
type TransportState string

const (
	StatePlaying TransportState = "PLAYING"
	StatePaused  TransportState = "PAUSED_PLAYBACK"
	StateStopped TransportState = "STOPPED"
	StateNoMedia TransportState = "NO_MEDIA_PRESENT"
)

// PositionInfo holds the current playback position.
type PositionInfo struct {
	TrackDuration time.Duration
	RelTime       time.Duration
	TrackURI      string
}

// Transport wraps an AVTransport1 client with higher-level methods.
type Transport struct {
	client *av1.AVTransport1
}

func NewTransport(client *av1.AVTransport1) *Transport {
	return &Transport{client: client}
}

func (t *Transport) SetURI(ctx context.Context, uri, metadata string) error {
	return t.client.SetAVTransportURICtx(ctx, 0, uri, metadata)
}

func (t *Transport) Play(ctx context.Context) error {
	return t.client.PlayCtx(ctx, 0, "1")
}

func (t *Transport) Stop(ctx context.Context) error {
	return t.client.StopCtx(ctx, 0)
}

func (t *Transport) Pause(ctx context.Context) error {
	return t.client.PauseCtx(ctx, 0)
}

func (t *Transport) Seek(ctx context.Context, position time.Duration) error {
	target := formatDuration(position)
	return t.client.SeekCtx(ctx, 0, "REL_TIME", target)
}

func (t *Transport) GetState(ctx context.Context) (TransportState, error) {
	state, _, _, err := t.client.GetTransportInfoCtx(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("get transport info: %w", err)
	}
	return TransportState(state), nil
}

func (t *Transport) GetPosition(ctx context.Context) (*PositionInfo, error) {
	_, trackDur, _, trackURI, relTime, _, _, _, err := t.client.GetPositionInfoCtx(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("get position info: %w", err)
	}

	return &PositionInfo{
		TrackDuration: parseDuration(trackDur),
		RelTime:       parseDuration(relTime),
		TrackURI:      trackURI,
	}, nil
}

// parseDuration parses a UPnP time string like "0:02:30" or "00:02:30" into a time.Duration.
func parseDuration(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" || s == "NOT_IMPLEMENTED" {
		return 0
	}

	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}

	var h, m, sec int
	fmt.Sscanf(parts[0], "%d", &h)
	fmt.Sscanf(parts[1], "%d", &m)

	// Handle fractional seconds like "30.500"
	var frac float64
	fmt.Sscanf(parts[2], "%f", &frac)
	sec = int(frac)
	ms := int((frac - float64(sec)) * 1000)

	return time.Duration(h)*time.Hour +
		time.Duration(m)*time.Minute +
		time.Duration(sec)*time.Second +
		time.Duration(ms)*time.Millisecond
}

// formatDuration formats a time.Duration as "HH:MM:SS" for UPnP Seek.
func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d:%02d", h, m, s)
}
