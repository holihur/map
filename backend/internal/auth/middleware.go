package auth

import (
	"context"
	"net/http"
	"strings"

	"mapserver/internal/models"
)

// Authenticator 提供鉴权中间件。
type Authenticator struct {
	secret []byte
	// versionCheck 可选：校验 Token 中的版本号是否仍然有效（用于吊销）。
	// 返回 (版本, 是否已知)；未知时不做校验。
	versionCheck func(ctx context.Context, userID int64) (int, bool)
	// apiKeyResolver 可选：将 X-API-Key 解析为调用方身份。
	apiKeyResolver APIKeyResolver
}

// NewAuthenticator 构造 Authenticator。
func NewAuthenticator(secret []byte) *Authenticator {
	return &Authenticator{secret: secret}
}

// SetVersionCheck 注册 Token 版本校验函数（用于支持吊销）。
func (a *Authenticator) SetVersionCheck(fn func(ctx context.Context, userID int64) (int, bool)) {
	a.versionCheck = fn
}

// Middleware 解析 Bearer Token（可选），将用户写入上下文。
// 无 token 或 token 非法时不会中断请求，由 RequireAuth 决定是否拒绝。
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API Key 优先于 JWT：适用于第三方只读消费。
		if key := apiKeyFromRequest(r); key != "" && a.apiKeyResolver != nil {
			if user, ident, ok := a.apiKeyResolver(r.Context(), key); ok {
				r = r.WithContext(WithUser(r.Context(), user))
				r = r.WithContext(WithAPIKey(r.Context(), ident))
				next.ServeHTTP(w, r)
				return
			}
		}
		token := bearerToken(r)
		if token != "" {
			if claims, err := Parse(a.secret, token); err == nil {
				valid := true
				if a.versionCheck != nil {
					if ver, ok := a.versionCheck(r.Context(), claims.UserID); ok && ver != claims.Version {
						valid = false
					}
				}
				if valid {
					user := &models.User{
						ID:           claims.UserID,
						Username:     claims.Username,
						Role:         claims.Role,
						TokenVersion: claims.Version,
					}
					r = r.WithContext(WithUser(r.Context(), user))
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuth 拒绝未认证请求。
func (a *Authenticator) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFrom(r.Context()); !ok {
			writeUnauthorized(w, "需要登录")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole 要求当前用户至少具备指定角色。
func (a *Authenticator) RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := UserFrom(r.Context())
			if !ok {
				writeUnauthorized(w, "需要登录")
				return
			}
			if !Can(user.Role, role) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"权限不足"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

func writeUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}
