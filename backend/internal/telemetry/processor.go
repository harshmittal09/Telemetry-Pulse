// Package telemetry wires together the sliding window, anomaly engine,
// and probe results into a unified processing pipeline.
//
// The TelemetryProcessor:
//   - Maintains one SlidingWindow per monitored endpoint.
//   - On each ProbeResult, updates the window (O(1)) and computes μ, σ, Z.
//   - Constructs a fully enriched TelemetryPayload.
//   - Calls a user-supplied PublishFunc to dispatch the payload
//     (Phase 2 plugs in Redis Pub/Sub here).
package telemetry

import (
	"sync"
	"time"

	"github.com/telemetrypulse/backend/internal/anomaly"
	"github.com/telemetrypulse/backend/internal/window"
	"github.com/telemetrypulse/backend/pkg/models"
)

// PublishFunc is the pluggable output sink for computed payloads.
// It is called synchronously inside Process(); implementations must be
// non-blocking or use a buffered channel to avoid back-pressure.
type PublishFunc func(payload models.TelemetryPayload)

// Processor orchestrates the statistical pipeline for all endpoints.
type Processor struct {
	mu      sync.Mutex
	windows map[models.EndpointID]*window.SlidingWindow
	engine  *anomaly.Engine
	publish PublishFunc
}

// NewProcessor creates a Processor backed by a default anomaly engine.
func NewProcessor(publish PublishFunc) *Processor {
	return &Processor{
		windows: make(map[models.EndpointID]*window.SlidingWindow),
		engine:  anomaly.New(anomaly.DefaultThreshold),
		publish: publish,
	}
}

// getOrCreateWindow returns the SlidingWindow for the given endpoint,
// creating it on first access. Protected by the Processor mutex.
func (p *Processor) getOrCreateWindow(id models.EndpointID) *window.SlidingWindow {
	p.mu.Lock()
	defer p.mu.Unlock()
	w, ok := p.windows[id]
	if !ok {
		w = window.New(window.DefaultWindowSize)
		p.windows[id] = w
	}
	return w
}

// Process ingests a single ProbeResult, updates the window, computes all
// statistics, and dispatches the enriched TelemetryPayload.
//
// This function is safe to call from multiple Goroutines concurrently
// (each with a different EndpointID). Per-endpoint windows are locked
// internally; the window map itself is guarded by Processor.mu.
func (p *Processor) Process(result models.ProbeResult) {
	w := p.getOrCreateWindow(result.EndpointID)

	// Add the new latency sample; get back the computed jitter.
	jitter := w.Add(result.Latency)

	// Derive O(1) statistics from the updated window.
	mean, _, stdDev, _ := w.Stats()

	// Run the Z-score anomaly evaluation (includes zero-state guardrail).
	zResult := p.engine.Evaluate(result.Latency, mean, stdDev)

	payload := models.TelemetryPayload{
		EndpointID:       result.EndpointID,
		Target:           result.Target,
		Protocol:         result.Protocol,
		Timestamp:        time.Now().UTC(),
		LatencyMs:        result.Latency,
		Jitter:           jitter,
		PacketLoss:       result.PacketLoss,
		Mean:             mean,
		StdDev:           stdDev,
		ZScore:           zResult.ZScore,
		IsAnomaly:        zResult.IsAnomaly,
		AnomalyThreshold: p.engine.Threshold,
	}

	p.publish(payload)
}
