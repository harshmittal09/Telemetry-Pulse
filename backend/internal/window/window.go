// Package window implements a fixed-size, O(1) sliding window data structure
// that maintains a running sum and running sum-of-squares for instant
// computation of moving mean (μ) and moving variance (σ²) without
// any linear scan across the stored samples.
//
// Complexity guarantees
// ---------------------
//   Add()  : O(1) — single eviction + update of two scalar accumulators.
//   Stats() : O(1) — direct read of pre-computed fields.
//   Memory  : O(N) — only N float64 values stored in a circular ring buffer.
package window

import (
	"math"
	"sync"
)

const (
	// DefaultWindowSize is the canonical window size mandated by the spec.
	DefaultWindowSize = 50
)

// SlidingWindow is a thread-safe, fixed-capacity circular buffer that
// keeps a running sum and running sum-of-squares so that μ and σ² are
// derivable in O(1) time on every insert.
//
// Mathematical identity used:
//
//	σ² = E[x²] - (E[x])²
//	   = (RunningSumSq / Count) - (RunningSum / Count)²
//
// This form avoids the numerically unstable two-pass algorithm and
// requires only two scalar accumulators.
type SlidingWindow struct {
	mu           sync.RWMutex
	capacity     int
	buf          []float64 // circular ring buffer, length == capacity
	head         int       // index of the OLDEST sample (next to evict)
	count        int       // number of valid samples currently stored (≤ capacity)
	runningSum   float64   // Σ xᵢ
	runningSumSq float64   // Σ xᵢ²
	prevLatency  float64   // last inserted value (for jitter calculation)
}

// New creates an initialised SlidingWindow with the given capacity.
// Panics if capacity < 1.
func New(capacity int) *SlidingWindow {
	if capacity < 1 {
		panic("window: capacity must be at least 1")
	}
	return &SlidingWindow{
		capacity: capacity,
		buf:      make([]float64, capacity),
	}
}

// Add inserts a new sample into the window.
//
// If the window is already full (count == capacity), the oldest sample
// is evicted first: its contribution is subtracted from both accumulators
// in O(1), then the new sample is appended and its contribution is added.
//
// Returns the computed Jitter (|current - previous|) before updating state.
func (w *SlidingWindow) Add(x float64) float64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	// --- Jitter -----------------------------------------------------------
	jitter := math.Abs(x - w.prevLatency)
	w.prevLatency = x

	// --- Eviction (O(1)) --------------------------------------------------
	if w.count == w.capacity {
		// The slot at w.head holds the oldest value.
		evicted := w.buf[w.head]
		w.runningSum -= evicted
		w.runningSumSq -= evicted * evicted
		// Advance head pointer with wrap-around.
		w.head = (w.head + 1) % w.capacity
	} else {
		w.count++
	}

	// --- Insertion (O(1)) -------------------------------------------------
	// The new sample occupies the slot just before head (modular arithmetic),
	// which is the tail of the logical queue.
	tail := (w.head + w.count - 1) % w.capacity
	w.buf[tail] = x
	w.runningSum += x
	w.runningSumSq += x * x

	return jitter
}

// Stats returns a snapshot of all derived statistics in O(1).
//
// Variance formula:
//
//	σ² = (Σxᵢ² / n) - (Σxᵢ / n)²
//
// This is the population variance of the window contents.
// StdDev is √σ². Both are 0 when count < 2 or all values are identical.
func (w *SlidingWindow) Stats() (mean, variance, stdDev float64, count int) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.count == 0 {
		return 0, 0, 0, 0
	}

	n := float64(w.count)
	mean = w.runningSum / n
	// E[x²] - (E[x])²
	variance = (w.runningSumSq / n) - (mean * mean)

	// Guard against floating-point drift producing tiny negative values.
	if variance < 0 {
		variance = 0
	}

	stdDev = math.Sqrt(variance)
	return mean, variance, stdDev, w.count
}

// Len returns the number of samples currently in the window.
func (w *SlidingWindow) Len() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.count
}

// Cap returns the fixed capacity of the window.
func (w *SlidingWindow) Cap() int {
	return w.capacity
}

// Reset clears all state, returning the window to its zero-value.
// Useful in testing and failure-injection scenarios.
func (w *SlidingWindow) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.head = 0
	w.count = 0
	w.runningSum = 0
	w.runningSumSq = 0
	w.prevLatency = 0
	// Zero out the buffer to prevent ghost reads.
	for i := range w.buf {
		w.buf[i] = 0
	}
}
