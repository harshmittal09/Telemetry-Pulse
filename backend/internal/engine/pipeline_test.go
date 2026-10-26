package engine

import (
	"testing"

	"telemetrypulse/internal/anomaly"
	"telemetrypulse/internal/probe"
)

func TestDownReadingsStayOutOfTheBaseline(t *testing.T) {
	p := NewPipeline(anomaly.NewDetector(anomaly.DefaultConfig()), nil, nil)

	for i := 0; i < 50; i++ {
		p.process(probe.ProbeResult{URL: "u", Latency: 40, OK: true, StatusCode: 200})
	}
	down := p.process(probe.ProbeResult{URL: "u", Latency: 0.3, Error: "connection refused"})
	if down.OK || down.Anomaly || down.Error != "connection refused" {
		t.Fatalf("down reading = %+v", down)
	}

	// baseline should still be 40, not pulled down by the 0.3ms failure
	next := p.process(probe.ProbeResult{URL: "u", Latency: 40, OK: true, StatusCode: 200})
	if next.BaselineMs != 40 {
		t.Fatalf("baseline = %v, want 40", next.BaselineMs)
	}
}
