package telemetry

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var prometheusLabelNames = []string{
	"metric",
	"http_method",
	"http_route",
	"http_status",
	"error_code",
	"operation",
	"target",
	"outcome",
	"lane",
	"modality",
	"retryable",
	"context_stage",
}

type Prometheus struct {
	events       *prometheus.CounterVec
	observations *prometheus.HistogramVec
	now          func() time.Time
}

func NewPrometheus(registerer prometheus.Registerer) (*Prometheus, error) {
	if registerer == nil {
		return nil, errors.New("prometheus registerer is required")
	}
	recorder := &Prometheus{
		events: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "admin_telemetry_events_total",
			Help: "Count of bounded backend runtime events.",
		}, prometheusLabelNames),
		observations: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "admin_telemetry_observation",
			Help:    "Bounded backend runtime observations such as seconds, retries, and token totals.",
			Buckets: prometheus.DefBuckets,
		}, prometheusLabelNames),
		now: time.Now,
	}
	if err := registerer.Register(recorder.events); err != nil {
		return nil, err
	}
	if err := registerer.Register(recorder.observations); err != nil {
		registerer.Unregister(recorder.events)
		return nil, err
	}
	return recorder, nil
}

func (recorder *Prometheus) Count(name string, delta float64, attributes Attributes) {
	if recorder == nil || recorder.events == nil || delta <= 0 {
		return
	}
	recorder.events.WithLabelValues(prometheusLabels(name, attributes)...).Add(delta)
}

func (recorder *Prometheus) Observe(name string, value float64, attributes Attributes) {
	if recorder == nil || recorder.observations == nil {
		return
	}
	recorder.observations.WithLabelValues(prometheusLabels(name, attributes)...).Observe(value)
}

func (recorder *Prometheus) Start(ctx context.Context, name string, attributes Attributes) (context.Context, func(error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	if recorder == nil {
		return ctx, func(error) {}
	}
	startedAt := recorder.clock()()
	recorder.Count(name+".started", 1, attributes)
	var once sync.Once
	return ctx, func(err error) {
		once.Do(func() {
			completed := cloneAttributes(attributes)
			completed["outcome"] = "ok"
			if err != nil {
				completed["outcome"] = "error"
			}
			recorder.Count(name+".completed", 1, completed)
			recorder.Observe(name+".duration_seconds", recorder.clock()().Sub(startedAt).Seconds(), completed)
		})
	}
}

func (recorder *Prometheus) clock() func() time.Time {
	if recorder != nil && recorder.now != nil {
		return recorder.now
	}
	return time.Now
}

func prometheusLabels(name string, attributes Attributes) []string {
	safe := SanitizeAttributes(attributes)
	return []string{
		normalizeMetricName(name),
		attributeString(safe, "http.method"),
		attributeString(safe, "http.route"),
		firstAttribute(safe, "http.status", "http.status_code"),
		attributeString(safe, "error.code"),
		firstAttribute(safe, "db.operation", "redis.operation", "scheduler.operation", "realtime.operation"),
		firstAttribute(safe, "db.table", "queue.type", "provider.name", "realtime.transport"),
		firstAttribute(safe, "outcome", "queue.outcome", "provider.status", "realtime.outcome", "scheduler.outcome"),
		attributeString(safe, "queue.lane"),
		attributeString(safe, "provider.modality"),
		attributeString(safe, "retryable"),
		attributeString(safe, "context.stage"),
	}
}

func firstAttribute(attributes Attributes, keys ...string) string {
	for _, key := range keys {
		if value := attributeString(attributes, key); value != "" {
			return value
		}
	}
	return ""
}

func attributeString(attributes Attributes, key string) string {
	if value, ok := attributes[key].(string); ok {
		return value
	}
	return ""
}

var _ Recorder = (*Prometheus)(nil)
