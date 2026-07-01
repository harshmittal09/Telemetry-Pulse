// Package config holds all runtime configuration for TelemetryPulse.
// Values are read from environment variables with sane defaults, making
// the service 12-factor compliant without requiring a config file.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config is the top-level configuration object passed to all subsystems.
type Config struct {
	Redis     RedisConfig
	WebSocket WebSocketConfig
	Probes    ProbeConfig
}

type RedisConfig struct {
	// URL is the full Redis connection string, e.g. "redis://localhost:6379/0".
	URL string
	// PubSubChannel is the Redis channel name for telemetry broadcasts.
	PubSubChannel string
	// SimChannelPrefix allows per-endpoint channels: <prefix>:<endpointID>
	// Currently we use a single fan-out channel for simplicity.
	SimChannelPrefix string
}

type WebSocketConfig struct {
	// ListenAddr is the HTTP server listen address, e.g. ":8080".
	ListenAddr string
	// Path is the WebSocket upgrade path, e.g. "/ws".
	Path string
	// BroadcastInterval is how often the hub broadcasts to all WS clients.
	// Spec mandates 500ms.
	BroadcastInterval time.Duration
	// WriteTimeout is the per-message write deadline.
	WriteTimeout time.Duration
	// PongTimeout is the inactivity deadline before a client is dropped.
	PongTimeout time.Duration
	// PingInterval is how often the server sends a WebSocket ping frame.
	PingInterval time.Duration
	// MaxMessageSize limits incoming message sizes (e.g. simulation commands).
	MaxMessageSize int64
}

type ProbeConfig struct {
	// IntervalMs is the probe ticker interval in milliseconds per endpoint.
	IntervalMs int
}

// Load reads from environment variables and returns a Config.
// All values fall back to production-safe defaults if unset.
func Load() Config {
	return Config{
		Redis: RedisConfig{
			URL:              getEnv("REDIS_URL", "redis://localhost:6379"),
			PubSubChannel:    getEnv("REDIS_CHANNEL", "telemetrypulse:telemetry"),
			SimChannelPrefix: getEnv("REDIS_SIM_PREFIX", "telemetrypulse:sim"),
		},
		WebSocket: WebSocketConfig{
			ListenAddr:        ":" + getEnv("PORT", "8080"),
			Path:              getEnv("WS_PATH", "/ws"),
			BroadcastInterval: time.Duration(getEnvInt("WS_BROADCAST_INTERVAL_MS", 500)) * time.Millisecond,
			WriteTimeout:      10 * time.Second,
			PongTimeout:       60 * time.Second,
			PingInterval:      54 * time.Second,
			MaxMessageSize:    512,
		},
		Probes: ProbeConfig{
			IntervalMs: getEnvInt("PROBE_INTERVAL_MS", 500),
		},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
