package admin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/middleware"
	realtimemodule "admin_back_go/internal/module/realtime"
	projecti18n "admin_back_go/internal/shared/i18n"
	"admin_back_go/internal/telemetry"

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
		realtimemodule.NewService(25_000_000_000),
		infrarealtime.NewUpgrader(func(*http.Request) bool { return true }),
		infrarealtime.NewManager(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	))

	server := httptest.NewServer(router)
	defer server.Close()

	client := dialRealtime(t, server.URL)
	defer client.Close()

	var connected infrarealtime.Envelope
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected.Type != realtimemodule.TypeConnectedV1 {
		t.Fatalf("expected connected event, got %#v", connected)
	}

	if err := client.WriteJSON(map[string]any{
		"type":       realtimemodule.TypePingV1,
		"request_id": "rid-1",
		"data":       map[string]any{},
	}); err != nil {
		t.Fatalf("write ping: %v", err)
	}

	var pong infrarealtime.Envelope
	if err := client.ReadJSON(&pong); err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if pong.Type != realtimemodule.TypePongV1 || pong.RequestID != "rid-1" {
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
		realtimemodule.NewService(25*time.Second),
		nil,
		infrarealtime.NewManager(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	))

	server := httptest.NewServer(router)
	defer server.Close()

	client := dialRealtime(t, server.URL)
	defer client.Close()

	var connected infrarealtime.Envelope
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if connected.Type != realtimemodule.TypeConnectedV1 {
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
		realtimemodule.NewService(25*time.Second),
		infrarealtime.NewUpgrader(func(*http.Request) bool { return true }),
		infrarealtime.NewManager(),
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
		realtimemodule.NewService(25*time.Second),
		infrarealtime.NewUpgrader(func(*http.Request) bool { return true }),
		infrarealtime.NewManager(),
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
		realtimemodule.NewService(25*time.Second),
		infrarealtime.NewUpgrader(func(*http.Request) bool { return true }),
		infrarealtime.NewManager(),
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
	manager := infrarealtime.NewManager()
	service := realtimemodule.NewService(25 * time.Second)
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
		infrarealtime.NewUpgrader(func(*http.Request) bool { return true }),
		manager,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	))

	server := httptest.NewServer(router)
	defer server.Close()

	client := dialRealtime(t, server.URL)
	var connected infrarealtime.Envelope
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
		realtimemodule.NewService(25*time.Second),
		infrarealtime.NewUpgrader(func(*http.Request) bool { return true }),
		infrarealtime.NewManager(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	))

	server := httptest.NewServer(router)
	defer server.Close()

	client := dialRealtime(t, server.URL)
	defer client.Close()
	var connected infrarealtime.Envelope
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}

	if err := client.WriteJSON(map[string]any{
		"type":       realtimemodule.TypeSubscribeV1,
		"request_id": "rid-subscribe",
		"data": map[string]any{
			"topics": []string{"user:8"},
		},
	}); err != nil {
		t.Fatalf("write subscribe: %v", err)
	}

	var reply infrarealtime.Envelope
	if err := client.ReadJSON(&reply); err != nil {
		t.Fatalf("read subscribe error: %v", err)
	}
	if reply.Type != realtimemodule.TypeErrorV1 || reply.RequestID != "rid-subscribe" {
		t.Fatalf("unexpected subscribe error reply: %#v", reply)
	}
}

func TestWebSocketRecordsIsolatedClientHandlerErrors(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	recorder := telemetry.NewMemoryRecorder()
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
		realtimemodule.NewService(25*time.Second),
		infrarealtime.NewUpgrader(func(*http.Request) bool { return true }),
		infrarealtime.NewManager(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		WithTelemetry(recorder),
	))

	server := httptest.NewServer(router)
	defer server.Close()

	client := dialRealtime(t, server.URL)
	defer client.Close()
	var connected infrarealtime.Envelope
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if err := client.WriteJSON(map[string]any{
		"type":       realtimemodule.TypeResumeV1,
		"request_id": "rid-resume-without-reader",
		"data": map[string]any{
			"after_sequence": 0,
		},
	}); err != nil {
		t.Fatalf("write resume without event reader: %v", err)
	}
	if err := client.WriteJSON(map[string]any{
		"type":       realtimemodule.TypePingV1,
		"request_id": "rid-after-error",
		"data":       map[string]any{},
	}); err != nil {
		t.Fatalf("write ping after error: %v", err)
	}
	var pong infrarealtime.Envelope
	if err := client.ReadJSON(&pong); err != nil {
		t.Fatalf("read pong after isolated handler error: %v", err)
	}
	if pong.Type != realtimemodule.TypePongV1 || pong.RequestID != "rid-after-error" {
		t.Fatalf("unexpected pong after isolated handler error: %#v", pong)
	}

	for _, event := range recorder.Events() {
		if event.Name == "realtime.handlers" && event.Attributes["realtime.operation"] == "handler" && event.Attributes["realtime.outcome"] == "error" {
			return
		}
	}
	t.Fatalf("production websocket session did not record isolated handler error: %+v", recorder.Events())
}

type staticEventReader struct {
	result *realtimemodule.ResumeResult
}

func (reader staticEventReader) ResumeUser(context.Context, realtimemodule.ResumeQuery) (*realtimemodule.ResumeResult, error) {
	return reader.result, nil
}

func TestWebSocketReplaysMoreEventsThanTheLiveSendBuffer(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	now := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
	events := make([]realtimemodule.Event, 32)
	for index := range events {
		eventID, err := infrarealtime.NewEventID(now.Add(time.Duration(index) * time.Millisecond))
		if err != nil {
			t.Fatalf("generate event ID: %v", err)
		}
		requestID := "rid-replay"
		events[index] = realtimemodule.Event{
			Sequence:    uint64(index + 1),
			EventID:     eventID,
			EventType:   realtimemodule.TypeNotificationCreatedV1,
			RequestID:   &requestID,
			TargetType:  realtimemodule.TargetTypeUser,
			TargetID:    "7",
			Durability:  string(infrarealtime.Durable),
			PayloadJSON: `{"task_id":1,"title":"title","content":"body","link":"/","level":"normal","notification_type":"info"}`,
			OccurredAt:  now.Add(time.Duration(index) * time.Millisecond),
		}
	}
	service := realtimemodule.NewService(25*time.Second, realtimemodule.WithEventReader(staticEventReader{result: &realtimemodule.ResumeResult{
		Events: events, LatestSequence: uint64(len(events)), OldestAvailableSequence: 1,
	}}))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, SessionID: 9, Platform: "admin"})
		c.Next()
	})
	RegisterRoutes(router, NewHandler(
		service,
		infrarealtime.NewUpgrader(func(*http.Request) bool { return true }),
		infrarealtime.NewManager(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		WithSendBuffer(1),
	))
	server := httptest.NewServer(router)
	defer server.Close()
	client := dialRealtime(t, server.URL)
	defer client.Close()
	var connected infrarealtime.Envelope
	if err := client.ReadJSON(&connected); err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if err := client.WriteJSON(map[string]any{
		"type": realtimemodule.TypeResumeV1, "request_id": "rid-resume", "data": map[string]any{"after_sequence": 0},
	}); err != nil {
		t.Fatalf("write resume: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	for index := range events {
		var replay infrarealtime.Envelope
		if err := client.ReadJSON(&replay); err != nil {
			t.Fatalf("read replay %d/%d: %v", index+1, len(events), err)
		}
		if replay.Sequence != uint64(index+1) {
			t.Fatalf("replay sequence=%d want=%d", replay.Sequence, index+1)
		}
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
