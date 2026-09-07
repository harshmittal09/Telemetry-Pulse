package main

import (
	"fmt"
	"telemetrypulse/internal/anomaly"
	"telemetrypulse/internal/probe"
)

func main() {

	urls := []string{"https://google.com", "https://github.com", "https://silvershinehouse.com"}
	channel := make(chan probe.ProbeResult)

	detector := anomaly.NewDetector()

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

		fmt.Printf("[URL: %s] Latency: %.2fms | Z-Score: %.2f | Anomaly: %v\n | TimeStamp : %v\n", url, latency, zScore, isAnomaly, timestrap)

	}

}
