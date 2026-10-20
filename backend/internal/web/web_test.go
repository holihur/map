package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func newTestHandler() *Handler {
	return &Handler{
		fsys: fstest.MapFS{
			"index.html":    &fstest.MapFile{Data: []byte("<html>app</html>")},
			"assets/app.js": &fstest.MapFile{Data: []byte("console.log(1)")},
		},
		source: "test",
	}
}

func TestServeIndex(t *testing.T) {
	h := newTestHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "app") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("index cache-control = %q", cc)
	}
}

func TestSPAFallback(t *testing.T) {
	h := newTestHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/map", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "app") {
		t.Fatalf("expected index fallback, got %d %q", rec.Code, rec.Body.String())
	}
}

func TestAssetCacheHeader(t *testing.T) {
	h := newTestHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("asset cache-control = %q", cc)
	}
}

func TestMissingAssetAndAPI(t *testing.T) {
	h := newTestHandler()
	for _, p := range []string{"/assets/nope.js", "/api/whatever"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", p, rec.Code)
		}
	}
}

func TestNilHandler(t *testing.T) {
	var h *Handler
	if h.Enabled() {
		t.Fatal("nil handler should not be enabled")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}
