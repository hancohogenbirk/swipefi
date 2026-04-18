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

// Hub manages WebSocket connections and broadcasts player state.
type Hub struct {
	mu    sync.Mutex
	conns map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{
		conns: make(map[*websocket.Conn]struct{}),
	}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade", "err", err)
		return
	}

	h.mu.Lock()
	h.conns[conn] = struct{}{}
	h.mu.Unlock()

	slog.Info("websocket connected", "remote", conn.RemoteAddr())

	// Read loop — just drain messages (client doesn't send meaningful data)
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.conns, conn)
			h.mu.Unlock()
			conn.Close()
			slog.Info("websocket disconnected", "remote", conn.RemoteAddr())
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

// Broadcast sends the player state to all connected WebSocket clients.
func (h *Hub) Broadcast(state player.PlayerState) {
	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for conn := range h.conns {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			delete(h.conns, conn)
		}
	}
}
