package pubsub

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

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

func (r *RedisClient) PublishAnomaly(url string, zScore float64, timestamp int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	payload := map[string]any{
		"url":       url,
		"zscore":    zScore,
		"timestamp": timestamp,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return r.client.Publish(ctx, "telemetry:alerts", jsonData).Err()
}
