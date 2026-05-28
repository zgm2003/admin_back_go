package realtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

func TestUpgraderExchangesProjectEnvelopes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := NewUpgrader(func(*http.Request) bool { return true }).Upgrade(w, r)
		if err != nil {
			t.Fatalf("Upgrade returned error: %v", err)
		}
		defer conn.Close()

		envelope, err := conn.ReadEnvelope()
		if err != nil {
			t.Fatalf("ReadEnvelope returned error: %v", err)
		}
		if envelope.Type != "realtime.ping.v1" || envelope.RequestID != "rid-1" {
			t.Fatalf("unexpected client envelope: %#v", envelope)
		}

		reply, err := NewEnvelope("realtime.pong.v1", envelope.RequestID, map[string]string{"server_time": "now"})
		if err != nil {
			t.Fatalf("NewEnvelope returned error: %v", err)
		}
		if err := conn.WriteEnvelope(reply); err != nil {
			t.Fatalf("WriteEnvelope returned error: %v", err)
		}
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):]
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	defer client.Close()

	if err := client.WriteJSON(map[string]any{
		"type":       "realtime.ping.v1",
		"request_id": "rid-1",
		"data":       map[string]any{},
	}); err != nil {
		t.Fatalf("client WriteJSON returned error: %v", err)
	}

	var reply Envelope
	if err := client.ReadJSON(&reply); err != nil {
		t.Fatalf("client ReadJSON returned error: %v", err)
	}
	if reply.Type != "realtime.pong.v1" || reply.RequestID != "rid-1" {
		t.Fatalf("unexpected reply: %#v", reply)
	}
	var data map[string]string
	if err := json.Unmarshal(reply.Data, &data); err != nil {
		t.Fatalf("invalid reply data: %v", err)
	}
	if data["server_time"] != "now" {
		t.Fatalf("unexpected reply data: %#v", data)
	}
}

func TestAllowedOriginCheckerAllowsConfiguredCrossPortBrowserOrigin(t *testing.T) {
	check := NewAllowedOriginChecker([]string{"http://127.0.0.1:5173"})
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/admin/v1/realtime/ws", nil)
	request.Header.Set("Origin", "http://127.0.0.1:5173")

	if !check(request) {
		t.Fatalf("expected configured dev origin to be allowed")
	}
}

func TestAllowedOriginCheckerAllowsMissingOriginAndSameHost(t *testing.T) {
	check := NewAllowedOriginChecker(nil)

	withoutOrigin := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/admin/v1/realtime/ws", nil)
	if !check(withoutOrigin) {
		t.Fatalf("expected non-browser request without Origin to be allowed")
	}

	sameHost := httptest.NewRequest(http.MethodGet, "http://admin.example.test/api/admin/v1/realtime/ws", nil)
	sameHost.Header.Set("Origin", "http://admin.example.test")
	if !check(sameHost) {
		t.Fatalf("expected same-host browser upgrade to be allowed")
	}
}

func TestAllowedOriginCheckerRejectsUnconfiguredOrigin(t *testing.T) {
	check := NewAllowedOriginChecker([]string{"http://127.0.0.1:5173"})
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/admin/v1/realtime/ws", nil)
	request.Header.Set("Origin", "http://evil.example.test")

	if check(request) {
		t.Fatalf("expected unconfigured origin to be rejected")
	}
}
