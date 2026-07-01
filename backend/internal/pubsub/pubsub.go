// Package pubsub implements the Redis Pub/Sub transport layer.
//
// Architecture
// ─────────────────────────────────────────────────────────────
//  telemetry.Processor
//       │  PublishFunc callback
//       ▼
//  Publisher.Publish()  ──► redis PUBLISH <channel> <json_payload>
//       │
//  Redis Server (single fan-out channel)
//       │
//  Subscriber.Subscribe() ──► per-message callback
//       │
//  wsserver.Hub  ──► WebSocket broadcast to all connected clients
//
// Design choices:
//   - A single Redis channel ("telemetrypulse:telemetry") carries all
//     endpoint payloads. Each payload contains EndpointID so subscribers
//     can filter if needed.
//   - Publisher serialises payloads to JSON inline (no intermediate buffer).
//   - Subscriber runs in a dedicated Goroutine; uses the go-redis v9 PubSub
//     API which provides a channel of *redis.Message.
//   - Both Publisher and Subscriber hold a single *redis.Client that is
//     safe for concurrent use per the go-redis documentation.
package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
	"github.com/telemetrypulse/backend/internal/config"
	"github.com/telemetrypulse/backend/pkg/models"
)

// ─────────────────────────────────────────────────────────────
// Publisher
// ─────────────────────────────────────────────────────────────

// Publisher serialises TelemetryPayload values to JSON and publishes them
// to the configured Redis Pub/Sub channel.
type Publisher struct {
	client  *redis.Client
	channel string
}

// NewPublisher creates a Publisher and verifies the Redis connection with PING.
func NewPublisher(cfg config.RedisConfig) (*Publisher, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("pubsub publisher: parse redis URL: %w", err)
	}
	client := redis.NewClient(opts)

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("pubsub publisher: redis PING failed: %w", err)
	}

	slog.Info("Redis publisher connected", "url", cfg.URL, "channel", cfg.PubSubChannel)
	return &Publisher{client: client, channel: cfg.PubSubChannel}, nil
}

// Publish serialises payload to JSON and publishes it to Redis.
//
// This is called synchronously inside telemetry.Processor.Process(), so it
// must be fast. go-redis uses a connection pool internally, so there is no
// blocking dial on each call.
//
// Returns an error only for logging — the caller (Processor) should not
// block on publish errors.
func (p *Publisher) Publish(ctx context.Context, payload models.TelemetryPayload) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pubsub: marshal payload: %w", err)
	}
	if err := p.client.Publish(ctx, p.channel, b).Err(); err != nil {
		return fmt.Errorf("pubsub: redis PUBLISH: %w", err)
	}
	return nil
}

// Close gracefully closes the underlying Redis connection pool.
func (p *Publisher) Close() error {
	return p.client.Close()
}

// ─────────────────────────────────────────────────────────────
// Subscriber
// ─────────────────────────────────────────────────────────────

// MessageHandler is the callback invoked for each received payload.
// Implementations must be non-blocking or hand off to a channel quickly.
type MessageHandler func(payload models.TelemetryPayload)

// Subscriber maintains a Redis PubSub subscription and dispatches decoded
// TelemetryPayload values to a registered MessageHandler.
type Subscriber struct {
	client  *redis.Client
	channel string
	handler MessageHandler
}

// NewSubscriber creates a Subscriber backed by a dedicated Redis connection.
// A separate client is used (rather than sharing with Publisher) because
// a connection in PubSub mode can only execute subscribe/unsubscribe commands.
func NewSubscriber(cfg config.RedisConfig, handler MessageHandler) (*Subscriber, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("pubsub subscriber: parse redis URL: %w", err)
	}
	client := redis.NewClient(opts)

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("pubsub subscriber: redis PING failed: %w", err)
	}

	slog.Info("Redis subscriber connected", "url", cfg.URL, "channel", cfg.PubSubChannel)
	return &Subscriber{
		client:  client,
		channel: cfg.PubSubChannel,
		handler: handler,
	}, nil
}

// Run starts the blocking subscription loop. It returns only when ctx is
// cancelled (graceful shutdown). Call this in a dedicated Goroutine.
//
// Message flow per cycle:
//  1. Receive *redis.Message from the PubSub channel.
//  2. Deserialise JSON → models.TelemetryPayload.
//  3. Dispatch to handler (non-blocking by contract).
func (s *Subscriber) Run(ctx context.Context) error {
	pubsub := s.client.Subscribe(ctx, s.channel)
	defer pubsub.Close()

	// Verify subscription is live before entering the loop.
	if _, err := pubsub.Receive(ctx); err != nil {
		return fmt.Errorf("pubsub: initial receive failed: %w", err)
	}

	msgCh := pubsub.Channel()

	slog.Info("Redis subscriber listening", "channel", s.channel)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Redis subscriber shutting down")
			return nil

		case msg, ok := <-msgCh:
			if !ok {
				// Channel closed by go-redis (e.g. connection lost).
				return fmt.Errorf("pubsub: message channel closed unexpectedly")
			}

			var payload models.TelemetryPayload
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				slog.Warn("pubsub: failed to deserialise payload",
					"error", err,
					"raw", msg.Payload)
				continue
			}

			s.handler(payload)
		}
	}
}

// Close gracefully closes the subscriber's Redis connection.
func (s *Subscriber) Close() error {
	return s.client.Close()
}
