package main

import (
	"fmt"
	"telemetrypulse/internal/anomaly"
	"telemetrypulse/internal/probe"
	"telemetrypulse/internal/pubsub"
)

func main() {

	urls := []string{"https://google.com", "https://github.com", "https://silvershinehouse.com"}
	channel := make(chan probe.ProbeResult)

	detector := anomaly.NewDetector()

	redisClient := pubsub.NewRedisClient()

	for _, url := range urls {
		probe.StartWorker(url, channel)
	}

	for result := range channel {
		url := result.Url
		latency := result.Latency
		statusCode := result.StatusCode
		timestrap := result.Timestrap

		if statusCode == 404 {
			continue
		}

		zScore, isAnomaly := detector.Analyze(url, latency)

		if isAnomaly {
			err := redisClient.PublishAnomaly(url, zScore, timestrap)
			if err != nil {
				fmt.Printf("Failed to publish to Redis: %v\n", err)
			}
		}

		fmt.Printf("[URL: %s] Latency: %.2fms | Z-Score: %.2f | Anomaly: %v\n | TimeStamp : %v\n", url, latency, zScore, isAnomaly, timestrap)

	}

}
