package replycommand

import (
	"context"
	"errors"
	"testing"
)

type fakeCancelSubscription struct {
	signal chan struct{}
}

func (f *fakeCancelSubscription) Signal() <-chan struct{} { return f.signal }
func (f *fakeCancelSubscription) Close() error            { return nil }

func TestRedisCancelSignalUsesCommandScopedChannel(t *testing.T) {
	var publishedChannel string
	publisher := &RedisCancelPublisher{publish: func(_ context.Context, channel string) error {
		publishedChannel = channel
		return errors.New("redis unavailable")
	}}
	if err := publisher.PublishCancel(context.Background(), 41); err == nil || publishedChannel != "ai:reply:cancel:41" {
		t.Fatalf("channel=%q err=%v", publishedChannel, err)
	}

	want := &fakeCancelSubscription{signal: make(chan struct{})}
	var subscribedChannel string
	subscriber := &RedisCancelSubscriber{subscribe: func(_ context.Context, channel string) (CancelSubscription, error) {
		subscribedChannel = channel
		return want, nil
	}}
	got, err := subscriber.SubscribeCancel(context.Background(), 41)
	if err != nil || got != want || subscribedChannel != "ai:reply:cancel:41" {
		t.Fatalf("subscription=%T channel=%q err=%v", got, subscribedChannel, err)
	}
}
