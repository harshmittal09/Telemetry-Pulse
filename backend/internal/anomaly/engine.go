// Package anomaly implements the Z-Score anomaly detection engine.
//
// Z-Score formula:
//
//	Z = (x - μ) / σ
//
// An anomaly is declared when |Z| >= threshold (default 2.5).
//
// CRITICAL GUARDRAIL — Zero-State Exception:
//
//	When σ = 0 (all window values are identical, or the window has < 2
//	samples), division by zero is a mathematical impossibility. This engine
//	explicitly intercepts that case and hard-codes Z = 0. This is
//	semantically correct: a value that equals the mean in a zero-variance
//	distribution deviates by exactly zero standard deviations.
package anomaly

import "math"

const (
	// DefaultThreshold is the absolute Z-score boundary above which a
	// data point is classified as anomalous (|Z| >= 2.5 → ~1.24% of
	// normally distributed samples under the tail).
	DefaultThreshold = 2.5
)

// Engine holds configuration for the anomaly detector.
// It is intentionally stateless — all state lives in the SlidingWindow.
type Engine struct {
	Threshold float64
}

// New returns an Engine with the given threshold.
// Pass DefaultThreshold for standard operation.
func New(threshold float64) *Engine {
	return &Engine{Threshold: threshold}
}

// ZScoreResult is the fully computed output of a single anomaly evaluation.
type ZScoreResult struct {
	// ZScore is the computed Z = (x - μ) / σ.
	// Hard-coded to 0 when σ == 0 (Zero-State Exception).
	ZScore float64

	// IsAnomaly is true when math.Abs(ZScore) >= Engine.Threshold.
	IsAnomaly bool

	// ZeroStateActive is true when σ == 0, signalling that the guardrail
	// fired and the Z-score was forced to 0. Useful for observability logs.
	ZeroStateActive bool
}

// Evaluate computes the Z-score of a new sample x given the pre-computed
// window statistics (mean μ, standard deviation σ).
//
// Parameters:
//   - x    : the incoming latency sample being evaluated.
//   - mean : μ from SlidingWindow.Stats().
//   - sigma: σ (standard deviation) from SlidingWindow.Stats().
//
// ZERO-STATE GUARDRAIL:
//
//	If sigma == 0 (OR is so small it would produce a non-finite result
//	after division), the function immediately returns ZScore=0,
//	IsAnomaly=false, ZeroStateActive=true.
//
//	We guard against BOTH the exact zero AND IEEE-754 underflow cases
//	(very small σ that yields ±Inf) by checking math.IsInf / math.IsNaN
//	on the computed result.
func (e *Engine) Evaluate(x, mean, sigma float64) ZScoreResult {
	// -----------------------------------------------------------------------
	// CRITICAL GUARDRAIL: Zero-State Exception
	// -----------------------------------------------------------------------
	// Primary guard — exact zero or negative (due to float rounding) sigma.
	if sigma <= 0 {
		return ZScoreResult{
			ZScore:          0,
			IsAnomaly:       false,
			ZeroStateActive: true,
		}
	}

	z := (x - mean) / sigma

	// Secondary guard — catches ±Inf or NaN produced by denormalised floats.
	if math.IsInf(z, 0) || math.IsNaN(z) {
		return ZScoreResult{
			ZScore:          0,
			IsAnomaly:       false,
			ZeroStateActive: true,
		}
	}

	return ZScoreResult{
		ZScore:          z,
		IsAnomaly:       math.Abs(z) >= e.Threshold,
		ZeroStateActive: false,
	}
}
