// Package wsserver implements the WebSocket server and connection hub.
//
// Architecture
// ─────────────────────────────────────────────────────────────
//
//  HTTP GET /ws
//       │  gorilla/websocket Upgrade
//       ▼
//  client (per connection)
//       │  registers with Hub
//       ▼
//  Hub
//   ├─ register chan    ← new client connects
//   ├─ unregister chan  ← client disconnects
//   ├─ ingest chan      ← TelemetryPayload from Redis subscriber
//   └─ 500ms Ticker ──► broadcast latest snapshot to ALL clients
//
// Broadcast Strategy — Throttled Snapshot Fan-out
// ─────────────────────────────────────────────────
// The Redis subscriber may receive payloads faster than 500ms (if multiple
// endpoints fire simultaneously). The hub accumulates them in a keyed map
// (endpointID → latest payload) and on every 500ms tick broadcasts the
// ENTIRE current snapshot as a single JSON frame. This means:
//   - Clients always receive the freshest data per endpoint.
//   - No dropped frames — only the stale intermediate values are discarded
//     (acceptable for a monitoring dashboard).
//   - Broadcast cost is O(clients × endpoints), not O(clients × messages).
//
// Client Write Safety
// ─────────────────────
// Each client owns a buffered outbound channel (chanSize=256). The hub
// writes to the channel non-blocking; if the channel is full the client
// is considered stalled and is forcibly unregistered.
// The actual WebSocket write runs in a per-client writePump goroutine, so
// hub.run() is never blocked by slow network I/O.
package wsserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/telemetrypulse/backend/internal/config"
	"github.com/telemetrypulse/backend/pkg/models"
)

// ─────────────────────────────────────────────────────────────
// Payload envelope sent over WebSocket
// ─────────────────────────────────────────────────────────────

// WSMessage is the strictly typed JSON frame sent to browser clients.
// It wraps a slice of TelemetryPayload (one per endpoint) so a single
// WebSocket frame carries the full system snapshot.
type WSMessage struct {
	// Type identifies the frame kind. Currently always "telemetry_snapshot".
	Type string `json:"type"`
	// Timestamp is the server-side broadcast time (UTC ISO-8601).
	Timestamp string `json:"timestamp"`
	// Payloads is the current snapshot — one entry per monitored endpoint.
	Payloads []models.TelemetryPayload `json:"payloads"`
}

// ─────────────────────────────────────────────────────────────
// client — represents one connected browser
// ─────────────────────────────────────────────────────────────

const (
	outboundBufSize = 256
)

type client struct {
	id   string
	conn *websocket.Conn
	// send is a buffered channel of pre-serialised JSON frames.
	send chan []byte
	hub  *Hub
	cfg  config.WebSocketConfig
}

// writePump runs in a dedicated goroutine per client.
// It drains the send channel and writes frames to the WebSocket connection.
// A ping ticker keeps the connection alive across idle intervals.
func (c *client) writePump() {
	pingTicker := time.NewTicker(c.cfg.PingInterval)
	defer func() {
		pingTicker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
			if !ok {
				// Hub closed the channel — send a close frame.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				slog.Debug("ws: write error — dropping client",
					"client_id", c.id, "error", err)
				return
			}

		case <-pingTicker.C:
			c.conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump runs in a dedicated goroutine per client.
// It discards all incoming data (clients are read-only in normal operation)
// but drives the pong handler to reset the read deadline, keeping the
// connection detection alive. If the connection goes idle beyond PongTimeout
// the read returns an error and the goroutine exits, triggering unregister.
func (c *client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(c.cfg.MaxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(c.cfg.PongTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(c.cfg.PongTimeout))
		return nil
	})

	for {
		// Discard incoming messages; exit on error (disconnect / timeout).
		if _, _, err := c.conn.ReadMessage(); err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure) {
				slog.Debug("ws: unexpected close", "client_id", c.id, "error", err)
			}
			return
		}
	}
}

// ─────────────────────────────────────────────────────────────
// Hub — the broadcast coordinator
// ─────────────────────────────────────────────────────────────

// Hub manages the set of active WebSocket clients and coordinates broadcasts.
// All mutations to the client set happen inside hub.run() — no external
// locking is required.
type Hub struct {
	// mu protects snapshot for concurrent reads from ServeHTTP (health) etc.
	mu sync.RWMutex

	// snapshot is the latest per-endpoint telemetry data, keyed by EndpointID.
	// Updated by Ingest(); read-snapshotted on each broadcast tick.
	snapshot map[models.EndpointID]models.TelemetryPayload

	// clients is the live set of connected WebSocket clients.
	clients map[*client]struct{}

	// register is the channel for new client connections.
	register chan *client
	// unregister is the channel for disconnected clients.
	unregister chan *client
	// ingest is the channel for incoming telemetry payloads from Redis.
	ingest chan models.TelemetryPayload

	// OnClientCountChange is called whenever a client connects or disconnects.
	OnClientCountChange func(count int)

	cfg config.WebSocketConfig
}

// NewHub creates a Hub with the given WebSocket config.
func NewHub(cfg config.WebSocketConfig) *Hub {
	return &Hub{
		snapshot:   make(map[models.EndpointID]models.TelemetryPayload),
		clients:    make(map[*client]struct{}),
		register:   make(chan *client, 32),
		unregister: make(chan *client, 32),
		ingest:     make(chan models.TelemetryPayload, 1024),
		cfg:        cfg,
	}
}

// Ingest is called by the Redis subscriber to deliver a new payload.
// It is safe to call from any goroutine. Non-blocking: drops if full.
func (h *Hub) Ingest(payload models.TelemetryPayload) {
	select {
	case h.ingest <- payload:
	default:
		slog.Warn("ws hub: ingest channel full — dropping payload",
			"endpoint_id", payload.EndpointID)
	}
}

// Run is the hub's main event loop. It must be called in a dedicated
// goroutine and runs until ctx is done. It:
//  1. Accepts register/unregister events.
//  2. Updates the snapshot on ingest.
//  3. Broadcasts the full snapshot to all clients every 500ms.
func (h *Hub) Run(done <-chan struct{}) {
	ticker := time.NewTicker(h.cfg.BroadcastInterval)
	defer ticker.Stop()

	slog.Info("ws hub: starting", "broadcast_interval", h.cfg.BroadcastInterval)

	for {
		select {
		case <-done:
			slog.Info("ws hub: shutting down")
			// Close all client send channels to trigger clean close frames.
			for c := range h.clients {
				close(c.send)
				delete(h.clients, c)
			}
			return

		case c := <-h.register:
			h.clients[c] = struct{}{}
			slog.Info("ws hub: client registered",
				"client_id", c.id, "total_clients", len(h.clients))
			if h.OnClientCountChange != nil {
				h.OnClientCountChange(len(h.clients))
			}

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
				slog.Info("ws hub: client unregistered",
					"client_id", c.id, "total_clients", len(h.clients))
				if h.OnClientCountChange != nil {
					h.OnClientCountChange(len(h.clients))
				}
			}

		case p := <-h.ingest:
			// Update the snapshot for this endpoint (O(1) map write).
			h.mu.Lock()
			h.snapshot[p.EndpointID] = p
			h.mu.Unlock()

		case <-ticker.C:
			// Broadcast the current snapshot to all connected clients.
			frame := h.buildFrame()
			if frame == nil {
				continue // no data yet
			}
			for c := range h.clients {
				select {
				case c.send <- frame:
				default:
					// Client's outbound buffer is full — it is stalled.
					// Unregister and close to prevent the hub from blocking.
					delete(h.clients, c)
					close(c.send)
					slog.Warn("ws hub: stalled client evicted", "client_id", c.id)
					if h.OnClientCountChange != nil {
						h.OnClientCountChange(len(h.clients))
					}
				}
			}
		}
	}
}

// buildFrame serialises the current snapshot into a WSMessage JSON frame.
// Returns nil if the snapshot is empty.
func (h *Hub) buildFrame() []byte {
	h.mu.RLock()
	if len(h.snapshot) == 0 {
		h.mu.RUnlock()
		return nil
	}
	payloads := make([]models.TelemetryPayload, 0, len(h.snapshot))
	for _, p := range h.snapshot {
		payloads = append(payloads, p)
	}
	h.mu.RUnlock()

	msg := WSMessage{
		Type:      "telemetry_snapshot",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Payloads:  payloads,
	}
	b, err := json.Marshal(msg)
	if err != nil {
		slog.Error("ws hub: failed to marshal snapshot", "error", err)
		return nil
	}
	return b
}

// ─────────────────────────────────────────────────────────────
// HTTP Handler
// ─────────────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// CheckOrigin allows all origins in development.
	// In production, restrict to your domain.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ServeHTTP upgrades an HTTP connection to WebSocket and registers the
// resulting client with the hub. It is the http.Handler for the /ws path.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws: upgrade failed", "error", err, "remote", r.RemoteAddr)
		return
	}

	c := &client{
		id:   uuid.New().String(),
		conn: conn,
		send: make(chan []byte, outboundBufSize),
		hub:  h,
		cfg:  h.cfg,
	}

	h.register <- c

	// Each client needs exactly two goroutines:
	// writePump — drains h.send → WebSocket.
	// readPump  — detects disconnection / pong (drives deadline reset).
	go c.writePump()
	go c.readPump()
}
