package config

import (
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port           string
	RedisURL       string
	Targets        []string
	ProbeInterval  time.Duration
	AllowedOrigins []string // extra origins allowed to open /ws, same origin is always allowed
	StaticDir      string   // folder with the dashboard page
	SpikeSigma     float64  // stddevs above normal before a reading counts as a spike
	SpikeMinMs     float64  // and it also has to be at least this many ms slower
}

var defaultTargets = []string{"https://google.com", "https://github.com", "https://silvershinehouse.com"}

// Load reads settings from env vars, anything missing falls back to a local default.
//
//	PORT            8080
//	REDIS_URL       redis://localhost:6379/0
//	TARGETS         comma separated urls
//	PROBE_INTERVAL  10s (minimum 1s)
//	ALLOWED_ORIGINS comma separated, e.g. http://localhost:5173
//	STATIC_DIR      ../frontend (relative to where you run the binary)
//	SPIKE_SIGMA     2.5 (minimum 1), raise it for fewer spikes
//	SPIKE_MIN_MS    10
func Load() Config {
	cfg := Config{
		Port:          getEnv("PORT", "8080"),
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379/0"),
		Targets:       defaultTargets,
		ProbeInterval: 10 * time.Second,
		StaticDir:     getEnv("STATIC_DIR", "../frontend"),
		SpikeSigma:    2.5,
		SpikeMinMs:    10,
	}

	if raw := os.Getenv("TARGETS"); raw != "" {
		var targets []string
		for _, t := range splitList(raw) {
			if u, err := url.Parse(t); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				log.Printf("Skipping invalid target %q", t)
				continue
			}
			targets = append(targets, t)
		}
		if len(targets) > 0 {
			cfg.Targets = targets
		}
	}

	if raw := os.Getenv("PROBE_INTERVAL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < time.Second {
			log.Printf("Ignoring PROBE_INTERVAL=%q, using %s", raw, cfg.ProbeInterval)
		} else {
			cfg.ProbeInterval = d
		}
	}

	cfg.SpikeSigma = getFloat("SPIKE_SIGMA", cfg.SpikeSigma, 1)
	cfg.SpikeMinMs = getFloat("SPIKE_MIN_MS", cfg.SpikeMinMs, 0)

	cfg.AllowedOrigins = splitList(os.Getenv("ALLOWED_ORIGINS"))

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getFloat(key string, fallback, min float64) float64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < min {
		log.Printf("Ignoring %s=%q, using %v", key, raw, fallback)
		return fallback
	}
	return v
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
