package api

import (
	"context"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"mapserver/internal/auth"
	"mapserver/internal/config"
	"mapserver/internal/models"
)

// ── 安全响应头 ──────────────────────────────────────────────────

// securityHeaders 设置常见安全响应头（含适配 MapLibre/OpenLayers 的 CSP）。
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "geolocation=(self), camera=(), microphone=()")
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; "+
				"font-src 'self' data:; "+
				"worker-src 'self' blob:; "+
				"connect-src 'self'; "+
				"object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// ── 限流（令牌桶，按客户端 IP）───────────────────────────────────

type rateBucket struct {
	tokens float64
	last   time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*rateBucket
	lastGC  time.Time
}

func newRateLimiter(rps, burst float64) *rateLimiter {
	if rps <= 0 || burst <= 0 {
		return nil
	}
	return &rateLimiter{rate: rps, burst: burst, buckets: map[string]*rateBucket{}, lastGC: time.Now()}
}

func (rl *rateLimiter) allow(key string) bool {
	if rl == nil {
		return true
	}
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// 定期清理空闲桶
	if now.Sub(rl.lastGC) > time.Minute {
		for k, b := range rl.buckets {
			if now.Sub(b.last) > 10*time.Minute {
				delete(rl.buckets, k)
			}
		}
		rl.lastGC = now
	}

	b := rl.buckets[key]
	if b == nil {
		b = &rateBucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	}
	b.tokens = math.Min(rl.burst, b.tokens+now.Sub(b.last).Seconds()*rl.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// rateLimit 仅对 API 请求限流（静态资源不限）。
func (h *Handlers) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.rate == nil || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		key := clientIP(r)
		if !h.rate.allow(key) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, errString("请求过于频繁，请稍后重试"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ── Token 版本缓存（吊销）───────────────────────────────────────

type versionEntry struct {
	version int
	expires time.Time
}

type tokenVersionCache struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[int64]versionEntry
}

func newTokenVersionCache(ttl time.Duration) *tokenVersionCache {
	return &tokenVersionCache{ttl: ttl, m: map[int64]versionEntry{}}
}

func (c *tokenVersionCache) get(id int64) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[id]
	if !ok || time.Now().After(e.expires) {
		return 0, false
	}
	return e.version, true
}

func (c *tokenVersionCache) set(id int64, v int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[id] = versionEntry{version: v, expires: time.Now().Add(c.ttl)}
}

func (c *tokenVersionCache) invalidate(id int64) {
	c.mu.Lock()
	delete(c.m, id)
	c.mu.Unlock()
}

// userTokenVersion 供鉴权中间件调用：返回用户当前 Token 版本。
func (h *Handlers) userTokenVersion(ctx context.Context, id int64) (int, bool) {
	if v, ok := h.versions.get(id); ok {
		return v, true
	}
	v, err := h.store.TokenVersion(ctx, id)
	if err != nil {
		return 0, false
	}
	h.versions.set(id, v)
	return v, true
}

// ── 认证相关接口 ────────────────────────────────────────────────

// signToken 为指定用户签发 Token。
func signToken(cfg config.Config, user *models.User) (string, error) {
	return auth.Sign([]byte(cfg.JWTSecret), user, cfg.TokenTTL)
}

// refreshToken 用当前有效 Token 换取新 Token（延长有效期）。
func (h *Handlers) refreshToken(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errString("未登录"))
		return
	}
	user, err := h.store.GetUserByUsername(r.Context(), current.Username)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errString("用户不存在"))
		return
	}
	token, err := signToken(h.cfg, user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

// logoutAll 递增 Token 版本，使该用户所有已签发 Token 失效。
func (h *Handlers) logoutAll(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errString("未登录"))
		return
	}
	v, err := h.store.BumpTokenVersion(r.Context(), current.ID)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	h.versions.set(current.ID, v)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "tokenVersion": v})
}
