package realtime

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	projecti18n "admin_back_go/internal/i18n"
	"admin_back_go/internal/middleware"
	platformrealtime "admin_back_go/internal/platform/realtime"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestWebSocketConnectsAndRepliesToPing(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{
			UserID:    7,
			SessionID: 9,
			Platform:  "admin",
		})
		c.Next()
	})
	RegisterRoutes(router, NewHandler(
		NewService(25_000_000_000),
		platformrealtime.NewUpgrader(func(*http.Request) bool { return true }),
		platformrealtime.NewManager(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	))

	server := httptest.NewServer(router)
	defer server.Close()

	client := dialRealtime(t, server.URL)
	defer client.Close()

	var connected platformrealtime.Envelope
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected.Type != TypeConnectedV1 {
		t.Fatalf("expected connected event, got %#v", connected)
	}

	if err := client.WriteJSON(map[string]any{
		"type":       TypePingV1,
		"request_id": "rid-1",
		"data":       map[string]any{},
	}); err != nil {
		t.Fatalf("write ping: %v", err)
	}

	var pong platformrealtime.Envelope
	if err := client.ReadJSON(&pong); err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if pong.Type != TypePongV1 || pong.RequestID != "rid-1" {
		t.Fatalf("unexpected pong: %#v", pong)
	}
	var data map[string]any
	if err := json.Unmarshal(pong.Data, &data); err != nil {
		t.Fatalf("invalid pong data: %v", err)
	}
	if data["server_time"] == "" {
		t.Fatalf("expected server_time in pong data, got %#v", data)
	}
}

func TestWebSocketUsesDefaultUpgraderWhenNil(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{
			UserID:    7,
			SessionID: 9,
			Platform:  "admin",
		})
		c.Next()
	})
	RegisterRoutes(router, NewHandler(
		NewService(25*time.Second),
		nil,
		platformrealtime.NewManager(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	))

	server := httptest.NewServer(router)
	defer server.Close()

	client := dialRealtime(t, server.URL)
	defer client.Close()

	var connected platformrealtime.Envelope
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected.Type != TypeConnectedV1 {
		t.Fatalf("expected connected event, got %#v", connected)
	}
}

func TestWebSocketRejectsWhenRealtimeDisabled(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{
			UserID:    7,
			SessionID: 9,
			Platform:  "admin",
		})
		c.Next()
	})
	RegisterRoutes(router, NewHandler(
		NewService(25*time.Second),
		platformrealtime.NewUpgrader(func(*http.Request) bool { return true }),
		platformrealtime.NewManager(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		WithEnabled(false),
	))

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):] + "/api/admin/v1/realtime/ws"
	_, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected disabled realtime websocket dial to fail")
	}
	if response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 response, got response=%#v err=%v", response, err)
	}
}

func TestWebSocketLocalizesDisabledResponse(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{
			UserID:    7,
			SessionID: 9,
			Platform:  "admin",
		})
		c.Next()
	})
	RegisterRoutes(router, NewHandler(
		NewService(25*time.Second),
		platformrealtime.NewUpgrader(func(*http.Request) bool { return true }),
		platformrealtime.NewManager(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		WithEnabled(false),
	))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/realtime/ws", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if body["msg"] != "Realtime is not enabled" {
		t.Fatalf("expected localized realtime disabled message, got %#v", body["msg"])
	}
}

func TestWebSocketLocalizesMissingIdentity(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	RegisterRoutes(router, NewHandler(
		NewService(25*time.Second),
		platformrealtime.NewUpgrader(func(*http.Request) bool { return true }),
		platformrealtime.NewManager(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/realtime/ws", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if body["msg"] != "Token is invalid or expired" {
		t.Fatalf("expected localized token message, got %#v", body["msg"])
	}
}

func TestWebSocketRegistersAndCleansUpSession(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	manager := platformrealtime.NewManager()
	service := NewService(25 * time.Second)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{
			UserID:    7,
			SessionID: 9,
			Platform:  "admin",
		})
		c.Next()
	})
	RegisterRoutes(router, NewHandler(
		service,
		platformrealtime.NewUpgrader(func(*http.Request) bool { return true }),
		manager,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	))

	server := httptest.NewServer(router)
	defer server.Close()

	client := dialRealtime(t, server.URL)
	var connected platformrealtime.Envelope
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}

	if got := manager.Count(); got != 1 {
		t.Fatalf("expected one registered realtime session, got %d", got)
	}

	if err := client.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye")); err != nil {
		t.Fatalf("write close: %v", err)
	}
	_ = client.Close()

	for i := 0; i < 20; i++ {
		if manager.Count() == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected realtime session cleanup, count=%d", manager.Count())
}

func TestWebSocketRejectsUnauthorizedSubscribeTopic(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{
			UserID:    7,
			SessionID: 9,
			Platform:  "admin",
		})
		c.Next()
	})
	RegisterRoutes(router, NewHandler(
		NewService(25*time.Second),
		platformrealtime.NewUpgrader(func(*http.Request) bool { return true }),
		platformrealtime.NewManager(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	))

	server := httptest.NewServer(router)
	defer server.Close()

	client := dialRealtime(t, server.URL)
	defer client.Close()
	var connected platformrealtime.Envelope
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}

	if err := client.WriteJSON(map[string]any{
		"type":       TypeSubscribeV1,
		"request_id": "rid-subscribe",
		"data": map[string]any{
			"topics": []string{"user:8"},
		},
	}); err != nil {
		t.Fatalf("write subscribe: %v", err)
	}

	var reply platformrealtime.Envelope
	if err := client.ReadJSON(&reply); err != nil {
		t.Fatalf("read subscribe error: %v", err)
	}
	if reply.Type != TypeErrorV1 || reply.RequestID != "rid-subscribe" {
		t.Fatalf("unexpected subscribe error reply: %#v", reply)
	}
}

func dialRealtime(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + serverURL[len("http"):] + "/api/admin/v1/realtime/ws"
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial realtime: %v", err)
	}
	return client
}
