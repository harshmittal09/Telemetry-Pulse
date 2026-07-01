// Package window_test exhaustively tests the SlidingWindow implementation.
//
// Test coverage:
//   - Basic insertion and count tracking.
//   - O(1) running sum correctness (cross-validated against naive scan).
//   - Moving mean accuracy at various fill levels.
//   - Moving variance accuracy (vs brute-force two-pass).
//   - Eviction correctness when the window overflows.
//   - Jitter computation.
//   - Thread-safety under concurrent access.
//   - Reset behaviour.
package window_test

import (
	"math"
	"sync"
	"testing"

	"github.com/telemetrypulse/backend/internal/window"
)

// tolerance for float64 comparison.
const eps = 1e-9

func floatEq(a, b float64) bool {
	return math.Abs(a-b) < eps
}

// bruteForceStats computes mean and variance via a naive two-pass scan.
// Used as a reference oracle in tests.
func bruteForceStats(samples []float64) (mean, variance float64) {
	if len(samples) == 0 {
		return 0, 0
	}
	n := float64(len(samples))
	for _, v := range samples {
		mean += v
	}
	mean /= n
	for _, v := range samples {
		d := v - mean
		variance += d * d
	}
	variance /= n
	return mean, variance
}

// TestNew verifies that a freshly created window is empty.
func TestNew(t *testing.T) {
	w := window.New(50)
	if w.Len() != 0 {
		t.Fatalf("expected Len=0 on new window, got %d", w.Len())
	}
	if w.Cap() != 50 {
		t.Fatalf("expected Cap=50, got %d", w.Cap())
	}
	mean, variance, stdDev, count := w.Stats()
	if count != 0 || mean != 0 || variance != 0 || stdDev != 0 {
		t.Fatalf("expected all-zero stats on empty window")
	}
}

// TestSingleInsert verifies stats after one insertion (variance must be 0).
func TestSingleInsert(t *testing.T) {
	w := window.New(50)
	w.Add(42.0)
	mean, variance, stdDev, count := w.Stats()

	if count != 1 {
		t.Fatalf("expected count=1, got %d", count)
	}
	if !floatEq(mean, 42.0) {
		t.Fatalf("expected mean=42.0, got %f", mean)
	}
	if !floatEq(variance, 0.0) {
		t.Fatalf("expected variance=0 for single sample, got %f", variance)
	}
	if !floatEq(stdDev, 0.0) {
		t.Fatalf("expected stdDev=0 for single sample, got %f", stdDev)
	}
}

// TestMeanAccuracy adds N < capacity values and checks mean against oracle.
func TestMeanAccuracy(t *testing.T) {
	w := window.New(50)
	samples := []float64{10, 20, 30, 40, 50}
	for _, s := range samples {
		w.Add(s)
	}
	mean, _, _, _ := w.Stats()
	oracleMean, _ := bruteForceStats(samples)
	if !floatEq(mean, oracleMean) {
		t.Fatalf("mean mismatch: got %f, want %f", mean, oracleMean)
	}
}

// TestVarianceAccuracy fills the window to capacity and validates variance
// against a brute-force two-pass computation.
func TestVarianceAccuracy(t *testing.T) {
	w := window.New(10)
	samples := []float64{1, 4, 9, 16, 25, 36, 49, 64, 81, 100}
	for _, s := range samples {
		w.Add(s)
	}
	_, variance, stdDev, _ := w.Stats()
	_, oracleVar := bruteForceStats(samples)
	oracleStdDev := math.Sqrt(oracleVar)

	if math.Abs(variance-oracleVar) > 1e-6 {
		t.Fatalf("variance mismatch: got %f, want %f", variance, oracleVar)
	}
	if math.Abs(stdDev-oracleStdDev) > 1e-6 {
		t.Fatalf("stdDev mismatch: got %f, want %f", stdDev, oracleStdDev)
	}
}

// TestEviction verifies that after the window fills and overflows, the
// oldest values are correctly evicted and the running stats remain accurate.
func TestEviction(t *testing.T) {
	capacity := 5
	w := window.New(capacity)

	// Fill the window: 1, 2, 3, 4, 5
	for i := 1; i <= capacity; i++ {
		w.Add(float64(i))
	}

	// Add a 6th value — this should evict 1.
	w.Add(6.0)

	// Window should now contain [2, 3, 4, 5, 6].
	expectedSamples := []float64{2, 3, 4, 5, 6}
	gotMean, gotVar, _, gotCount := w.Stats()
	wantMean, wantVar := bruteForceStats(expectedSamples)

	if gotCount != capacity {
		t.Fatalf("expected count=%d after overflow, got %d", capacity, gotCount)
	}
	if math.Abs(gotMean-wantMean) > 1e-9 {
		t.Fatalf("mean after eviction: got %f, want %f", gotMean, wantMean)
	}
	if math.Abs(gotVar-wantVar) > 1e-9 {
		t.Fatalf("variance after eviction: got %f, want %f", gotVar, wantVar)
	}
}

// TestJitter verifies that the returned jitter equals |current - previous|.
func TestJitter(t *testing.T) {
	w := window.New(10)
	w.Add(20.0) // first; no prev — jitter == 20
	j1 := w.Add(30.0)
	if !floatEq(j1, 10.0) {
		t.Fatalf("expected jitter=10, got %f", j1)
	}
	j2 := w.Add(25.0)
	if !floatEq(j2, 5.0) {
		t.Fatalf("expected jitter=5, got %f", j2)
	}
	j3 := w.Add(25.0) // same value
	if !floatEq(j3, 0.0) {
		t.Fatalf("expected jitter=0 for identical consecutive samples, got %f", j3)
	}
}

// TestReset verifies that Reset clears all state.
func TestReset(t *testing.T) {
	w := window.New(10)
	for i := 0; i < 10; i++ {
		w.Add(float64(i))
	}
	w.Reset()
	mean, variance, stdDev, count := w.Stats()
	if count != 0 || mean != 0 || variance != 0 || stdDev != 0 {
		t.Fatalf("expected all-zero stats after Reset()")
	}
}

// TestConcurrentAdd stress-tests the window under concurrent goroutine access.
// The test does not check specific values — it verifies there are no data races
// (run with -race flag) and no panics.
func TestConcurrentAdd(t *testing.T) {
	w := window.New(50)
	var wg sync.WaitGroup
	numGoroutines := 100
	insertsPerGoroutine := 200

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(base float64) {
			defer wg.Done()
			for j := 0; j < insertsPerGoroutine; j++ {
				w.Add(base + float64(j))
				w.Stats() // concurrent reads mixed with writes
			}
		}(float64(i))
	}
	wg.Wait()
	// Window must remain consistent.
	_, _, _, count := w.Stats()
	if count > 50 {
		t.Fatalf("count exceeded capacity: %d", count)
	}
}
