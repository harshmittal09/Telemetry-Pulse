package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthySite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != userAgent {
			t.Errorf("missing user agent, got %q", r.Header.Get("User-Agent"))
		}
	}))
	defer srv.Close()

	res := probeOnce(context.Background(), srv.URL)
	if !res.OK || res.StatusCode != 200 || res.Error != "" {
		t.Fatalf("got %+v, want ok 200", res)
	}
}

func TestErrorStatusIsDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	res := probeOnce(context.Background(), srv.URL)
	if res.OK || res.StatusCode != 404 || res.Error != "HTTP 404" {
		t.Fatalf("got %+v, want down with HTTP 404", res)
	}
}

func TestDeadServerIsDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	res := probeOnce(context.Background(), url)
	if res.OK || res.StatusCode != 0 || res.Error != "connection refused" {
		t.Fatalf("got %+v, want down with connection refused", res)
	}
}

func TestWorkerStopsWithContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan ProbeResult)
	StartWorker(ctx, srv.URL, 20*time.Millisecond, out)

	<-out // first probe happens right away
	<-out
	cancel()

	select {
	case <-out:
		// one probe may already have been in flight
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case r := <-out:
		t.Fatalf("worker still running after cancel: %+v", r)
	case <-time.After(100 * time.Millisecond):
	}
}
