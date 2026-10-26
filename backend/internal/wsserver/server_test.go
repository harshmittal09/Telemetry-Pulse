package wsserver

import (
	"net/http/httptest"
	"testing"
)

func TestOriginChecker(t *testing.T) {
	check := originChecker([]string{"http://localhost:5173"})

	cases := []struct {
		origin string
		want   bool
	}{
		{"", true},                          // curl, not a browser
		{"https://pulse.example.com", true}, // same host as the request
		{"http://localhost:5173", true},     // explicitly allowed
		{"https://evil.example", false},
	}

	for _, c := range cases {
		r := httptest.NewRequest("GET", "https://pulse.example.com/ws", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := check(r); got != c.want {
			t.Errorf("origin %q: got %v, want %v", c.origin, got, c.want)
		}
	}
}
