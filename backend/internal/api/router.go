// Package api 提供 HTTP 路由与处理器。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"mapserver/internal/auth"
	"mapserver/internal/cache"
	"mapserver/internal/config"
	"mapserver/internal/metrics"
	"mapserver/internal/models"
	"mapserver/internal/store"
	"mapserver/internal/web"
)

// Handlers 持有依赖。
type Handlers struct {
	store    *store.Store
	cfg      config.Config
	log      *slog.Logger
	auth     *auth.Authenticator
	tiles    *cache.ByteCache
	meta     *cache.ByteCache
	disk     *cache.DiskCache
	web      *web.Handler
	metrics  *metrics.Registry
	rate     *rateLimiter
	versions *tokenVersionCache
}

// NewRouter 构造带全部路由的 http.Handler。
func NewRouter(st *store.Store, cfg config.Config, log *slog.Logger) http.Handler {
	h := &Handlers{
		store:    st,
		cfg:      cfg,
		log:      log,
		auth:     auth.NewAuthenticator([]byte(cfg.JWTSecret)),
		tiles:    cache.NewByteCache(cfg.TileCacheEntries, cfg.TileCacheTTL),
		meta:     cache.NewByteCache(64, cfg.MetaCacheTTL),
		disk:     cache.NewDiskCache(cfg.TileCacheDir, cfg.TileDiskTTL, int64(cfg.TileCacheMaxMB)*1024*1024),
		web:      web.New(cfg.StaticDir),
		metrics:  metrics.New(),
		rate:     newRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst),
		versions: newTokenVersionCache(30 * time.Second),
	}
	h.auth.SetVersionCheck(h.userTokenVersion)
	h.auth.SetAPIKeyResolver(func(ctx context.Context, key string) (*models.User, *auth.APIKeyIdentity, bool) {
		k, err := st.FindAPIKey(ctx, key)
		if err != nil {
			return nil, nil, false
		}
		go st.TouchAPIKey(context.Background(), k.ID)
		return &models.User{Username: "api-key:" + k.Name, Role: k.Role},
			&auth.APIKeyIdentity{ID: k.ID, Role: k.Role, LayerName: k.LayerName}, true
	})
	h.registerMetrics()
	if h.web.Enabled() {
		log.Info("已启用前端静态托管", "source", h.web.Source())
	} else {
		log.Warn("未找到前端构建产物，仅提供 API（请先执行 make frontend-build）")
	}
	if h.disk.Enabled() {
		log.Info("已启用瓦片磁盘缓存", "dir", h.disk.Root(), "maxMB", cfg.TileCacheMaxMB)
		// 定期清理过期缓存文件
		go func() {
			ticker := time.NewTicker(10 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				if n := h.disk.CleanupExpired(); n > 0 {
					log.Info("清理过期瓦片缓存", "removed", n)
				}
			}
		}()
	}

	editorOnly := h.auth.RequireRole(auth.RoleEditor)
	adminOnly := h.auth.RequireRole(auth.RoleAdmin)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "If-None-Match", "X-API-Key", "X-AI-API-Key", "X-AI-Base-URL", "X-AI-Model"},
		ExposedHeaders:   []string{"Content-Length", "ETag"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(h.auth.Middleware)
	r.Use(h.auditMiddleware)
	r.Use(h.rateLimit)
	r.Use(securityHeaders)
	r.Use(h.metricsMiddleware)

	r.Get("/health", h.health)
	r.Get("/healthz", h.health)
	r.Get("/readyz", h.ready)
	r.Get("/metrics", h.metricsHandler)
	r.Get("/api/capabilities", h.capabilities)
	r.Get("/api/stats", h.stats)
	r.Get("/api/crs", h.listCRS)
	r.Get("/api/crs/{srid}", h.getCRS)
	r.Get("/api/tilematrixsets", h.listTileMatrixSets)
	r.Get("/api/export", h.exportAll)
	r.Get("/api/search", h.search)

	// OGC API - Features（现代 REST/JSON，与 WFS 并存）
	r.Get("/ogcapi", h.ogcLanding)
	r.Get("/ogcapi/conformance", h.ogcConformance)
	r.Get("/ogcapi/collections", h.ogcCollections)
	r.Get("/ogcapi/collections/{name}", h.ogcCollection)
	r.Get("/ogcapi/collections/{name}/items", h.ogcItems)
	r.Get("/ogcapi/collections/{name}/items/{fid}", h.ogcItem)
	r.Get("/api/wms", h.wms)
	r.Get("/api/wfs", h.wfs)
	r.With(editorOnly).Post("/api/wfs", h.wfsTransaction)
	r.With(adminOnly).Post("/api/cache/purge", h.purgeCache)

	// 分享地图（GET 公开）
	r.Route("/api/maps", func(r chi.Router) {
		r.With(h.auth.RequireAuth).Get("/", h.listMaps)
		r.With(h.auth.RequireAuth).Post("/", h.createMap)
		r.Get("/{token}", h.getMap)
		r.With(h.auth.RequireAuth).Put("/{token}", h.updateMap)
		r.With(h.auth.RequireAuth).Delete("/{token}", h.deleteMap)
	})

	// API Key / 审计（管理员）
	r.Route("/api/apikeys", func(r chi.Router) {
		r.Use(adminOnly)
		r.Get("/", h.listAPIKeys)
		r.Post("/", h.createAPIKey)
		r.Post("/{id}/revoke", h.revokeAPIKey)
		r.Delete("/{id}", h.deleteAPIKey)
	})
	r.With(adminOnly).Get("/api/audit", h.listAuditLogs)

	// 自然语言地图助手（BYOK，OpenAI 兼容 / ADK 风格工具调用）
	r.With(h.auth.RequireAuth).Post("/api/ai/chat", h.aiChat)
	r.With(h.auth.RequireAuth).Post("/api/ai/confirm", h.aiConfirm)

	// 认证
	r.Post("/api/auth/login", h.login)
	r.With(h.auth.RequireAuth).Get("/api/auth/me", h.me)
	r.With(h.auth.RequireAuth).Post("/api/auth/refresh", h.refreshToken)
	r.With(h.auth.RequireAuth).Post("/api/auth/logout-all", h.logoutAll)

	// 用户管理
	r.Route("/api/users", func(r chi.Router) {
		r.With(editorOnly).Get("/", h.listUsers)
		r.With(adminOnly).Post("/", h.createUser)
		r.With(adminOnly).Put("/{id}", h.updateUser)
		r.With(adminOnly).Delete("/{id}", h.deleteUser)
	})

	r.Route("/api/layers", func(r chi.Router) {
		r.Get("/", h.listLayers)
		r.With(editorOnly).Post("/", h.createLayer)
		r.With(editorOnly).Post("/import", h.importNewLayer)
		r.With(editorOnly).Post("/reorder", h.reorderLayers)
		r.Route("/{layer}", func(r chi.Router) {
			r.Get("/", h.getLayer)
			r.With(editorOnly).Patch("/", h.updateLayer)
			r.With(editorOnly).Put("/", h.updateLayer)
			r.With(adminOnly).Delete("/", h.deleteLayer)
			r.Get("/bounds", h.layerBounds)
			r.Get("/stats", h.fieldStats)
			r.Get("/access", h.getLayerAccess)
			r.With(editorOnly).Put("/access", h.setLayerAccess)
			r.With(editorOnly).Post("/transform", h.transformLayer)
			r.Get("/export", h.exportLayer)
			r.With(editorOnly).Post("/import", h.importLayer)
			r.Get("/features", h.listFeatures)
			r.With(editorOnly).Post("/features", h.createFeatures)
			r.With(editorOnly).Post("/features/batch", h.batchFeatures)
			r.Post("/analyze", h.analyzeFeatures)
			r.Get("/features/{fid}", h.getFeature)
			r.With(editorOnly).Patch("/features/{fid}", h.updateFeature)
			r.With(editorOnly).Put("/features/{fid}", h.updateFeature)
			r.With(editorOnly).Delete("/features/{fid}", h.deleteFeature)
		})
	})

	// 第三方图层（XYZ / WMS / WMTS / MVT / GeoJSON / ArcGIS）
	r.Route("/api/external-layers", func(r chi.Router) {
		r.Get("/", h.listExternalLayers)
		r.With(editorOnly).Post("/", h.createExternalLayer)
		r.Route("/{name}", func(r chi.Router) {
			r.Get("/", h.getExternalLayer)
			r.Get("/tile/{z}/{x}/{y}", h.externalTile)
			r.Get("/data", h.externalData)
			r.With(editorOnly).Patch("/", h.updateExternalLayer)
			r.With(editorOnly).Put("/", h.updateExternalLayer)
			r.With(editorOnly).Delete("/", h.deleteExternalLayer)
		})
	})

	// 内置第三方底图代理（高德 / Google，避免 CORS）
	r.Get("/api/basemaps/{id}/tile/{z}/{x}/{y}", h.basemapTile)

	r.Get("/api/tiles/{layer}/{z}/{x}/{y}.mvt", h.tileMVT)
	r.Get("/api/tiles/{layer}/{z}/{x}/{y}.pbf", h.tileMVT)
	r.Get("/api/tiles/{layer}/{z}/{x}/{y}.png", h.tilePNG)
	r.Get("/api/tiles/{layer}/{tms}/{z}/{x}/{y}.mvt", h.tileMVT)
	r.Get("/api/tiles/{layer}/{tms}/{z}/{x}/{y}.pbf", h.tileMVT)
	r.Get("/api/tiles/{layer}/{tms}/{z}/{x}/{y}.png", h.tilePNG)

	r.Get("/api/wmts", h.wmtsKVP)
	r.Get("/api/wmts/rest/{layer}/{style}/{tms}/{z}/{y}/{x}.{ext}", h.wmtsREST)
	r.Route("/api/styles", func(r chi.Router) {
		r.Get("/", h.listStyles)
		r.With(editorOnly).Post("/", h.createStyle)
		r.With(editorOnly).Post("/sld", h.uploadSLD)
		r.Get("/{name}", h.getStyle)
		r.With(editorOnly).Put("/{id}", h.updateStyle)
		r.With(editorOnly).Delete("/{id}", h.deleteStyle)
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		// 非 API 的 GET/HEAD 请求交给前端（SPA 路由回退）。
		if h.web.Enabled() && (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
			!strings.HasPrefix(r.URL.Path, "/api/") {
			h.web.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusNotFound, errors.New("route not found"))
	})
	return r
}

func (h *Handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "mapserver"})
}

// ready 就绪探针：检查数据库连接。
func (h *Handlers) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.store.Pool().Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "service": "mapserver"})
}

// invalidateLayer 使指定图层的瓦片缓存（内存 + 磁盘）与全部元数据缓存失效。
func (h *Handlers) invalidateLayer(name string) {
	h.tiles.InvalidatePrefix(name + "/")
	_ = h.disk.RemovePrefix(name + "/")
	h.meta.Purge()
}

// invalidateMeta 仅使元数据缓存失效。
func (h *Handlers) invalidateMeta() { h.meta.Purge() }

// ── 通用响应工具 ────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	return dec.Decode(v)
}

// statusForError 将 store 错误映射为 HTTP 状态码。
func statusForError(err error) int {
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
