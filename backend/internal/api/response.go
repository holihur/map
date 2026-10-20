package api

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"

	"mapserver/internal/cache"
)

// writeCachedJSON 输出带 ETag 的 JSON；命中条件请求时返回 304。
// build 仅在缓存未命中时调用。
func writeCachedJSON(w http.ResponseWriter, r *http.Request, c *cache.ByteCache, key string, build func() ([]byte, error)) {
	body, ok := c.Get(key)
	if !ok {
		b, err := build()
		if err != nil {
			writeError(w, statusForError(err), err)
			return
		}
		body = b
		c.Set(key, body)
	}

	etag := `"` + hashHex(body) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=15")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func hashHex(b []byte) string {
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}
