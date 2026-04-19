package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"swipefi/internal/player"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Default ping/pong timing. Package-level vars (not consts) so tests can
// override the defaults before constructing a Hub. The values captured at
// NewHub() are stored on the Hub itself, so concurrent goroutines never
// race on these package vars.
var (
	pongWait     = 60 * time.Second
	pingInterval = (pongWait * 9) / 10
	writeWait    = 5 * time.Second
)

// wsClient wraps a websocket connection with a per-conn write mutex so
// ping writes (from the ping ticker goroutine) and broadcast writes
// (from Hub.Broadcast) don't interleave on the underlying socket.
type wsClient struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	done      chan struct{}
	writeWait time.Duration
}

// writeMessage serialises all writes to the connection.
func (c *wsClient) writeMessage(msgType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(c.writeWait))
	return c.conn.WriteMessage(msgType, data)
}

// Hub manages WebSocket connections and broadcasts player state.
type Hub struct {
	mu       sync.Mutex
	conns    map[*websocket.Conn]*wsClient
	getState func() player.PlayerState
	// Timing captured from package vars at construction so per-Hub
	// goroutines never race on package-level overrides from tests.
	pongWait     time.Duration
	pingInterval time.Duration
	writeWait    time.Duration
}

// NewHub creates a Hub. getState may be nil; when non-nil it is called on
// every new connection to send the current player state immediately.
func NewHub(getState func() player.PlayerState) *Hub {
	return &Hub{
		conns:        make(map[*websocket.Conn]*wsClient),
		getState:     getState,
		pongWait:     pongWait,
		pingInterval: pingInterval,
		writeWait:    writeWait,
	}
}

// ConnCount returns the number of registered connections. Test helper.
func (h *Hub) ConnCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.conns)
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade", "err", err)
		return
	}

	client := &wsClient{
		conn:      conn,
		done:      make(chan struct{}),
		writeWait: h.writeWait,
	}

	// Install pong handler BEFORE spawning goroutines so the very first
	// pong resets the deadline correctly.
	pongWait := h.pongWait
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	h.mu.Lock()
	h.conns[conn] = client
	h.mu.Unlock()

	slog.Info("websocket connected", "remote", conn.RemoteAddr())

	// Send current state immediately so the client doesn't have to wait
	// for the next broadcast.
	if h.getState != nil {
		if data, err := json.Marshal(h.getState()); err == nil {
			if err := client.writeMessage(websocket.TextMessage, data); err != nil {
				slog.Debug("initial state write failed", "err", err)
			}
		}
	}

	// Ping ticker — drops the conn if a write fails (which implies the
	// pong-wait deadline has also likely already been missed).
	go h.pingLoop(client)

	// Read loop — drains client messages and also processes pong frames.
	// When ReadMessage returns an error (pong timeout or client close),
	// we tear down.
	go func() {
		defer h.removeClient(client)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (h *Hub) pingLoop(client *wsClient) {
	ticker := time.NewTicker(h.pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-client.done:
			return
		case <-ticker.C:
			if err := client.writeMessage(websocket.PingMessage, nil); err != nil {
				// Write failed — the read goroutine will also see an
				// error shortly and remove the client.
				return
			}
		}
	}
}

func (h *Hub) removeClient(client *wsClient) {
	h.mu.Lock()
	if _, ok := h.conns[client.conn]; ok {
		delete(h.conns, client.conn)
		close(client.done)
	}
	h.mu.Unlock()
	client.conn.Close()
	slog.Info("websocket disconnected", "remote", client.conn.RemoteAddr())
}

// Broadcast sends the player state to all connected WebSocket clients.
func (h *Hub) Broadcast(state player.PlayerState) {
	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	// Snapshot clients under the hub mutex, then write outside the lock
	// so a slow client can't block broadcasts to healthy clients. Per-
	// client writeMu still serialises ping/broadcast writes on each conn.
	h.mu.Lock()
	clients := make([]*wsClient, 0, len(h.conns))
	for _, c := range h.conns {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	for _, c := range clients {
		if err := c.writeMessage(websocket.TextMessage, data); err != nil {
			h.removeClient(c)
		}
	}
}
