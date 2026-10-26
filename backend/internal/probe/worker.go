package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

const (
	timeout   = 5 * time.Second
	userAgent = "Telemetry-Pulse/1.0 (+https://github.com/harshmittal09/Telemetry-Pulse)"
)

type ProbeResult struct {
	URL        string
	Latency    float64 // ms until the response headers arrive (dns + connect + tls + redirects)
	StatusCode int     // 0 if the request never got a response
	OK         bool    // got a response with status < 400
	Error      string  // short reason when OK is false
	Timestamp  int64   // unix ms when the probe started
}

var client = &http.Client{Timeout: timeout}

// StartWorker probes url right away and then once every interval until ctx is cancelled.
func StartWorker(ctx context.Context, url string, interval time.Duration, outChan chan<- ProbeResult) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			result := probeOnce(ctx, url)

			select {
			case outChan <- result:
			case <-ctx.Done():
				return
			}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
}

func probeOnce(ctx context.Context, url string) ProbeResult {
	result := ProbeResult{
		URL:       url,
		Timestamp: time.Now().UnixMilli(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		result.Error = "invalid url"
		return result
	}
	req.Header.Set("User-Agent", userAgent)

	start := time.Now()
	res, err := client.Do(req)
	result.Latency = time.Since(start).Seconds() * 1000

	if err != nil {
		result.Error = describeError(err)
		return result
	}

	// not reading the body on purpose, we only time until the headers arrive and
	// skipping the download keeps bandwidth tiny (each probe uses a fresh connection)
	res.Body.Close()

	result.StatusCode = res.StatusCode
	result.OK = res.StatusCode < 400
	if !result.OK {
		result.Error = fmt.Sprintf("HTTP %d", res.StatusCode)
	}

	return result
}

func describeError(err error) string {
	var dnsErr *net.DNSError
	var netErr net.Error

	switch {
	case errors.As(err, &dnsErr):
		return "dns lookup failed"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	default:
		return "unreachable"
	}
}
