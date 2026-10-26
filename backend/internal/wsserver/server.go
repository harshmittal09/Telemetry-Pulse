package wsserver

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"telemetrypulse/internal/pubsub"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 50 * time.Second // has to be less than pongWait
)

type Wsserver struct {
	ctx         context.Context
	redisClient *pubsub.RedisClient
	upgrader    websocket.Upgrader
}

func NewWSServer(ctx context.Context, redisClient *pubsub.RedisClient, allowedOrigins []string) *Wsserver {
	return &Wsserver{
		ctx:         ctx,
		redisClient: redisClient,
		upgrader: websocket.Upgrader{
			CheckOrigin: originChecker(allowedOrigins),
		},
	}
}

// pages served by this same server can always connect, other sites only if listed
func originChecker(allowed []string) func(r *http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // not a browser
		}

		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		if strings.EqualFold(u.Host, r.Host) {
			return true
		}

		for _, a := range allowed {
			if strings.EqualFold(origin, a) {
				return true
			}
		}
		return false
	}
}

func (ws *Wsserver) HandleComm(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(ws.ctx)
	defer cancel()

	sub := ws.redisClient.Subscribe(ctx)
	defer sub.Close()

	// the browser never sends us anything, but reading is the only way to
	// notice it closed the tab, without this the redis subscription leaked
	go func() {
		defer cancel()
		conn.SetReadLimit(512)
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(pongWait))
		})
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ping := time.NewTicker(pingPeriod)
	defer ping.Stop()

	messages := sub.Channel()

	for {
		select {
		case <-ctx.Done():
			return

		case msg, ok := <-messages:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				return
			}

		case <-ping.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
