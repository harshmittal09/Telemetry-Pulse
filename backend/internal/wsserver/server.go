package wsserver

import (
	"context"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Wsserver struct {
	redisClient *redis.Client
}

func NewWSServer() *Wsserver {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	return &Wsserver{
		redisClient: rdb,
	}
}

func (ws *Wsserver) HandleComm(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	ctx := context.Background()
	pubsub := ws.redisClient.Subscribe(ctx, "telemetry:alerts")
	defer pubsub.Close()

	ch := pubsub.Channel()

	for msg := range ch {
		err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload))
		if err != nil {
			log.Printf("Write Error: %v", err)
			break
		}
	}

}
