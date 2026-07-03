// lambda/watchman/main.go — TelemetryPulse Lambda Watchman
//
// This AWS Lambda function acts as an external uptime cron watchdog.
// It is triggered on a schedule (e.g., every 1 minute via EventBridge)
// and pings the TelemetryPulse /health endpoint.
//
// Behaviour:
//   - Issues an HTTP GET to the configured TARGET_URL (/health).
//   - Verifies the response is HTTP 200 within a 5-second timeout.
//   - Parses the JSON body to confirm {"status":"ok"}.
//   - Emits a structured CloudWatch log entry on every invocation.
//   - Returns a non-nil error to Lambda on failure — this triggers
//     CloudWatch Alarms if the function error rate exceeds threshold.
//
// Configuration (Lambda environment variables):
//
//   TARGET_URL          Full URL of the /health endpoint
//                       e.g. "https://telemetrypulse.onrender.com/health"
//   HTTP_TIMEOUT_SECS   Timeout in seconds (default: 5)
//   ALERT_ON_STATUS     HTTP status code to treat as failure (default: any non-200)
//
// EventBridge schedule example (rate expression):
//   rate(1 minute)
//
// Deployment:
//   GOOS=linux GOARCH=amd64 go build -o bootstrap ./lambda/watchman
//   zip watchman.zip bootstrap
//   aws lambda update-function-code --function-name telemetrypulse-watchman \
//       --zip-file fileb://watchman.zip

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
)

// ── Types ─────────────────────────────────────────────────────────────────────

// HealthResponse is the expected JSON body from TelemetryPulse's /health endpoint.
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// WatchmanResult is returned by the Lambda handler and logged to CloudWatch.
type WatchmanResult struct {
	Success        bool    `json:"success"`
	TargetURL      string  `json:"target_url"`
	HTTPStatus     int     `json:"http_status"`
	ServiceStatus  string  `json:"service_status"`
	LatencyMs      float64 `json:"latency_ms"`
	InvokedAt      string  `json:"invoked_at"`
	Error          string  `json:"error,omitempty"`
}

// ── Config ────────────────────────────────────────────────────────────────────

type config struct {
	targetURL      string
	httpTimeoutSec int
}

func loadConfig() (config, error) {
	url := os.Getenv("TARGET_URL")
	if url == "" {
		return config{}, fmt.Errorf("watchman: TARGET_URL environment variable not set")
	}

	timeoutSec := 5
	if v := os.Getenv("HTTP_TIMEOUT_SECS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSec = n
		}
	}

	return config{targetURL: url, httpTimeoutSec: timeoutSec}, nil
}

// ── Handler ───────────────────────────────────────────────────────────────────

// handler is the Lambda entry point. It is invoked by EventBridge on schedule.
// The input event payload is ignored — only the schedule trigger matters.
func handler(ctx context.Context, _ json.RawMessage) (WatchmanResult, error) {
	invokedAt := time.Now().UTC()

	// ── Load configuration ────────────────────────────────────────────────
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("Watchman config error", "error", err)
		return WatchmanResult{
			Success:   false,
			InvokedAt: invokedAt.Format(time.RFC3339),
			Error:     err.Error(),
		}, err
	}

	result := WatchmanResult{
		TargetURL: cfg.targetURL,
		InvokedAt: invokedAt.Format(time.RFC3339),
	}

	// ── Execute health check ──────────────────────────────────────────────
	client := &http.Client{
		Timeout: time.Duration(cfg.httpTimeoutSec) * time.Second,
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.httpTimeoutSec)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, cfg.targetURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("build request: %s", err)
		slog.Error("Watchman request build failed", "error", err, "url", cfg.targetURL)
		return result, err
	}
	req.Header.Set("User-Agent", "TelemetryPulse-Watchman/1.0")

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Seconds() * 1000

	result.LatencyMs = latency

	if err != nil {
		result.Error = fmt.Sprintf("HTTP request failed: %s", err)
		slog.Error("Watchman HTTP error",
			"url", cfg.targetURL,
			"latency_ms", latency,
			"error", err,
		)
		return result, err
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode

	// ── Parse response body ────────────────────────────────────────────────
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var healthResp HealthResponse
	if jsonErr := json.Unmarshal(body, &healthResp); jsonErr == nil {
		result.ServiceStatus = healthResp.Status
	} else {
		result.ServiceStatus = "(unparseable)"
	}

	// ── Assert HTTP 200 + status=ok ────────────────────────────────────────
	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("unexpected HTTP %d", resp.StatusCode)
		slog.Error("Watchman health check FAILED",
			"url", cfg.targetURL,
			"http_status", resp.StatusCode,
			"body", string(body),
			"latency_ms", latency,
		)
		return result, fmt.Errorf("health check failed: HTTP %d", resp.StatusCode)
	}

	if healthResp.Status != "ok" {
		result.Error = fmt.Sprintf("unexpected service status: %q", healthResp.Status)
		slog.Error("Watchman service status DEGRADED",
			"url", cfg.targetURL,
			"service_status", healthResp.Status,
			"latency_ms", latency,
		)
		return result, fmt.Errorf("service status degraded: %q", healthResp.Status)
	}

	// ── Success ────────────────────────────────────────────────────────────
	result.Success = true

	slog.Info("Watchman health check OK",
		"url", cfg.targetURL,
		"http_status", resp.StatusCode,
		"service_status", healthResp.Status,
		"latency_ms", latency,
	)

	return result, nil
}

// ── Entrypoint ────────────────────────────────────────────────────────────────

func main() {
	// Configure structured JSON logging — CloudWatch Insights can query these fields.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("TelemetryPulse Watchman Lambda starting")

	// lambda.Start blocks until the Lambda runtime terminates this instance.
	lambda.Start(handler)
}
