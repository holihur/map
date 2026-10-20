// Package config 从环境变量加载服务配置。
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 保存服务运行所需配置。
type Config struct {
	Addr        string
	DatabaseURL string
	CORSOrigins []string
	MaxFeatures int // 单次查询返回的最大要素数
	TileMaxZoom int

	// 鉴权
	JWTSecret     string
	TokenTTL      time.Duration
	AdminUsername string
	AdminPassword string

	// 限流（每秒请求数与突发量，0 为禁用）
	RateLimitRPS   float64
	RateLimitBurst float64

	// 运行环境：production 时禁止使用默认密钥
	AppEnv string

	// 缓存
	TileCacheEntries int
	TileCacheTTL     time.Duration
	MetaCacheTTL     time.Duration
	TileCacheDir     string
	TileDiskTTL      time.Duration
	TileCacheMaxMB   int

	// 前端静态资源目录（为空时自动探测，优先使用内嵌资源）
	StaticDir string
}

// Load 读取环境变量并返回配置，未设置时使用合理默认值。
func Load() Config {
	loadDotEnv()
	return Config{
		Addr:        env("HTTP_ADDR", ":8420"),
		DatabaseURL: env("DATABASE_URL", "postgres://map:map@localhost:5432/mapserver?sslmode=disable"),
		CORSOrigins: split(env("CORS_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		MaxFeatures: envInt("MAX_FEATURES", 100000),
		TileMaxZoom: envInt("TILE_MAX_ZOOM", 22),

		JWTSecret:      env("JWT_SECRET", "dev-secret-change-me"),
		TokenTTL:       time.Duration(envInt("TOKEN_TTL_HOURS", 72)) * time.Hour,
		AdminUsername:  env("ADMIN_USERNAME", "admin"),
		AdminPassword:  env("ADMIN_PASSWORD", "admin123"),
		AppEnv:         env("APP_ENV", "development"),
		RateLimitRPS:   envFloat("RATE_LIMIT_RPS", 30),
		RateLimitBurst: envFloat("RATE_LIMIT_BURST", 60),

		TileCacheEntries: envInt("TILE_CACHE_ENTRIES", 2048),
		TileCacheTTL:     time.Duration(envInt("TILE_CACHE_TTL_SECONDS", 300)) * time.Second,
		MetaCacheTTL:     time.Duration(envInt("META_CACHE_TTL_SECONDS", 30)) * time.Second,
		TileCacheDir:     env("TILE_CACHE_DIR", "./data/tile-cache"),
		TileDiskTTL:      time.Duration(envInt("TILE_DISK_TTL_SECONDS", 3600)) * time.Second,
		TileCacheMaxMB:   envInt("TILE_CACHE_MAX_MB", 256),

		StaticDir: env("STATIC_DIR", ""),
	}
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// IsProduction 表示是否以生产模式运行。
func (c Config) IsProduction() bool {
	return strings.EqualFold(c.AppEnv, "production")
}

// UsesDefaultSecret 表示 JWT 密钥仍是默认值。
func (c Config) UsesDefaultSecret() bool {
	return c.JWTSecret == "" || c.JWTSecret == "dev-secret-change-me"
}

// envFloat 读取浮点环境变量。
func envFloat(key string, def float64) float64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func split(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// loadDotEnv 从常见位置的 .env 文件加载配置（不覆盖已存在的环境变量）。
// 仅实现 KEY=VALUE 与 # 注释的简单语法，避免额外依赖。
func loadDotEnv() {
	paths := []string{".env", "backend/.env", "../.env"}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, val, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			val = strings.Trim(strings.TrimSpace(val), `"'`)
			if key != "" && os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
		_ = f.Close()
	}
}
