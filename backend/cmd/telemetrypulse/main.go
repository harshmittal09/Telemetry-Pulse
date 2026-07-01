// cmd/telemetrypulse/main.go — Phase 2 entry point.
//
// Full system wiring:
//
//  probe.Dispatcher (3 goroutines, one per endpoint)
//       │  ProbeResult callback
//       ▼
//  telemetry.Processor
//       │  PublishFunc → pubsub.Publisher.Publish()
//       ▼
//  Redis PUBLISH "telemetrypulse:telemetry"
//       │
//  pubsub.Subscriber.Run() (dedicated goroutine)
//       │  MessageHandler → hub.Ingest()
//       ▼
//  wsserver.Hub.Run() (dedicated goroutine)
//       │  500ms ticker → buildFrame() → broadcast
//       ▼
//  WebSocket clients (browser)
//
//  HTTP endpoints:
//    GET  /ws            → WebSocket upgrade
//    GET  /health        → liveness probe
//    POST /api/simulate  → failure injection (Phase 5 frontend)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/telemetrypulse/backend/internal/config"
	"github.com/telemetrypulse/backend/internal/probe"
	"github.com/telemetrypulse/backend/internal/pubsub"
	"github.com/telemetrypulse/backend/internal/telemetry"
	"github.com/telemetrypulse/backend/internal/wsserver"
	"github.com/telemetrypulse/backend/pkg/models"
)

func main() {
	// ── Structured logging ─────────────────────────────────────────────────
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// ── Configuration ──────────────────────────────────────────────────────
	cfg := config.Load()

	// ── Redis Publisher ────────────────────────────────────────────────────
	publisher, err := pubsub.NewPublisher(cfg.Redis)
	if err != nil {
		slog.Error("Failed to connect Redis publisher", "error", err)
		os.Exit(1)
	}
	defer publisher.Close()

	// ── Telemetry Processor ────────────────────────────────────────────────
	// The PublishFunc is the integration seam between Phase 1 and Phase 2.
	// We run the Redis publish in a goroutine to avoid blocking the probe
	// loop if Redis is temporarily slow.
	bgCtx := context.Background()
	processor := telemetry.NewProcessor(func(p models.TelemetryPayload) {
		if err := publisher.Publish(bgCtx, p); err != nil {
			slog.Warn("Redis publish error", "error", err, "endpoint", p.EndpointID)
		}
	})

	// ── Probe Dispatcher ───────────────────────────────────────────────────
	prober := probe.NewSimulatedProber()
	dispatcher := probe.NewDispatcher(prober, processor.Process)

	endpoints := []models.EndpointConfig{
		{ID: "ep-01", Target: "8.8.8.8", Protocol: "icmp", ProbeIntervalMs: cfg.Probes.IntervalMs},
		{ID: "ep-02", Target: "1.1.1.1", Protocol: "icmp", ProbeIntervalMs: cfg.Probes.IntervalMs},
		{ID: "ep-03", Target: "api.example.com:443", Protocol: "tcp", ProbeIntervalMs: cfg.Probes.IntervalMs},
	}

	// ── WebSocket Hub ──────────────────────────────────────────────────────
	hub := wsserver.NewHub(cfg.WebSocket)

	// ── Redis Subscriber ───────────────────────────────────────────────────
	// The subscriber feeds decoded payloads directly into the hub's ingest
	// channel. The hub accumulates them and broadcasts a snapshot every 500ms.
	subscriber, err := pubsub.NewSubscriber(cfg.Redis, hub.Ingest)
	if err != nil {
		slog.Error("Failed to connect Redis subscriber", "error", err)
		os.Exit(1)
	}
	defer subscriber.Close()

	// ── HTTP Server ────────────────────────────────────────────────────────
	server := wsserver.NewServer(cfg.WebSocket, hub, prober)

	// ── Graceful shutdown context ──────────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── Start all subsystems ───────────────────────────────────────────────
	slog.Info("TelemetryPulse starting",
		"listen_addr", cfg.WebSocket.ListenAddr,
		"redis_url", cfg.Redis.URL,
		"redis_channel", cfg.Redis.PubSubChannel,
		"probe_interval_ms", cfg.Probes.IntervalMs,
		"endpoints", len(endpoints),
	)

	// Hub event loop.
	hubDone := make(chan struct{})
	go func() {
		hub.Run(ctx.Done())
		close(hubDone)
	}()

	// Redis subscriber loop.
	go func() {
		if err := subscriber.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("Redis subscriber error", "error", err)
		}
	}()

	// Probe dispatcher — one goroutine per endpoint.
	dispatcher.Start(ctx, endpoints)

	// HTTP server — runs until ctx cancellation triggers Shutdown().
	go func() {
		if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server error", "error", err)
			stop() // propagate fatal error to shutdown sequence
		}
	}()

	// ── Wait for shutdown signal ───────────────────────────────────────────
	<-ctx.Done()
	slog.Info("Shutdown signal received")

	// Give in-flight requests 10 seconds to drain.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	// Wait for probes to drain.
	dispatcher.Wait()

	// Wait for hub to finish broadcasting.
	<-hubDone

	slog.Info("TelemetryPulse stopped cleanly")
}
