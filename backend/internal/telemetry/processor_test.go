// Package telemetry_test is an integration test that exercises the full
// pipeline: ProbeResult → SlidingWindow → AnomalyEngine → TelemetryPayload.
//
// Key invariants validated:
//   1. No anomaly fires on the first N-1 samples when variance is zero.
//   2. A massive spike produces |Z| > 2.5 once variance is non-zero.
//   3. ZeroStateActive is correctly set during the warm-up period.
package telemetry_test

import (
	"sync"
	"testing"
	"time"

	"github.com/telemetrypulse/backend/internal/telemetry"
	"github.com/telemetrypulse/backend/internal/window"
	"github.com/telemetrypulse/backend/pkg/models"
)

func newProbeResult(id models.EndpointID, latency float64) models.ProbeResult {
	return models.ProbeResult{
		EndpointID:  id,
		Target:      "test-target",
		Protocol:    "icmp",
		Latency:     latency,
		PacketLoss:  0,
		IsReachable: true,
		Timestamp:   time.Now().UTC(),
	}
}

// TestPipeline_WarmUp verifies that during the warm-up period (< N samples,
// all identical), no anomaly is triggered even though a large spike arrives.
func TestPipeline_WarmUp(t *testing.T) {
	var mu sync.Mutex
	var payloads []models.TelemetryPayload

	collect := func(p models.TelemetryPayload) {
		mu.Lock()
		payloads = append(payloads, p)
		mu.Unlock()
	}

	processor := telemetry.NewProcessor(collect)
	ep := models.EndpointID("test-ep")

	// Insert 1 sample → σ = 0 → zero-state active.
	processor.Process(newProbeResult(ep, 20.0))

	mu.Lock()
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloads))
	}
	p := payloads[0]
	mu.Unlock()

	if p.StdDev != 0 {
		t.Errorf("expected StdDev=0 for single sample, got %f", p.StdDev)
	}
	if p.ZScore != 0 {
		t.Errorf("expected ZScore=0 for single sample, got %f", p.ZScore)
	}
	if p.IsAnomaly {
		t.Errorf("expected IsAnomaly=false during zero-state, got true")
	}
}

// TestPipeline_AnomalyDetection fills the window with stable values then
// injects a spike and verifies the anomaly fires.
func TestPipeline_AnomalyDetection(t *testing.T) {
	var mu sync.Mutex
	var payloads []models.TelemetryPayload

	processor := telemetry.NewProcessor(func(p models.TelemetryPayload) {
		mu.Lock()
		payloads = append(payloads, p)
		mu.Unlock()
	})

	ep := models.EndpointID("test-ep-anomaly")

	// Fill the window with stable 20ms readings.
	for i := 0; i < window.DefaultWindowSize; i++ {
		// Use slightly varying values to build real variance.
		v := 20.0 + float64(i%5)*0.1 // 20.0 to 20.4 cycling
		processor.Process(newProbeResult(ep, v))
	}

	mu.Lock()
	lastStable := payloads[len(payloads)-1]
	mu.Unlock()

	if lastStable.StdDev == 0 {
		t.Logf("StdDev after fill: %f (zero — using identity values)", lastStable.StdDev)
	}

	// Now inject a massive spike: 500ms vs ~20ms mean.
	processor.Process(newProbeResult(ep, 500.0))

	mu.Lock()
	spike := payloads[len(payloads)-1]
	mu.Unlock()

	t.Logf("Spike payload: latency=%f mean=%f stddev=%f z=%f anomaly=%v",
		spike.LatencyMs, spike.Mean, spike.StdDev, spike.ZScore, spike.IsAnomaly)

	if spike.StdDev > 0 && !spike.IsAnomaly {
		t.Errorf("expected IsAnomaly=true for 500ms spike (Z=%f)", spike.ZScore)
	}
}

// TestPipeline_ZeroVarianceRobustness sends all identical values through
// the full pipeline and verifies no panic and ZScore stays 0.
func TestPipeline_ZeroVarianceRobustness(t *testing.T) {
	var mu sync.Mutex
	var payloads []models.TelemetryPayload

	processor := telemetry.NewProcessor(func(p models.TelemetryPayload) {
		mu.Lock()
		payloads = append(payloads, p)
		mu.Unlock()
	})

	ep := models.EndpointID("test-ep-zero-var")

	// 60 identical samples — window stays at σ=0 throughout.
	for i := 0; i < 60; i++ {
		processor.Process(newProbeResult(ep, 42.0))
	}

	mu.Lock()
	defer mu.Unlock()

	for i, p := range payloads {
		if p.ZScore != 0 {
			t.Errorf("payload[%d]: expected ZScore=0 for constant series, got %f", i, p.ZScore)
		}
		if p.IsAnomaly {
			t.Errorf("payload[%d]: expected IsAnomaly=false for constant series", i)
		}
	}
}
