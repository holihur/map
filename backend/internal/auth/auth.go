// Package auth 提供基于 JWT 的鉴权、bcrypt 密码哈希与角色中间件。
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"mapserver/internal/models"
)

// 角色常量。权限由高到低：admin > editor > viewer。
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// ErrUnauthorized 表示凭证缺失或非法。
var ErrUnauthorized = errors.New("unauthorized")

// HashPassword 使用 bcrypt 生成密码哈希。
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword 校验明文密码与哈希是否匹配。
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// Claims 是 JWT 载荷。
type Claims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"sub"`
	Role     string `json:"role"`
	Version  int    `json:"ver"`
	Expires  int64  `json:"exp"`
	IssuedAt int64  `json:"iat"`
}

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// Sign 生成 HS256 签名的 JWT。
func Sign(secret []byte, user *models.User, ttl time.Duration) (string, error) {
	h, _ := json.Marshal(header{Alg: "HS256", Typ: "JWT"})
	now := time.Now()
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		Version:  user.TokenVersion,
		IssuedAt: now.Unix(),
		Expires:  now.Add(ttl).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64(h) + "." + b64(payload)
	sig := sign(secret, signingInput)
	return signingInput + "." + sig, nil
}

// Parse 校验 JWT 并返回载荷。
func Parse(secret []byte, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrUnauthorized
	}
	signingInput := parts[0] + "." + parts[1]
	expected := sign(secret, signingInput)
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return nil, ErrUnauthorized
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrUnauthorized
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrUnauthorized
	}
	if claims.Expires < time.Now().Unix() {
		return nil, fmt.Errorf("%w: token expired", ErrUnauthorized)
	}
	return &claims, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func sign(secret []byte, input string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(input))
	return b64(mac.Sum(nil))
}

// roleRank 返回角色等级，用于比较权限。
func roleRank(role string) int {
	switch role {
	case RoleAdmin:
		return 3
	case RoleEditor:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// Can 判断 role 是否至少具备 required 权限。
func Can(role, required string) bool {
	return roleRank(role) >= roleRank(required)
}

// ── 上下文 ──────────────────────────────────────────────────────

type ctxKey struct{}

// WithUser 将用户写入请求上下文。
func WithUser(ctx context.Context, u *models.User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFrom 从上下文读取用户。
func UserFrom(ctx context.Context) (*models.User, bool) {
	u, ok := ctx.Value(ctxKey{}).(*models.User)
	return u, ok
}
