package main

import (
	"log"
	"net/http"
	"telemetrypulse/internal/anomaly"
	"telemetrypulse/internal/engine"
	"telemetrypulse/internal/probe"
	"telemetrypulse/internal/pubsub"
	"telemetrypulse/internal/wsserver"
)

func main() {

	urls := []string{"https://google.com", "https://github.com", "https://silvershinehouse.com"}
	channel := make(chan probe.ProbeResult)

	detector := anomaly.NewDetector()
	redisClient := pubsub.NewRedisClient()
	wsApp := wsserver.NewWSServer()

	pipeline := engine.NewPipeline(detector, redisClient)
	http.HandleFunc("/ws", wsApp.HandleComm)

	go func() {
		err := http.ListenAndServe(":8080", nil)

		if err != nil {
			log.Fatalf("Websocket Server crashed: %v", err)
		}
	}()

	for _, url := range urls {
		probe.StartWorker(url, channel)
	}

	pipeline.Start(channel)

}
