// Package wsserver — server.go sets up the HTTP mux, CORS middleware,
// the WebSocket upgrade route, and the REST endpoints used by the
// failure-injection API (Phase 5).
package wsserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/telemetrypulse/backend/internal/config"
	"github.com/telemetrypulse/backend/internal/probe"
)

// Server wraps http.Server and wires all routes.
type Server struct {
	httpServer *http.Server
	hub        *Hub
	prober     *probe.SimulatedProber
}

// NewServer constructs the HTTP server with all routes registered.
//
// Routes:
//   GET  /ws              — WebSocket upgrade (handled by Hub)
//   GET  /health          — liveness probe
//   POST /api/simulate    — failure injection (Phase 5)
func NewServer(cfg config.WebSocketConfig, hub *Hub, prober *probe.SimulatedProber) *Server {
	mux := http.NewServeMux()

	// WebSocket endpoint.
	mux.Handle("/ws", withCORS(hub))

	// Health-check (used by load balancers / k8s probes).
	mux.Handle("/health", withCORS(http.HandlerFunc(healthHandler)))

	// Failure injection endpoint (Phase 5 activates this fully).
	mux.Handle("/api/simulate", withCORS(http.HandlerFunc(
		makeSimulateHandler(prober),
	)))

	s := &Server{
		httpServer: &http.Server{
			Addr:         cfg.ListenAddr,
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		hub:    hub,
		prober: prober,
	}
	return s
}

// Start begins listening. Blocks until ListenAndServe returns.
func (s *Server) Start() error {
	slog.Info("HTTP server starting", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully drains connections.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// ─────────────────────────────────────────────────────────────
// Route handlers
// ─────────────────────────────────────────────────────────────

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// SimulateRequest is the JSON body accepted by POST /api/simulate.
// Validated and documented here so Phase 5 frontend can rely on it.
type SimulateRequest struct {
	// LatencyMs is the artificial latency spike to inject (e.g. 2000).
	LatencyMs float64 `json:"latency_ms"`
	// PacketLoss is the artificial packet loss percentage (0–100).
	PacketLoss float64 `json:"packet_loss"`
}

// makeSimulateHandler returns an http.HandlerFunc that accepts a
// SimulateRequest and arms the SimulatedProber for one anomaly cycle.
func makeSimulateHandler(prober *probe.SimulatedProber) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req SimulateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		// Clamp values to sane ranges.
		if req.LatencyMs < 0 {
			req.LatencyMs = 0
		}
		if req.PacketLoss < 0 {
			req.PacketLoss = 0
		}
		if req.PacketLoss > 100 {
			req.PacketLoss = 100
		}

		prober.InjectAnomaly(req.LatencyMs, req.PacketLoss)

		slog.Info("Anomaly injection armed",
			"latency_ms", req.LatencyMs,
			"packet_loss", req.PacketLoss)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "injection_armed",
			"latency_ms": req.LatencyMs,
			"packet_loss": req.PacketLoss,
		})
	}
}

// ─────────────────────────────────────────────────────────────
// CORS middleware
// ─────────────────────────────────────────────────────────────

// withCORS wraps any handler to add permissive CORS headers for
// local development (Vite dev server on :5173).
// In production, restrict Access-Control-Allow-Origin to your domain.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
