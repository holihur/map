package metrics

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	r := New()
	r.Describe("http_requests_total", "Total HTTP requests")
	r.Inc("http_requests_total", map[string]string{"method": "GET", "status": "200"})
	r.Inc("http_requests_total", map[string]string{"method": "GET", "status": "200"})
	r.Inc("http_requests_total", map[string]string{"method": "POST", "status": "201"})
	r.SetGauge("mapserver_tile_cache_entries", "cache entries", 3)

	out := r.Render()
	if !strings.Contains(out, "# TYPE http_requests_total counter") {
		t.Fatalf("missing TYPE line:\n%s", out)
	}
	if !strings.Contains(out, `http_requests_total{method="GET",status="200"} 2`) {
		t.Fatalf("missing counter value:\n%s", out)
	}
	if !strings.Contains(out, "mapserver_tile_cache_entries 3") {
		t.Fatalf("missing gauge:\n%s", out)
	}
}
