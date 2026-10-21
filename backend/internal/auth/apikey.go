package auth

import (
	"context"
	"net/http"
	"strings"

	"mapserver/internal/models"
)

// APIKeyIdentity 描述通过 X-API-Key 认证的调用方。
type APIKeyIdentity struct {
	ID        int64
	Role      string
	LayerName string // 非空时仅允许访问该图层
}

type apiKeyCtxKey struct{}

// APIKeyFrom 返回当前请求的 API Key 身份。
func APIKeyFrom(ctx context.Context) (*APIKeyIdentity, bool) {
	v, ok := ctx.Value(apiKeyCtxKey{}).(*APIKeyIdentity)
	return v, ok
}

// WithAPIKey 将 API Key 身份写入上下文。
func WithAPIKey(ctx context.Context, id *APIKeyIdentity) context.Context {
	return context.WithValue(ctx, apiKeyCtxKey{}, id)
}

// APIKeyResolver 根据明文 Key 查找身份。返回的用户仅需 ID/Username/Role。
type APIKeyResolver func(ctx context.Context, key string) (*models.User, *APIKeyIdentity, bool)

// SetAPIKeyResolver 注册 API Key 解析器。
func (a *Authenticator) SetAPIKeyResolver(fn APIKeyResolver) {
	a.apiKeyResolver = fn
}

// apiKeyFromRequest 提取 API Key（优先 Header，其次查询参数）。
func apiKeyFromRequest(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-API-Key")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("api_key")); v != "" {
		return v
	}
	return strings.TrimSpace(r.URL.Query().Get("apiKey"))
}
