package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newFakeLLM(t *testing.T, reply func(call int, body string) string) *httptest.Server {
	t.Helper()
	call := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply(call, string(body))))
	}))
}

func readAll(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func newTileUpstream(t *testing.T, calls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		(*calls)++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("TILE"))
	}))
}
