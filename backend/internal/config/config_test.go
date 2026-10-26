package config

import (
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("TARGETS", "")
	t.Setenv("PROBE_INTERVAL", "")

	t.Setenv("STATIC_DIR", "")

	cfg := Load()
	if cfg.Port != "8080" || cfg.ProbeInterval != 10*time.Second || len(cfg.Targets) != 3 || cfg.StaticDir != "../frontend" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("PORT", "10000")
	t.Setenv("REDIS_URL", "redis://example:6380/1")
	t.Setenv("TARGETS", " https://a.com , not a url, ftp://b.com,http://c.com ")
	t.Setenv("PROBE_INTERVAL", "30s")
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:5173")

	cfg := Load()
	if cfg.Port != "10000" || cfg.RedisURL != "redis://example:6380/1" || cfg.ProbeInterval != 30*time.Second {
		t.Fatalf("got %+v", cfg)
	}
	if len(cfg.Targets) != 2 || cfg.Targets[0] != "https://a.com" || cfg.Targets[1] != "http://c.com" {
		t.Fatalf("targets = %v", cfg.Targets)
	}
	if len(cfg.AllowedOrigins) != 1 {
		t.Fatalf("origins = %v", cfg.AllowedOrigins)
	}
}

func TestTooShortIntervalIsIgnored(t *testing.T) {
	t.Setenv("PROBE_INTERVAL", "100ms")
	if cfg := Load(); cfg.ProbeInterval != 10*time.Second {
		t.Fatalf("interval = %s", cfg.ProbeInterval)
	}
}
