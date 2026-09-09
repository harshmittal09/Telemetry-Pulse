package engine

import (
	"fmt"
	"telemetrypulse/internal/anomaly"
	"telemetrypulse/internal/probe"
	"telemetrypulse/internal/pubsub"
)

type Pipeline struct {
	detector    *anomaly.Detector
	redisClient *pubsub.RedisClient
}

func NewPipeline(detector *anomaly.Detector, redisClient *pubsub.RedisClient) *Pipeline {
	return &Pipeline{
		detector:    detector,
		redisClient: redisClient,
	}
}

func (p *Pipeline) Start(channel <-chan probe.ProbeResult) {
	for result := range channel {
		url := result.Url
		latency := result.Latency
		statusCode := result.StatusCode
		timestrap := result.Timestrap

		if statusCode == 404 {
			continue
		}

		zScore, isAnomaly := p.detector.Analyze(url, latency)

		if isAnomaly {
			err := p.redisClient.PublishAnomaly(url, zScore, timestrap)
			if err != nil {
				fmt.Printf("Failed to publish to Redis: %v\n", err)
			}
		}

		fmt.Printf("[URL: %s] Latency: %.2fms | Z-Score: %.2f | Anomaly: %v\n | TimeStamp : %v\n", url, latency, zScore, isAnomaly, timestrap)

	}
}
