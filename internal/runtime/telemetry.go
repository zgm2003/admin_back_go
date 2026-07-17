package runtime

import (
	"context"

	"admin_back_go/internal/telemetry"
)

type processOptions struct {
	recorder telemetry.Recorder
}

type ProcessOption func(*processOptions)

func WithTelemetry(recorder telemetry.Recorder) ProcessOption {
	return func(options *processOptions) {
		if recorder != nil {
			options.recorder = recorder
		}
	}
}

func resolveProcessOptions(optionValues []ProcessOption) processOptions {
	settings := processOptions{recorder: telemetry.Noop()}
	for _, option := range optionValues {
		if option != nil {
			option(&settings)
		}
	}
	if settings.recorder == nil {
		settings.recorder = telemetry.Noop()
	}
	return settings
}

func runSchedulerReconciliation(ctx context.Context, recorder telemetry.Recorder, reconcile func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	ctx, finish := recorder.Start(ctx, "scheduler.reconciliation", telemetry.Attributes{
		"scheduler.operation": "reconcile",
	})
	err := reconcile(ctx)
	finish(err)
	return err
}
