package dlna

import (
	"context"
	"time"
)

// Transporter is the interface for controlling a DLNA renderer.
// Implemented by Transport; can be mocked in tests.
type Transporter interface {
	SetURI(ctx context.Context, uri, metadata string) error
	Play(ctx context.Context) error
	Stop(ctx context.Context) error
	Pause(ctx context.Context) error
	Seek(ctx context.Context, position time.Duration) error
	GetState(ctx context.Context) (TransportState, error)
	GetPosition(ctx context.Context) (*PositionInfo, error)
}
