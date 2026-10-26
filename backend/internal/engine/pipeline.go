package engine

import (
	"context"
	"log"
	"telemetrypulse/internal/anomaly"
	"telemetrypulse/internal/probe"
	"telemetrypulse/internal/pubsub"
)

type Pipeline struct {
	detector    *anomaly.Detector
	redisClient *pubsub.RedisClient
	history     *History
}

func NewPipeline(detector *anomaly.Detector, redisClient *pubsub.RedisClient, history *History) *Pipeline {
	return &Pipeline{
		detector:    detector,
		redisClient: redisClient,
		history:     history,
	}
}

// Start blocks until ctx is cancelled, every probe result becomes a Reading
// that is stored in history and published for the websocket clients.
func (p *Pipeline) Start(ctx context.Context, channel <-chan probe.ProbeResult) {
	for {
		select {
		case <-ctx.Done():
			return
		case result := <-channel:
			reading := p.process(result)
			p.history.Add(reading)

			if err := p.redisClient.Publish(ctx, reading); err != nil {
				log.Printf("Failed to publish to Redis: %v", err)
			}

			if reading.OK {
				log.Printf("[%s] %d %.2fms | baseline %.2fms | z %.2f | anomaly %v",
					reading.URL, reading.Status, reading.LatencyMs, reading.BaselineMs, reading.ZScore, reading.Anomaly)
			} else {
				log.Printf("[%s] DOWN (%s) after %.2fms", reading.URL, reading.Error, reading.LatencyMs)
			}
		}
	}
}

func (p *Pipeline) process(result probe.ProbeResult) Reading {
	reading := Reading{
		URL:       result.URL,
		LatencyMs: round2(result.Latency),
		Status:    result.StatusCode,
		OK:        result.OK,
		Error:     result.Error,
		Timestamp: result.Timestamp,
	}

	// failed requests come back almost instantly, so they stay out of the
	// baseline, otherwise a dead site would look like the fastest one
	if result.OK {
		res := p.detector.Analyze(result.URL, result.Latency)
		reading.ZScore = round2(res.ZScore)
		reading.BaselineMs = round2(res.Baseline)
		reading.Anomaly = res.Anomaly
	}

	return reading
}
