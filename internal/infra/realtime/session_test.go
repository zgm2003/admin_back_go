package realtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSessionSendDropsConnectionWhenQueueIsFull(t *testing.T) {
	session := NewSession(nil, SessionOptions{SendBuffer: 1})

	first := mustRealtimeEnvelope(t, "realtime.notice.v1", "rid-1")
	if err := session.Send(first); err != nil {
		t.Fatalf("first Send returned error: %v", err)
	}

	second := mustRealtimeEnvelope(t, "realtime.notice.v1", "rid-2")
	err := session.Send(second)
	if !errors.Is(err, ErrSendQueueFull) {
		t.Fatalf("expected ErrSendQueueFull, got %v", err)
	}

	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("session was not closed after send queue overflow")
	}
}

func TestManagerRegistersReplacesSendsAndUnregistersSessions(t *testing.T) {
	manager := NewManager()
	first := NewSession(nil, SessionOptions{SendBuffer: 1})
	unregisterFirst := manager.Register("admin:7:9", first)

	if got := manager.Count(); got != 1 {
		t.Fatalf("expected one registered session, got %d", got)
	}

	replacement := NewSession(nil, SessionOptions{SendBuffer: 1})
	unregisterReplacement := manager.Register("admin:7:9", replacement)

	select {
	case <-first.Done():
	case <-time.After(time.Second):
		t.Fatal("first session was not closed when replaced")
	}
	if got := manager.Count(); got != 1 {
		t.Fatalf("expected replacement to keep one registered session, got %d", got)
	}

	message := mustRealtimeEnvelope(t, "realtime.notice.v1", "rid-1")
	if err := manager.Send("admin:7:9", message); err != nil {
		t.Fatalf("manager Send returned error: %v", err)
	}

	unregisterFirst()
	if got := manager.Count(); got != 1 {
		t.Fatalf("stale unregister removed replacement session, count=%d", got)
	}

	unregisterReplacement()
	if got := manager.Count(); got != 0 {
		t.Fatalf("expected no registered sessions after unregister, got %d", got)
	}

	if err := manager.Send("admin:7:9", message); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound after unregister, got %v", err)
	}
}

func TestSessionServeUsesWritePumpAndRepliesToPing(t *testing.T) {
	serverDone := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := NewUpgrader(func(*http.Request) bool { return true }).Upgrade(w, r)
		if err != nil {
			serverDone <- err
			return
		}

		session := NewSession(conn, SessionOptions{
			SendBuffer:   2,
			WriteWait:    time.Second,
			PongWait:     5 * time.Second,
			PingInterval: time.Hour,
		})
		connected := mustRealtimeEnvelope(t, "realtime.connected.v1", "rid-connected")
		if err := session.Send(connected); err != nil {
			serverDone <- err
			return
		}
		serverDone <- session.Serve(context.Background(), func(ctx context.Context, envelope Envelope) (*Envelope, error) {
			if envelope.Type != "realtime.ping.v1" {
				t.Fatalf("unexpected envelope type: %s", envelope.Type)
			}
			pong := mustRealtimeEnvelope(t, "realtime.pong.v1", envelope.RequestID)
			return &pong, nil
		})
	}))
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):], nil)
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}

	var connected Envelope
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected.Type != "realtime.connected.v1" {
		t.Fatalf("unexpected connected event: %#v", connected)
	}

	if err := client.WriteJSON(map[string]any{
		"type":       "realtime.ping.v1",
		"request_id": "rid-ping",
		"data":       map[string]any{},
	}); err != nil {
		t.Fatalf("write ping: %v", err)
	}

	var pong Envelope
	if err := client.ReadJSON(&pong); err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if pong.Type != "realtime.pong.v1" || pong.RequestID != "rid-ping" {
		t.Fatalf("unexpected pong: %#v", pong)
	}

	if err := client.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye")); err != nil {
		t.Fatalf("write close: %v", err)
	}
	_ = client.Close()

	select {
	case err := <-serverDone:
		if err != nil && !errors.Is(err, ErrConnectionClosed) {
			t.Fatalf("Serve returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish after client close")
	}
}

func mustRealtimeEnvelope(t *testing.T, typ string, requestID string) Envelope {
	t.Helper()
	envelope, err := NewEnvelope(typ, requestID, map[string]any{})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	return envelope
}
