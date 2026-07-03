// Package probe — system.go
//
// SystemProber captures live Go runtime memory statistics and internal
// network log summaries. It emulates what a production agent would collect
// from a running service, providing insight into GC pressure, heap
// allocation trends, and goroutine counts.
//
// This probe runs on a separate interval from the network probes and
// publishes to the same Redis Pub/Sub pipeline under its own
// EndpointID ("system-local").
package probe

import (
	"context"
	"runtime"
	"time"

	"github.com/telemetrypulse/backend/pkg/models"
)

// SystemProber captures application-level memory and runtime telemetry
// using Go's built-in runtime.MemStats. No external dependencies required.
type SystemProber struct{}

// NewSystemProber returns a ready-to-use SystemProber.
func NewSystemProber() *SystemProber {
	return &SystemProber{}
}

// ProbeSystem captures a single system metrics snapshot and returns a
// SystemMetrics value. It honours context cancellation.
func (s *SystemProber) ProbeSystem(ctx context.Context) (models.SystemMetrics, error) {
	select {
	case <-ctx.Done():
		return models.SystemMetrics{}, ctx.Err()
	default:
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	return models.SystemMetrics{
		Timestamp: time.Now().UTC(),

		// Heap memory
		HeapAllocBytes:  ms.HeapAlloc,  // bytes currently allocated on heap
		HeapSysBytes:    ms.HeapSys,    // bytes obtained from OS for heap
		HeapInuseBytes:  ms.HeapInuse,  // bytes in in-use spans
		HeapIdleBytes:   ms.HeapIdle,   // bytes in idle spans
		HeapReleasedBytes: ms.HeapReleased, // bytes returned to OS

		// GC pressure indicators
		NumGC:        ms.NumGC,        // total GC cycles completed
		NumForcedGC:  ms.NumForcedGC,  // GC cycles forced by runtime.GC()
		PauseTotalNs: ms.PauseTotalNs, // cumulative GC pause time (nanoseconds)
		LastGCPauseNs: func() uint64 {
			if ms.NumGC > 0 {
				return ms.PauseNs[(ms.NumGC+255)%256]
			}
			return 0
		}(),

		// Allocation throughput
		TotalAllocBytes: ms.TotalAlloc, // cumulative bytes allocated (ever)
		MallocCount:     ms.Mallocs,    // cumulative alloc count
		FreeCount:       ms.Frees,      // cumulative free count

		// Concurrency
		NumGoroutines: uint64(runtime.NumGoroutine()),
	}, nil
}

// SystemDispatcher is a dedicated probe loop for system metrics.
// It is intentionally separate from the network Dispatcher so it can
// run on an independent interval (e.g., every 2s vs 500ms).
type SystemDispatcher struct {
	prober   *SystemProber
	callback SystemResultCallback
	interval time.Duration
}

// SystemResultCallback receives each SystemMetrics snapshot.
type SystemResultCallback func(m models.SystemMetrics)

// NewSystemDispatcher creates a SystemDispatcher.
func NewSystemDispatcher(prober *SystemProber, interval time.Duration, cb SystemResultCallback) *SystemDispatcher {
	return &SystemDispatcher{
		prober:   prober,
		callback: cb,
		interval: interval,
	}
}

// Start launches the system probe loop. Runs until ctx is cancelled.
func (d *SystemDispatcher) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(d.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m, err := d.prober.ProbeSystem(ctx)
				if err != nil {
					return // context cancelled
				}
				d.callback(m)
			}
		}
	}()
}
