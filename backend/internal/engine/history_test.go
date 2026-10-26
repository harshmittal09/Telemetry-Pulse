package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHistoryKeepsOnlyTheLatest(t *testing.T) {
	h := NewHistory([]string{"a"}, 3, time.Second)
	for i := int64(1); i <= 5; i++ {
		h.Add(Reading{URL: "a", Timestamp: i})
	}

	got := h.snapshot().Targets[0].Readings
	if len(got) != 3 || got[0].Timestamp != 3 || got[2].Timestamp != 5 {
		t.Fatalf("got %+v, want timestamps 3,4,5", got)
	}
}

func TestHistoryEndpoint(t *testing.T) {
	h := NewHistory([]string{"a", "b"}, 10, 10*time.Second)
	h.Add(Reading{URL: "b", LatencyMs: 42, OK: true, Status: 200})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/history", nil))

	var body historyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.IntervalMs != 10000 || len(body.Targets) != 2 {
		t.Fatalf("got %+v", body)
	}
	// targets keep their configured order, even ones with no readings yet
	if body.Targets[0].URL != "a" || len(body.Targets[0].Readings) != 0 {
		t.Fatalf("first target = %+v", body.Targets[0])
	}
	if body.Targets[1].Readings[0].LatencyMs != 42 {
		t.Fatalf("second target = %+v", body.Targets[1])
	}
}
