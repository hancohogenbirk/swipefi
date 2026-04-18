package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"swipefi/internal/player"
)

func TestBroadcast_CompletesWithSlowClient(t *testing.T) {
	hub := NewHub()

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	// Connect a client but never read — simulates a stuck/slow client
	slowConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial slow client: %v", err)
	}
	defer slowConn.Close()

	// Connect a fast client that reads messages
	fastConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial fast client: %v", err)
	}
	defer fastConn.Close()

	time.Sleep(50 * time.Millisecond) // Let connections register

	// Broadcast many messages — must complete without blocking indefinitely
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			hub.Broadcast(player.PlayerState{State: "playing"})
		}
		close(done)
	}()

	select {
	case <-done:
		// Good — broadcast completed
	case <-time.After(10 * time.Second):
		t.Fatal("Broadcast blocked for >10s — likely stuck on slow client")
	}

	// Fast client should receive at least one message
	fastConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = fastConn.ReadMessage()
	if err != nil {
		t.Errorf("fast client should receive messages: %v", err)
	}
}
