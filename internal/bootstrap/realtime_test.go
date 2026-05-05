package bootstrap

import (
	"errors"
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
