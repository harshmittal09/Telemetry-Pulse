package pubsub

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// Channel carries every reading, not just anomalies
const Channel = "telemetry:readings"

type RedisClient struct {
	client *redis.Client
}

// NewRedisClient takes a url like redis://user:password@host:6379/0 (rediss:// for TLS)
func NewRedisClient(redisURL string) (*RedisClient, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	return &RedisClient{
		client: redis.NewClient(opts),
	}, nil
}

// WaitReady pings redis until it answers, so we fail at startup instead of
// silently dropping every reading later
func (r *RedisClient) WaitReady(ctx context.Context, attempts int) error {
	var err error
	for i := 0; i < attempts; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = r.client.Ping(pingCtx).Err()
		cancel()
		if err == nil {
			return nil
		}

		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (r *RedisClient) Publish(ctx context.Context, v any) error {
	jsonData, err := json.Marshal(v)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return r.client.Publish(ctx, Channel, jsonData).Err()
}

func (r *RedisClient) Subscribe(ctx context.Context) *redis.PubSub {
	return r.client.Subscribe(ctx, Channel)
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}
