package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"telemetrypulse/internal/anomaly"
	"telemetrypulse/internal/config"
	"telemetrypulse/internal/engine"
	"telemetrypulse/internal/probe"
	"telemetrypulse/internal/pubsub"
	"telemetrypulse/internal/wsserver"
)

func main() {
	cfg := config.Load()

	// Ctrl+C locally, SIGTERM when the host restarts or redeploys us
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	redisClient, err := pubsub.NewRedisClient(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Invalid REDIS_URL: %v", err)
	}
	defer redisClient.Close()

	if err := redisClient.WaitReady(ctx, 10); err != nil {
		log.Fatalf("Redis is not reachable: %v", err)
	}

	detector := anomaly.NewDetector()
	history := engine.NewHistory(cfg.Targets, 90, cfg.ProbeInterval)
	pipeline := engine.NewPipeline(detector, redisClient, history)
	wsApp := wsserver.NewWSServer(ctx, redisClient, cfg.AllowedOrigins)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsApp.HandleComm)
	mux.Handle("/api/history", history)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	if _, err := os.Stat(cfg.StaticDir); err != nil {
		log.Printf("No dashboard found at %s, serving only the API", cfg.StaticDir)
	} else {
		mux.Handle("/", http.FileServer(http.Dir(cfg.StaticDir)))
	}

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Listening on :%s, probing %d targets every %s", cfg.Port, len(cfg.Targets), cfg.ProbeInterval)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server crashed: %v", err)
		}
	}()

	channel := make(chan probe.ProbeResult, 64)
	for _, url := range cfg.Targets {
		probe.StartWorker(ctx, url, cfg.ProbeInterval, channel)
	}

	pipeline.Start(ctx, channel) // blocks until shutdown

	log.Println("Shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
