package realtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/telemetry"

	"github.com/gorilla/websocket"
)

func TestSlowClientSendQueueDisconnects(t *testing.T) {
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

func TestResumeReplayWaitsForQueueDrain(t *testing.T) {
	session := NewSession(nil, SessionOptions{SendBuffer: 1})
	defer session.Close()
	first := mustRealtimeEnvelope(t, "notification.created.v1", "rid-first")
	second := mustRealtimeEnvelope(t, "notification.created.v1", "rid-second")
	if err := session.Send(first); err != nil {
		t.Fatalf("prime replay queue: %v", err)
	}
	drained := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		<-session.send
		close(drained)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.SendReplay(ctx, second); err != nil {
		t.Fatalf("SendReplay returned error: %v", err)
	}
	<-drained
	assertSessionQueued(t, session, second)
}

func TestSubscriptionFiltersDeliveryAndReplaceRemovesOldTopics(t *testing.T) {
	manager := NewManager()
	session := NewSession(nil, SessionOptions{SendBuffer: 2})
	defer session.Close()
	manager.Register("admin:7:9", session)
	message := mustRealtimeEnvelope(t, "notification.created.v1", "rid-subscription")

	if err := manager.SendToUser("admin", 7, message); !errors.Is(err, ErrTopicNotSubscribed) {
		t.Fatalf("unsubscribed session received delivery: %v", err)
	}
	select {
	case got := <-session.send:
		t.Fatalf("unsubscribed session queued envelope: %#v", got)
	default:
	}

	session.ReplaceTopics([]string{"user:7", "platform:admin"})
	if err := manager.SendToUser("admin", 7, message); err != nil {
		t.Fatalf("subscribed delivery failed: %v", err)
	}
	assertSessionQueued(t, session, message)

	session.ReplaceTopics([]string{"platform:admin"})
	if session.Subscribed("user:7") {
		t.Fatal("replace retained a removed user topic")
	}
	if err := manager.SendToUser("admin", 7, message); !errors.Is(err, ErrTopicNotSubscribed) {
		t.Fatalf("removed topic still received delivery: %v", err)
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
	replacement.ReplaceTopics([]string{"session:9"})
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

func TestManagerRecordsConnectionReconnectDropAndSendPressureWithoutSessionIdentity(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	manager := NewManager(WithTelemetry(recorder))
	first := NewSession(nil, SessionOptions{SendBuffer: 1})
	manager.Register("admin:7:secret-session", first)
	replacement := NewSession(nil, SessionOptions{SendBuffer: 1})
	replacement.ReplaceTopics([]string{"session:secret-session"})
	unregister := manager.Register("admin:7:secret-session", replacement)

	message := mustRealtimeEnvelope(t, "realtime.notice.v1", "private-request")
	if err := manager.Send("admin:7:secret-session", message); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if err := manager.Send("admin:7:secret-session", message); !errors.Is(err, ErrSendQueueFull) {
		t.Fatalf("expected send pressure, got %v", err)
	}
	unregister()

	events := recorder.Events()
	want := map[string]bool{"connect": false, "reconnect": false, "send_pressure": false, "drop": false}
	for _, event := range events {
		operation, _ := event.Attributes["realtime.operation"].(string)
		if _, exists := want[operation]; exists {
			want[operation] = true
		}
	}
	for operation, seen := range want {
		if !seen {
			t.Fatalf("missing realtime %s event: %+v", operation, events)
		}
	}
	text := strings.ToLower(fmt.Sprint(events))
	if strings.Contains(text, "secret-session") || strings.Contains(text, "private-request") || strings.Contains(text, "admin:7") {
		t.Fatalf("realtime identity leaked: %s", text)
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

func TestEnvelopeHandlerIsolationRecoversPanicAndContinuesReadPump(t *testing.T) {
	recorder := telemetry.NewMemoryRecorder()
	serverDone := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := NewUpgrader(func(*http.Request) bool { return true }).Upgrade(w, r)
		if err != nil {
			serverDone <- err
			return
		}
		session := NewSession(conn, SessionOptions{SendBuffer: 4, WriteWait: time.Second, PongWait: 5 * time.Second, PingInterval: time.Hour, Recorder: recorder})
		serverDone <- session.Serve(context.Background(),
			func(context.Context, Envelope) (*Envelope, error) { panic("broken handler") },
			func(context.Context, Envelope) (*Envelope, error) { return nil, errors.New("isolated handler error") },
			func(_ context.Context, envelope Envelope) (*Envelope, error) {
				pong := mustRealtimeEnvelope(t, "realtime.pong.v1", envelope.RequestID)
				return &pong, nil
			},
		)
	}))
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):], nil)
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	defer client.Close()
	for _, requestID := range []string{"rid-one", "rid-two"} {
		if err := client.WriteJSON(map[string]any{"type": "realtime.ping.v1", "request_id": requestID, "data": map[string]any{}}); err != nil {
			t.Fatalf("write ping: %v", err)
		}
		var pong Envelope
		if err := client.ReadJSON(&pong); err != nil {
			t.Fatalf("read pong after isolated handler failure: %v", err)
		}
		if pong.RequestID != requestID {
			t.Fatalf("unexpected pong: %#v", pong)
		}
	}

	events := recorder.Events()
	var panics, failures int
	for _, event := range events {
		operation, _ := event.Attributes["realtime.operation"].(string)
		outcome, _ := event.Attributes["realtime.outcome"].(string)
		if operation != "handler" {
			continue
		}
		switch outcome {
		case "panic":
			panics++
		case "error":
			failures++
		}
	}
	if panics != 2 || failures != 2 {
		t.Fatalf("handler isolation metrics panic=%d error=%d events=%+v", panics, failures, events)
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
