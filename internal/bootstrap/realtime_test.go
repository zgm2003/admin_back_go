package bootstrap

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/config"
	platformrealtime "admin_back_go/internal/platform/realtime"
)

func TestNewRealtimeStackUsesNoopPublisherWhenDisabled(t *testing.T) {
	stack := newRealtimeStack(config.RealtimeConfig{
		Enabled:           false,
		Publisher:         config.RealtimePublisherLocal,
		HeartbeatInterval: 10 * time.Second,
		SendBuffer:        8,
	})

	if stack.enabled {
		t.Fatalf("expected realtime stack to be disabled")
	}
	if _, ok := stack.publisher.(platformrealtime.NoopPublisher); !ok {
		t.Fatalf("expected noop publisher when realtime disabled, got %T", stack.publisher)
	}
	if stack.manager == nil || stack.handler == nil {
		t.Fatalf("expected disabled stack to still build explicit manager and handler")
	}
}

func TestNewRealtimeStackUsesLocalPublisherWhenEnabled(t *testing.T) {
	stack := newRealtimeStack(config.RealtimeConfig{
		Enabled:           true,
		Publisher:         config.RealtimePublisherLocal,
		HeartbeatInterval: 10 * time.Second,
		SendBuffer:        8,
	})

	if !stack.enabled {
		t.Fatalf("expected realtime stack to be enabled")
	}
	publisher, ok := stack.publisher.(*platformrealtime.LocalPublisher)
	if !ok {
		t.Fatalf("expected local publisher, got %T", stack.publisher)
	}
	if err := publisher.Publish(t.Context(), platformrealtime.Publication{}); !errors.Is(err, platformrealtime.ErrPublicationTargetRequired) {
		t.Fatalf("expected local publisher to be wired, got %v", err)
	}
}

func TestNewRealtimeStackUsesNoopPublisherWhenConfigured(t *testing.T) {
	stack := newRealtimeStack(config.RealtimeConfig{
		Enabled:           true,
		Publisher:         config.RealtimePublisherNoop,
		HeartbeatInterval: 10 * time.Second,
		SendBuffer:        8,
	})

	if !stack.enabled {
		t.Fatalf("expected realtime route to remain enabled when only publisher is noop")
	}
	if _, ok := stack.publisher.(platformrealtime.NoopPublisher); !ok {
		t.Fatalf("expected noop publisher, got %T", stack.publisher)
	}
}

func TestNewRealtimeStackUsesRedisPublisherAndSubscriberWhenConfigured(t *testing.T) {
	stack := newRealtimeStack(config.RealtimeConfig{
		Enabled:           true,
		Publisher:         config.RealtimePublisherRedis,
		RedisChannel:      "admin_go:realtime:test",
		HeartbeatInterval: 10 * time.Second,
		SendBuffer:        8,
	})

	if !stack.enabled {
		t.Fatalf("expected realtime stack to be enabled")
	}
	if _, ok := stack.publisher.(*platformrealtime.RedisPublisher); !ok {
		t.Fatalf("expected redis publisher, got %T", stack.publisher)
	}
	if stack.subscriber == nil {
		t.Fatalf("expected redis subscriber")
	}
}

func TestNewRealtimeStackAppliesCodeOwnedRealtimeDefaults(t *testing.T) {
	stack := newRealtimeStack(config.RealtimeConfig{
		Enabled:   true,
		Publisher: config.RealtimePublisherRedis,
	})

	if !stack.enabled {
		t.Fatalf("expected realtime stack to be enabled")
	}
	if got := realtimePublisherChannel(t, stack.publisher); got != config.DefaultRealtimeRedisChannel {
		t.Fatalf("expected redis publisher channel %q, got %q", config.DefaultRealtimeRedisChannel, got)
	}
	if got := realtimeSubscriberChannel(t, stack.subscriber); got != config.DefaultRealtimeRedisChannel {
		t.Fatalf("expected redis subscriber channel %q, got %q", config.DefaultRealtimeRedisChannel, got)
	}
	if got := realtimeHandlerHeartbeat(t, stack.handler); got != config.DefaultRealtimeHeartbeatInterval {
		t.Fatalf("expected handler heartbeat %s, got %s", config.DefaultRealtimeHeartbeatInterval, got)
	}
	if got := realtimeHandlerSendBuffer(t, stack.handler); got != config.DefaultRealtimeSendBuffer {
		t.Fatalf("expected handler send buffer %d, got %d", config.DefaultRealtimeSendBuffer, got)
	}
}

func realtimePublisherChannel(t *testing.T, publisher platformrealtime.Publisher) string {
	t.Helper()
	value := reflect.ValueOf(publisher)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		t.Fatalf("expected pointer publisher, got %T", publisher)
	}
	field := value.Elem().FieldByName("channel")
	if !field.IsValid() {
		t.Fatalf("publisher %T has no channel field", publisher)
	}
	return field.String()
}

func realtimeSubscriberChannel(t *testing.T, subscriber *platformrealtime.RedisSubscriber) string {
	t.Helper()
	if subscriber == nil {
		t.Fatalf("expected redis subscriber")
	}
	field := reflect.ValueOf(subscriber).Elem().FieldByName("channel")
	if !field.IsValid() {
		t.Fatalf("subscriber has no channel field")
	}
	return field.String()
}

func realtimeHandlerHeartbeat(t *testing.T, handler any) time.Duration {
	t.Helper()
	value := reflect.ValueOf(handler)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		t.Fatalf("expected pointer handler, got %T", handler)
	}
	service := value.Elem().FieldByName("service")
	if !service.IsValid() || service.IsNil() {
		t.Fatalf("handler has no service field")
	}
	heartbeat := service.Elem().FieldByName("heartbeatInterval")
	if !heartbeat.IsValid() {
		t.Fatalf("service has no heartbeatInterval field")
	}
	return time.Duration(heartbeat.Int())
}

func realtimeHandlerSendBuffer(t *testing.T, handler any) int {
	t.Helper()
	value := reflect.ValueOf(handler)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		t.Fatalf("expected pointer handler, got %T", handler)
	}
	field := value.Elem().FieldByName("sendBuffer")
	if !field.IsValid() {
		t.Fatalf("handler has no sendBuffer field")
	}
	return int(field.Int())
}

func TestNewRealtimeStackRejectsUnknownPublisherExplicitly(t *testing.T) {
	stack := newRealtimeStack(config.RealtimeConfig{
		Enabled:           true,
		Publisher:         "unknown",
		HeartbeatInterval: 10 * time.Second,
		SendBuffer:        8,
	})

	if stack.enabled {
		t.Fatalf("expected unknown realtime publisher to disable websocket upgrades explicitly")
	}
	if _, ok := stack.publisher.(platformrealtime.NoopPublisher); !ok {
		t.Fatalf("expected noop publisher for rejected config, got %T", stack.publisher)
	}
}
