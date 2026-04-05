package dlna

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/huin/goupnp/dcps/av1"
)

type Device struct {
	Name     string `json:"name"`
	UDN      string `json:"udn"`
	Location string `json:"location"`
}

type Renderer struct {
	Device
	Transport *av1.AVTransport1
}

type Discovery struct {
	mu        sync.RWMutex
	renderers map[string]*Renderer
}

func NewDiscovery() *Discovery {
	return &Discovery{
		renderers: make(map[string]*Renderer),
	}
}

func (d *Discovery) Scan(ctx context.Context) error {
	slog.Info("scanning for DLNA renderers")

	clients, errs, err := av1.NewAVTransport1ClientsCtx(ctx)
	if err != nil {
		return fmt.Errorf("discover renderers: %w", err)
	}
	for _, e := range errs {
		if e != nil {
			slog.Warn("discovery error", "err", e)
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.renderers = make(map[string]*Renderer)
	for _, client := range clients {
		root := client.RootDevice
		udn := root.Device.UDN
		name := root.Device.FriendlyName
		loc := client.Location.String()

		slog.Info("found renderer", "name", name, "udn", udn)

		d.renderers[udn] = &Renderer{
			Device: Device{
				Name:     name,
				UDN:      udn,
				Location: loc,
			},
			Transport: client,
		}
	}

	slog.Info("discovery complete", "renderers", len(d.renderers))
	return nil
}

func (d *Discovery) ListDevices() []Device {
	d.mu.RLock()
	defer d.mu.RUnlock()

	devices := make([]Device, 0, len(d.renderers))
	for _, r := range d.renderers {
		devices = append(devices, r.Device)
	}
	return devices
}

func (d *Discovery) GetRenderer(udn string) (*Renderer, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	r, ok := d.renderers[udn]
	return r, ok
}

// GetLocalIP returns the first non-loopback IPv4 address on this machine.
// Needed to construct stream URLs that DLNA renderers can reach.
func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no suitable network interface found")
}
