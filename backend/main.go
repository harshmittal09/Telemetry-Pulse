package main

import (
	"context"
	"log"
	"net/http"
	"telemetrypulse/internal/anomaly"
	"telemetrypulse/internal/engine"
	"telemetrypulse/internal/probe"
	"telemetrypulse/internal/pubsub"
	"telemetrypulse/internal/wsserver"
	"time"
)

func main() {

	urls := []string{"https://google.com", "https://github.com", "https://silvershinehouse.com"}
	interval := 10 * time.Second
	channel := make(chan probe.ProbeResult, 64)

	detector := anomaly.NewDetector()
	redisClient := pubsub.NewRedisClient()
	wsApp := wsserver.NewWSServer()
	history := engine.NewHistory(urls, 90, interval)

	pipeline := engine.NewPipeline(detector, redisClient, history)
	http.HandleFunc("/ws", wsApp.HandleComm)
	http.Handle("/api/history", history)

	go func() {
		err := http.ListenAndServe(":8080", nil)

		if err != nil {
			log.Fatalf("Websocket Server crashed: %v", err)
		}
	}()

	ctx := context.Background()
	for _, url := range urls {
		probe.StartWorker(ctx, url, interval, channel)
	}

	pipeline.Start(ctx, channel)

}
