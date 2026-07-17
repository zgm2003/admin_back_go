package redisclient

import (
	"context"
	"net"
	"strings"
	"time"

	"admin_back_go/internal/telemetry"

	"github.com/redis/go-redis/v9"
)

type telemetryHook struct {
	recorder telemetry.Recorder
}

func newTelemetryHook(recorder telemetry.Recorder) *telemetryHook {
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	return &telemetryHook{recorder: recorder}
}

func (hook *telemetryHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network string, address string) (net.Conn, error) {
		return next(ctx, network, address)
	}
}

func (hook *telemetryHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, command redis.Cmder) error {
		startedAt := time.Now()
		err := next(ctx, command)
		hook.record(commandName(command), time.Since(startedAt), err)
		return err
	}
}

func (hook *telemetryHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, commands []redis.Cmder) error {
		startedAt := time.Now()
		err := next(ctx, commands)
		duration := time.Since(startedAt)
		for _, command := range commands {
			commandErr := err
			if command != nil && command.Err() != nil {
				commandErr = command.Err()
			}
			hook.record(commandName(command), duration, commandErr)
		}
		return err
	}
}

func (hook *telemetryHook) record(operation string, duration time.Duration, err error) {
	outcome := "ok"
	if err != nil && err != redis.Nil {
		outcome = "error"
	}
	attributes := telemetry.Attributes{
		"redis.operation": operation,
		"outcome":         outcome,
	}
	hook.recorder.Count("redis.commands", 1, attributes)
	hook.recorder.Observe("redis.duration_seconds", duration.Seconds(), attributes)
}

func commandName(command redis.Cmder) string {
	if command == nil {
		return "unknown"
	}
	name := strings.ToLower(strings.TrimSpace(command.Name()))
	if name == "" {
		return "unknown"
	}
	return name
}

var _ redis.Hook = (*telemetryHook)(nil)
