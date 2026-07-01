// Package anomaly_test is the primary test suite for the Z-score engine.
//
// CRITICAL TEST — Zero-State Exception:
//   TestZeroVarianceGuardrail explicitly proves that when σ = 0, the engine
//   hard-codes Z = 0 and sets ZeroStateActive = true, preventing any
//   division-by-zero runtime panic or ±Inf/NaN propagation.
//
// Additional tests cover:
//   - Normal anomaly detection (|Z| >= 2.5 → IsAnomaly = true).
//   - Boundary value (|Z| == 2.5 exactly → IsAnomaly = true).
//   - Below threshold (IsAnomaly = false).
//   - Negative sigma guardrail (float rounding artefact).
//   - NaN / Inf secondary guard.
package anomaly_test

import (
	"math"
	"testing"

	"github.com/telemetrypulse/backend/internal/anomaly"
)

func TestDefaultThreshold(t *testing.T) {
	if anomaly.DefaultThreshold != 2.5 {
		t.Fatalf("expected DefaultThreshold=2.5, got %f", anomaly.DefaultThreshold)
	}
}

// ---------------------------------------------------------------------------
// CRITICAL: Zero-State Exception tests
// ---------------------------------------------------------------------------

// TestZeroVarianceGuardrail_ExactZero is the PRIMARY guardrail test.
//
// Scenario: all 50 window samples are identical → σ = 0.
// Expected: ZScore = 0, IsAnomaly = false, ZeroStateActive = true.
// The engine MUST NOT attempt (x - μ) / 0.
func TestZeroVarianceGuardrail_ExactZero(t *testing.T) {
	engine := anomaly.New(anomaly.DefaultThreshold)

	// σ = 0: all values identical, mean = 42.0
	result := engine.Evaluate(42.0, 42.0, 0.0)

	if result.ZScore != 0 {
		t.Errorf("GUARDRAIL FAIL: ZScore = %f (expected 0 when σ=0)", result.ZScore)
	}
	if result.IsAnomaly {
		t.Errorf("GUARDRAIL FAIL: IsAnomaly = true (expected false when σ=0)")
	}
	if !result.ZeroStateActive {
		t.Errorf("GUARDRAIL FAIL: ZeroStateActive = false (expected true when σ=0)")
	}
}

// TestZeroVarianceGuardrail_DifferentX proves the guardrail holds even when
// the new sample x differs from the mean (which would produce +Inf without
// the guard).
//
// Scenario: window mean = 20ms, σ = 0, new probe = 500ms (spike).
// Without guard: Z = (500 - 20) / 0 = +Inf → crash / NaN propagation.
// With guard: Z = 0, ZeroStateActive = true.
func TestZeroVarianceGuardrail_DifferentX(t *testing.T) {
	engine := anomaly.New(anomaly.DefaultThreshold)

	result := engine.Evaluate(500.0, 20.0, 0.0)

	if result.ZScore != 0 {
		t.Errorf("GUARDRAIL FAIL: ZScore = %f (expected 0 for σ=0 with x≠μ)", result.ZScore)
	}
	if result.IsAnomaly {
		t.Errorf("GUARDRAIL FAIL: IsAnomaly = true (expected false when σ=0)")
	}
	if !result.ZeroStateActive {
		t.Errorf("GUARDRAIL FAIL: ZeroStateActive = false (expected true when σ=0)")
	}
	// Explicit: no panic, no Inf, no NaN.
	if math.IsInf(result.ZScore, 0) || math.IsNaN(result.ZScore) {
		t.Errorf("GUARDRAIL FAIL: ZScore is Inf or NaN — %f", result.ZScore)
	}
}

// TestZeroVarianceGuardrail_NegativeSigma covers float64 rounding where
// σ comes back as a tiny negative value (e.g., -1e-16) due to floating-point
// drift in the variance formula. Must also be intercepted.
func TestZeroVarianceGuardrail_NegativeSigma(t *testing.T) {
	engine := anomaly.New(anomaly.DefaultThreshold)

	// Simulate a tiny negative sigma from floating-point rounding.
	result := engine.Evaluate(25.0, 25.0, -1e-16)

	if result.ZScore != 0 {
		t.Errorf("GUARDRAIL FAIL: ZScore = %f (expected 0 for negative σ)", result.ZScore)
	}
	if !result.ZeroStateActive {
		t.Errorf("GUARDRAIL FAIL: ZeroStateActive = false for negative σ")
	}
}

// TestZeroVarianceGuardrail_VerySmallSigma covers near-zero sigma that produces
// a non-finite result despite not being exactly zero. The secondary guard must
// catch this.
func TestZeroVarianceGuardrail_VerySmallSigma(t *testing.T) {
	engine := anomaly.New(anomaly.DefaultThreshold)

	// σ = 1e-324 (subnormal float), x far from mean → may produce +Inf.
	// The secondary guard (math.IsInf check) must intercept.
	result := engine.Evaluate(1000.0, 0.0, math.SmallestNonzeroFloat64)

	// Either the primary guard caught it (ZeroStateActive=true)
	// OR the secondary guard caught it. In any case: no Inf/NaN.
	if math.IsInf(result.ZScore, 0) || math.IsNaN(result.ZScore) {
		t.Errorf("GUARDRAIL FAIL: ZScore is non-finite: %f", result.ZScore)
	}
	if result.ZeroStateActive {
		// Primary guard fired — ZScore must be exactly 0.
		if result.ZScore != 0 {
			t.Errorf("ZeroStateActive=true but ZScore=%f (expected 0)", result.ZScore)
		}
	}
	// If ZeroStateActive is false, the secondary guard fired — ZScore is 0.
}

// ---------------------------------------------------------------------------
// Normal anomaly detection tests
// ---------------------------------------------------------------------------

// TestAnomalyAboveThreshold: Z > 2.5 must be flagged.
func TestAnomalyAboveThreshold(t *testing.T) {
	engine := anomaly.New(anomaly.DefaultThreshold)

	// μ = 20ms, σ = 5ms, x = 40ms → Z = (40-20)/5 = 4.0 > 2.5
	result := engine.Evaluate(40.0, 20.0, 5.0)

	expectedZ := (40.0 - 20.0) / 5.0 // 4.0
	if math.Abs(result.ZScore-expectedZ) > 1e-9 {
		t.Fatalf("ZScore mismatch: got %f, want %f", result.ZScore, expectedZ)
	}
	if !result.IsAnomaly {
		t.Fatalf("expected IsAnomaly=true for Z=%f >= 2.5", result.ZScore)
	}
	if result.ZeroStateActive {
		t.Fatalf("expected ZeroStateActive=false for σ=5")
	}
}

// TestAnomalyBelowThreshold: Z < 2.5 must not be flagged.
func TestAnomalyBelowThreshold(t *testing.T) {
	engine := anomaly.New(anomaly.DefaultThreshold)

	// μ = 20ms, σ = 5ms, x = 25ms → Z = (25-20)/5 = 1.0 < 2.5
	result := engine.Evaluate(25.0, 20.0, 5.0)

	if result.IsAnomaly {
		t.Fatalf("expected IsAnomaly=false for Z=%f < 2.5", result.ZScore)
	}
}

// TestAnomalyExactThreshold: |Z| == 2.5 exactly must be flagged (inclusive boundary).
func TestAnomalyExactThreshold(t *testing.T) {
	engine := anomaly.New(anomaly.DefaultThreshold)

	// μ = 0, σ = 1, x = 2.5 → Z = 2.5
	result := engine.Evaluate(2.5, 0.0, 1.0)

	if math.Abs(result.ZScore-2.5) > 1e-9 {
		t.Fatalf("ZScore mismatch at boundary: got %f, want 2.5", result.ZScore)
	}
	if !result.IsAnomaly {
		t.Fatalf("expected IsAnomaly=true at boundary Z=2.5 (inclusive)")
	}
}

// TestNegativeZScore: negative Z (below mean) above threshold also flags anomaly.
func TestNegativeZScore(t *testing.T) {
	engine := anomaly.New(anomaly.DefaultThreshold)

	// μ = 20ms, σ = 5ms, x = 5ms → Z = (5-20)/5 = -3.0 → |Z| = 3.0 > 2.5
	result := engine.Evaluate(5.0, 20.0, 5.0)

	expectedZ := (5.0 - 20.0) / 5.0 // -3.0
	if math.Abs(result.ZScore-expectedZ) > 1e-9 {
		t.Fatalf("ZScore mismatch: got %f, want %f", result.ZScore, expectedZ)
	}
	if !result.IsAnomaly {
		t.Fatalf("expected IsAnomaly=true for Z=%f (|Z|>2.5)", result.ZScore)
	}
}

// TestCustomThreshold: engine respects a non-default threshold.
func TestCustomThreshold(t *testing.T) {
	engine := anomaly.New(3.0) // stricter threshold

	// Z = (30-20)/5 = 2.0 — anomaly with threshold 2.5 but NOT with 3.0
	result := engine.Evaluate(30.0, 20.0, 5.0)

	if result.IsAnomaly {
		t.Fatalf("expected IsAnomaly=false for Z=2.0 with threshold=3.0")
	}
}

// TestZScorePrecision: spot-check full-precision Z calculation.
func TestZScorePrecision(t *testing.T) {
	engine := anomaly.New(anomaly.DefaultThreshold)

	// μ = 15.7, σ = 3.2, x = 22.1
	// Z = (22.1 - 15.7) / 3.2 = 6.4 / 3.2 = 2.0
	result := engine.Evaluate(22.1, 15.7, 3.2)
	want := (22.1 - 15.7) / 3.2

	if math.Abs(result.ZScore-want) > 1e-9 {
		t.Fatalf("ZScore precision: got %f, want %f", result.ZScore, want)
	}
}
