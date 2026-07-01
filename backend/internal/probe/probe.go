// Package probe implements simulated ICMP/TCP network probes.
//
// In a production system these would issue real ICMP echo requests
// (requiring raw socket privileges) or establish real TCP connections.
// For this implementation we use a mathematically faithful simulation that
// produces realistic latency distributions (log-normal base + spikes) so
// the statistical engine has meaningful data to process.
//
// Each probe runs in its own Goroutine, dispatched by the Dispatcher.
package probe

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/telemetrypulse/backend/pkg/models"
)

// Prober defines the interface for a single endpoint probe execution.
type Prober interface {
	// Probe executes one probe cycle and returns a ProbeResult.
	// It must honour context cancellation (for graceful shutdown).
	Probe(ctx context.Context, cfg models.EndpointConfig) (models.ProbeResult, error)
}

// SimulatedProber is the default Prober implementation.
// It simulates realistic network latency using a log-normal distribution
// (matching empirical network RTT distributions) with configurable
// failure injection hooks.
type SimulatedProber struct {
	mu sync.RWMutex

	// anomalyInjected is flipped by the failure-injection API (Phase 5).
	// When true, the next probe produces an artificial spike.
	anomalyInjected bool

	// injectedLatencyMs is the artificial latency to inject when anomalyInjected == true.
	injectedLatencyMs float64

	// injectedPacketLoss is the artificial packet loss (0–100) to inject.
	injectedPacketLoss float64
}

// NewSimulatedProber creates a SimulatedProber with default (non-injected) state.
func NewSimulatedProber() *SimulatedProber {
	return &SimulatedProber{}
}

// InjectAnomaly arms the prober with artificial failure parameters.
// The injected values will be used for the NEXT probe cycle, then cleared.
func (p *SimulatedProber) InjectAnomaly(latencyMs, packetLoss float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.anomalyInjected = true
	p.injectedLatencyMs = latencyMs
	p.injectedPacketLoss = packetLoss
}

// Probe executes a single synthetic probe.
//
// Normal operation: latency is sampled from a log-normal distribution
// with μ_ln = 3.0, σ_ln = 0.3 (giving a mean ≈ e^(3+0.045) ≈ 22ms,
// matching typical intra-region cloud latency).
//
// Injected anomaly: replaces all computed values with injected constants,
// then clears the injection flag (single-use).
func (p *SimulatedProber) Probe(ctx context.Context, cfg models.EndpointConfig) (models.ProbeResult, error) {
	select {
	case <-ctx.Done():
		return models.ProbeResult{}, ctx.Err()
	default:
	}

	// Check and consume injection state atomically.
	p.mu.Lock()
	injected := p.anomalyInjected
	injLatency := p.injectedLatencyMs
	injPacketLoss := p.injectedPacketLoss
	if injected {
		p.anomalyInjected = false // single-use; clear after consumption
	}
	p.mu.Unlock()

	var latency float64
	var packetLoss float64
	var reachable bool

	if injected {
		latency = injLatency
		packetLoss = injPacketLoss
		reachable = packetLoss < 100
	} else {
		latency = sampleLogNormal(3.0, 0.3)
		packetLoss = samplePacketLoss()
		reachable = packetLoss < 100
	}

	return models.ProbeResult{
		EndpointID:  cfg.ID,
		Target:      cfg.Target,
		Protocol:    cfg.Protocol,
		Latency:     latency,
		PacketLoss:  packetLoss,
		IsReachable: reachable,
		Timestamp:   time.Now().UTC(),
	}, nil
}

// ---------------------------------------------------------------------------
// Statistical sampling helpers
// ---------------------------------------------------------------------------

// sampleLogNormal draws from a log-normal distribution using the Box-Muller
// transform. Parameters mu and sigma are the underlying normal's mean and
// standard deviation (not the log-normal's).
func sampleLogNormal(mu, sigma float64) float64 {
	// Box-Muller: Z = sqrt(-2*ln(U1)) * cos(2π*U2)
	u1 := rand.Float64()
	u2 := rand.Float64()
	// Avoid log(0)
	if u1 == 0 {
		u1 = 1e-10
	}
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	return math.Exp(mu + sigma*z)
}

// samplePacketLoss returns a simulated packet loss percentage.
// 95% of probes have 0% loss; 4% have 1–10%; 1% simulate total outage.
func samplePacketLoss() float64 {
	r := rand.Float64()
	switch {
	case r < 0.95:
		return 0
	case r < 0.99:
		return rand.Float64() * 10
	default:
		return 100
	}
}

// ---------------------------------------------------------------------------
// Dispatcher — concurrent probe runner
// ---------------------------------------------------------------------------

// ResultCallback is called by the Dispatcher each time a probe cycle
// completes. It is invoked from within a Goroutine so implementations
// must be safe for concurrent use.
type ResultCallback func(result models.ProbeResult)

// Dispatcher manages a pool of probe Goroutines, one per endpoint.
// Each Goroutine runs on its own independent ticker so endpoints do not
// block each other.
type Dispatcher struct {
	prober   Prober
	callback ResultCallback
	wg       sync.WaitGroup
	mu       sync.RWMutex
	active   bool
}

// NewDispatcher creates a Dispatcher with the given Prober and callback.
func NewDispatcher(prober Prober, callback ResultCallback) *Dispatcher {
	return &Dispatcher{
		prober:   prober,
		callback: callback,
		active:   false, // Start inactive until clients connect
	}
}

// SetActive toggles the dispatcher's polling state.
func (d *Dispatcher) SetActive(active bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.active = active
}

// IsActive returns whether the dispatcher should actively poll.
func (d *Dispatcher) IsActive() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.active
}

// Start launches one Goroutine per endpoint config.
// The Goroutines run until ctx is cancelled (graceful shutdown).
func (d *Dispatcher) Start(ctx context.Context, endpoints []models.EndpointConfig) {
	for _, ep := range endpoints {
		ep := ep // capture loop variable
		d.wg.Add(1)
		go d.runProbeLoop(ctx, ep)
	}
}

// Wait blocks until all probe Goroutines have exited.
func (d *Dispatcher) Wait() {
	d.wg.Wait()
}

// runProbeLoop is the per-endpoint Goroutine. It fires a probe on the
// configured interval, then passes the result to the callback.
func (d *Dispatcher) runProbeLoop(ctx context.Context, cfg models.EndpointConfig) {
	defer d.wg.Done()

	interval := time.Duration(cfg.ProbeIntervalMs) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !d.IsActive() {
				continue // Demand-driven: skip probe when no clients connected
			}
			result, err := d.prober.Probe(ctx, cfg)
			if err != nil {
				// Context cancelled — exit cleanly.
				return
			}
			d.callback(result)
		}
	}
}
