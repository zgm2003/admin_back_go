package redeemcode

import (
	"time"

	"admin_back_go/internal/telemetry"
)

const (
	metricBatches           = "payment.redeem_code.batches"
	metricCodes             = "payment.redeem_code.codes"
	metricStateTransitions  = "payment.redeem_code.state_transitions"
	metricRedemptions       = "payment.redeem_code.redemptions"
	metricRedemptionLatency = "payment.redeem_code.redemption_latency_seconds"
	metricConflicts         = "payment.redeem_code.conflicts"
)

type metrics struct{ recorder telemetry.Recorder }

func newMetrics(recorder telemetry.Recorder) metrics {
	if recorder == nil {
		recorder = telemetry.Noop()
	}
	return metrics{recorder: recorder}
}

func (metrics metrics) batch(outcome, reason string) {
	metrics.recorder.Count(metricBatches, 1, telemetry.Attributes{
		"operation": "generate", "outcome": controlledOutcome(outcome), "reason": controlledReason(reason),
	})
}

func (metrics metrics) codes(quantity int, state string) {
	if quantity <= 0 {
		return
	}
	metrics.recorder.Count(metricCodes, float64(quantity), telemetry.Attributes{
		"operation": "inventory", "state": controlledState(state),
	})
}

func (metrics metrics) transition(quantity int, state, reason string) {
	if quantity <= 0 {
		return
	}
	metrics.recorder.Count(metricStateTransitions, float64(quantity), telemetry.Attributes{
		"operation": "transition", "state": controlledState(state), "reason": controlledReason(reason),
	})
}

func (metrics metrics) redemption(outcome, reason string, elapsed time.Duration) {
	attributes := telemetry.Attributes{
		"operation": "redeem", "outcome": controlledOutcome(outcome), "reason": controlledReason(reason),
	}
	metrics.recorder.Count(metricRedemptions, 1, attributes)
	if elapsed < 0 {
		elapsed = 0
	}
	metrics.recorder.Observe(metricRedemptionLatency, elapsed.Seconds(), attributes)
}

func (metrics metrics) conflict(operation, reason string) {
	metrics.recorder.Count(metricConflicts, 1, telemetry.Attributes{
		"operation": controlledOperation(operation), "outcome": "conflict", "reason": controlledReason(reason),
	})
}

func controlledOperation(value string) string {
	switch value {
	case "generate", "void", "redeem", "lookup", "list", "export", "transition", "inventory":
		return value
	default:
		return "other"
	}
}

func controlledOutcome(value string) string {
	switch value {
	case "ok", "error", "conflict", "unavailable", "replayed", "rejected":
		return value
	default:
		return "error"
	}
}

func controlledState(value string) string {
	switch value {
	case StateUnused, StateUsed, StateVoided, StateExpired:
		return value
	default:
		return "unknown"
	}
}

func controlledReason(value string) string {
	switch value {
	case "created", "replayed", "admin_void", "expired", "unavailable", "request", "batch_collision",
		"code_collision", "source_unique", "wallet_overflow", "integrity", "dependency", "invalid", "none":
		return value
	default:
		return "other"
	}
}
