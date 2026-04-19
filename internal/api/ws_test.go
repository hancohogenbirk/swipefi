package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"swipefi/internal/player"
)

func TestBroadcast_CompletesWithSlowClient(t *testing.T) {
	hub := NewHub(nil)

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

// TestWS_DeadClientRemovedByPingTimeout verifies that a client which never
// responds to ping frames is evicted from the hub after the pong-wait
// timeout expires. Without this, dead mobile clients linger forever and the
// hub's conns map leaks.
func TestWS_DeadClientRemovedByPingTimeout(t *testing.T) {
	// Shorten timings so the test runs fast.
	origPong, origPing := pongWait, pingInterval
	pongWait = 300 * time.Millisecond
	pingInterval = 100 * time.Millisecond
	defer func() {
		pongWait = origPong
		pingInterval = origPing
	}()

	hub := NewHub(nil)
	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	deadConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial dead client: %v", err)
	}
	// Override the default pong handler with one that never replies.
	// Gorilla's dialer installs an automatic pong reply; we suppress it
	// by swapping in a no-op ping handler.
	deadConn.SetPingHandler(func(string) error { return nil })
	defer deadConn.Close()

	// Give the hub time to register.
	waitUntil(t, 500*time.Millisecond, func() bool {
		return hub.ConnCount() == 1
	}, "hub did not register client")

	// Start a background reader so the gorilla read loop processes control
	// frames locally, but it won't auto-reply to pings because we suppressed
	// the ping handler.
	go func() {
		for {
			if _, _, err := deadConn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// After pongWait + a small margin, the hub must have evicted the conn.
	deadline := time.Now().Add(pongWait + 500*time.Millisecond)
	for time.Now().Before(deadline) {
		if hub.ConnCount() == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("dead client not evicted after pong timeout, conns=%d", hub.ConnCount())
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

// TestWS_InitialStateSentOnConnect verifies that a newly connected client
// receives the current player state immediately, without having to wait for
// the next broadcast. Without this, a client that connects while playback
// is idle learns nothing until state changes.
func TestWS_InitialStateSentOnConnect(t *testing.T) {
	want := player.PlayerState{
		State:      "playing",
		PositionMs: 12345,
		DurationMs: 678000,
	}
	hub := NewHub(func() player.PlayerState { return want })

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read initial message: %v", err)
	}

	var got player.PlayerState
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.State != want.State {
		t.Errorf("state = %q, want %q", got.State, want.State)
	}
	if got.PositionMs != want.PositionMs {
		t.Errorf("positionMs = %d, want %d", got.PositionMs, want.PositionMs)
	}
	if got.DurationMs != want.DurationMs {
		t.Errorf("durationMs = %d, want %d", got.DurationMs, want.DurationMs)
	}
}
