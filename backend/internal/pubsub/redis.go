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

func NewRedisClient() *RedisClient {

	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	return &RedisClient{
		client: rdb,
	}
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
