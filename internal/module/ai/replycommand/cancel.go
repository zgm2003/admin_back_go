package replycommand

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"admin_back_go/internal/infra/redisclient"

	"github.com/redis/go-redis/v9"
)

var ErrCancelSignalsNotConfigured = errors.New("reply command cancel signals are not configured")

const cancelChannelPrefix = "ai:reply:cancel:"

type CancelPublisher interface {
	PublishCancel(context.Context, uint64) error
}

type CancelSubscription interface {
	Signal() <-chan struct{}
	Close() error
}

type CancelSubscriber interface {
	SubscribeCancel(context.Context, uint64) (CancelSubscription, error)
}

type RedisCancelPublisher struct {
	publish func(context.Context, string) error
}

func NewRedisCancelPublisher(client *redisclient.Client) *RedisCancelPublisher {
	if client == nil || client.Redis == nil {
		return &RedisCancelPublisher{}
	}
	return &RedisCancelPublisher{publish: func(ctx context.Context, channel string) error {
		return client.Redis.Publish(ctx, channel, "cancel").Err()
	}}
}

func (p *RedisCancelPublisher) PublishCancel(ctx context.Context, commandID uint64) error {
	if p == nil || p.publish == nil {
		return ErrCancelSignalsNotConfigured
	}
	channel, err := cancelChannel(commandID)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return p.publish(ctx, channel)
}

type RedisCancelSubscriber struct {
	subscribe func(context.Context, string) (CancelSubscription, error)
}

func NewRedisCancelSubscriber(client *redisclient.Client) *RedisCancelSubscriber {
	if client == nil || client.Redis == nil {
		return &RedisCancelSubscriber{}
	}
	return &RedisCancelSubscriber{subscribe: func(ctx context.Context, channel string) (CancelSubscription, error) {
		if ctx == nil {
			ctx = context.Background()
		}
		pubsub := client.Redis.Subscribe(ctx, channel)
		if _, err := pubsub.Receive(ctx); err != nil {
			_ = pubsub.Close()
			return nil, err
		}
		return newRedisCancelSubscription(ctx, pubsub), nil
	}}
}

func (s *RedisCancelSubscriber) SubscribeCancel(ctx context.Context, commandID uint64) (CancelSubscription, error) {
	if s == nil || s.subscribe == nil {
		return nil, ErrCancelSignalsNotConfigured
	}
	channel, err := cancelChannel(commandID)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.subscribe(ctx, channel)
}

func cancelChannel(commandID uint64) (string, error) {
	if commandID == 0 {
		return "", ErrCreateInputInvalid
	}
	return cancelChannelPrefix + strconv.FormatUint(commandID, 10), nil
}

type redisCancelSubscription struct {
	signal chan struct{}
	cancel context.CancelFunc
	pubsub *redis.PubSub
	done   chan struct{}
	once   sync.Once
}

func newRedisCancelSubscription(parent context.Context, pubsub *redis.PubSub) *redisCancelSubscription {
	ctx, cancel := context.WithCancel(parent)
	subscription := &redisCancelSubscription{
		signal: make(chan struct{}, 1),
		cancel: cancel,
		pubsub: pubsub,
		done:   make(chan struct{}),
	}
	go func() {
		defer close(subscription.done)
		messages := pubsub.Channel()
		select {
		case <-ctx.Done():
		case message, ok := <-messages:
			if ok && message != nil {
				subscription.signal <- struct{}{}
			}
		}
	}()
	return subscription
}

func (s *redisCancelSubscription) Signal() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.signal
}

func (s *redisCancelSubscription) Close() error {
	if s == nil {
		return nil
	}
	var closeErr error
	s.once.Do(func() {
		s.cancel()
		if s.pubsub != nil {
			closeErr = s.pubsub.Close()
		}
		<-s.done
	})
	if closeErr != nil {
		return fmt.Errorf("close reply cancel subscription: %w", closeErr)
	}
	return nil
}
