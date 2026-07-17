package telemetry

import (
	"context"
	"strings"
	"sync"
	"time"
)

type Attributes map[string]any

type Recorder interface {
	Count(name string, delta float64, attributes Attributes)
	Observe(name string, value float64, attributes Attributes)
	Start(ctx context.Context, name string, attributes Attributes) (context.Context, func(error))
}

type noopRecorder struct{}

func Noop() Recorder {
	return noopRecorder{}
}

func (noopRecorder) Count(string, float64, Attributes)   {}
func (noopRecorder) Observe(string, float64, Attributes) {}
func (noopRecorder) Start(ctx context.Context, _ string, _ Attributes) (context.Context, func(error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	return ctx, func(error) {}
}

type EventKind string

const (
	EventCount   EventKind = "count"
	EventObserve EventKind = "observe"
	EventStart   EventKind = "start"
	EventEnd     EventKind = "end"
)

type Event struct {
	Kind       EventKind
	Name       string
	Value      float64
	Attributes Attributes
}

type MemoryRecorder struct {
	mu     sync.Mutex
	events []Event
	now    func() time.Time
}

func NewMemoryRecorder() *MemoryRecorder {
	return &MemoryRecorder{now: time.Now}
}

func (recorder *MemoryRecorder) Count(name string, delta float64, attributes Attributes) {
	recorder.append(Event{Kind: EventCount, Name: normalizeMetricName(name), Value: delta, Attributes: SanitizeAttributes(attributes)})
}

func (recorder *MemoryRecorder) Observe(name string, value float64, attributes Attributes) {
	recorder.append(Event{Kind: EventObserve, Name: normalizeMetricName(name), Value: value, Attributes: SanitizeAttributes(attributes)})
}

func (recorder *MemoryRecorder) Start(ctx context.Context, name string, attributes Attributes) (context.Context, func(error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	name = normalizeMetricName(name)
	attributes = SanitizeAttributes(attributes)
	startedAt := recorder.clock()()
	recorder.append(Event{Kind: EventStart, Name: name, Attributes: cloneAttributes(attributes)})
	var once sync.Once
	return ctx, func(err error) {
		once.Do(func() {
			ended := cloneAttributes(attributes)
			ended["outcome"] = "ok"
			if err != nil {
				ended["outcome"] = "error"
			}
			recorder.append(Event{
				Kind:       EventEnd,
				Name:       name,
				Value:      recorder.clock()().Sub(startedAt).Seconds(),
				Attributes: SanitizeAttributes(ended),
			})
		})
	}
}

func (recorder *MemoryRecorder) Events() []Event {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	events := make([]Event, len(recorder.events))
	for index, event := range recorder.events {
		event.Attributes = cloneAttributes(event.Attributes)
		events[index] = event
	}
	return events
}

func (recorder *MemoryRecorder) append(event Event) {
	if recorder == nil {
		return
	}
	recorder.mu.Lock()
	recorder.events = append(recorder.events, event)
	recorder.mu.Unlock()
}

func (recorder *MemoryRecorder) clock() func() time.Time {
	if recorder != nil && recorder.now != nil {
		return recorder.now
	}
	return time.Now
}

func cloneAttributes(attributes Attributes) Attributes {
	cloned := make(Attributes, len(attributes))
	for key, value := range attributes {
		cloned[key] = value
	}
	return cloned
}

func normalizeMetricName(name string) string {
	const maxMetricNameLength = 96
	name = strings.TrimSpace(name)
	if len(name) > maxMetricNameLength {
		name = name[:maxMetricNameLength]
	}
	return name
}
