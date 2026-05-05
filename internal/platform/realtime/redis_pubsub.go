package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

var ErrRealtimeRedisNotReady = errors.New("realtime redis client is not ready")

type redisClient interface {
	Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
}

// RedisPublisher publishes realtime publications to a Redis Pub/Sub channel.
type RedisPublisher struct {
	client  redisClient
	channel string
}

// NewRedisPublisher creates a Redis-backed realtime publisher.
func NewRedisPublisher(client redisClient, channel string) *RedisPublisher {
	return &RedisPublisher{client: client, channel: strings.TrimSpace(channel)}
}

// Publish serializes one publication and sends it to Redis Pub/Sub.
func (p *RedisPublisher) Publish(ctx context.Context, publication Publication) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validatePublicationTarget(publication); err != nil {
		return err
	}
	if p == nil || p.client == nil || strings.TrimSpace(p.channel) == "" {
		return ErrRealtimeRedisNotReady
	}
	payload, err := encodeRedisPublication(publication)
	if err != nil {
		return err
	}
	return p.client.Publish(ctx, p.channel, payload).Err()
}

// RedisSubscriber consumes Redis Pub/Sub realtime publications and forwards
// them into the local publisher owned by this process.
type RedisSubscriber struct {
	client    redisClient
	channel   string
	publisher Publisher
	logger    *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewRedisSubscriber creates a Redis subscriber that forwards to local delivery.
func NewRedisSubscriber(client redisClient, channel string, publisher Publisher, loggers ...*slog.Logger) *RedisSubscriber {
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &RedisSubscriber{client: client, channel: strings.TrimSpace(channel), publisher: publisher, logger: logger}
}

// Start begins the Redis subscription loop. It returns after the goroutine is
// started; use Shutdown to stop it.
func (s *RedisSubscriber) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.client == nil || strings.TrimSpace(s.channel) == "" || s.publisher == nil {
		return ErrRealtimeRedisNotReady
	}

	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	done := s.done
	s.mu.Unlock()

	pubsub := s.client.Subscribe(runCtx, s.channel)
	go func() {
		defer close(done)
		defer pubsub.Close()
		ch := pubsub.Channel()
		for {
			select {
			case <-runCtx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if err := s.handlePayload(runCtx, []byte(msg.Payload)); err != nil && !errors.Is(err, ErrSessionNotFound) && s.logger != nil {
					s.logger.WarnContext(runCtx, "failed to handle realtime redis publication", "channel", s.channel, "error", err)
				}
			}
		}
	}()
	return nil
}

// Shutdown stops the subscription loop.
func (s *RedisSubscriber) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()
	if cancel == nil || done == nil {
		return nil
	}
	cancel()
	if ctx == nil {
		<-done
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *RedisSubscriber) handlePayload(ctx context.Context, payload []byte) error {
	publication, err := decodeRedisPublication(payload)
	if err != nil {
		return err
	}
	if s == nil || s.publisher == nil {
		return ErrRealtimeRedisNotReady
	}
	return s.publisher.Publish(ctx, publication)
}

func encodeRedisPublication(publication Publication) ([]byte, error) {
	if err := validatePublicationTarget(publication); err != nil {
		return nil, err
	}
	return json.Marshal(publication)
}

func decodeRedisPublication(payload []byte) (Publication, error) {
	var publication Publication
	if err := json.Unmarshal(payload, &publication); err != nil {
		return Publication{}, err
	}
	if len(publication.Envelope.Data) == 0 {
		publication.Envelope.Data = json.RawMessage(`{}`)
	}
	if err := validatePublicationTarget(publication); err != nil {
		return Publication{}, err
	}
	return publication, nil
}

func validatePublicationTarget(publication Publication) error {
	if strings.TrimSpace(publication.SessionKey) != "" {
		return nil
	}
	if strings.TrimSpace(publication.Platform) != "" && publication.UserID > 0 {
		return nil
	}
	return ErrPublicationTargetRequired
}
