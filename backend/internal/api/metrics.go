package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// registerMetrics 注册仪表（采集时求值）与帮助文本。
func (h *Handlers) registerMetrics() {
	h.metrics.Describe("mapserver_http_requests_total", "HTTP 请求数。")
	h.metrics.Describe("mapserver_http_request_duration_seconds_sum", "HTTP 请求耗时（秒）累计。")
	h.metrics.Describe("mapserver_http_request_duration_seconds_count", "HTTP 请求计数（用于求平均耗时）。")
	h.metrics.Describe("mapserver_tile_cache_hits_total", "矢量/栅格瓦片内存缓存命中次数。")
	h.metrics.Describe("mapserver_tile_cache_misses_total", "矢量/栅格瓦片内存缓存未命中次数。")

	h.metrics.RegisterGauge("mapserver_tile_cache_entries", "内存瓦片缓存条目数。", func() float64 {
		_, _, size, _ := h.tiles.Stats()
		return float64(size)
	})
	h.metrics.RegisterGauge("mapserver_tile_cache_hits", "内存瓦片缓存命中总数。", func() float64 {
		hits, _, _, _ := h.tiles.Stats()
		return float64(hits)
	})
	h.metrics.RegisterGauge("mapserver_tile_cache_misses", "内存瓦片缓存未命中总数。", func() float64 {
		_, misses, _, _ := h.tiles.Stats()
		return float64(misses)
	})
	h.metrics.RegisterGauge("mapserver_disk_cache_files", "磁盘缓存文件数。", func() float64 {
		files, _ := h.disk.Stats()
		return float64(files)
	})
	h.metrics.RegisterGauge("mapserver_disk_cache_bytes", "磁盘缓存占用字节数。", func() float64 {
		_, bytes := h.disk.Stats()
		return float64(bytes)
	})
	h.metrics.RegisterGauge("mapserver_disk_cache_limit_bytes", "磁盘缓存容量上限（字节，0 为不限）。", func() float64 {
		return float64(h.cfg.TileCacheMaxMB) * 1024 * 1024
	})
}

// metricsMiddleware 统计请求数、耗时与状态码。
func (h *Handlers) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		h.metrics.Inc("mapserver_http_requests_total", map[string]string{
			"method": r.Method,
			"route":  route,
			"status": strconv.Itoa(status),
		})
		h.metrics.Add("mapserver_http_request_duration_seconds_sum", map[string]string{"route": route}, time.Since(start).Seconds())
		h.metrics.Inc("mapserver_http_request_duration_seconds_count", map[string]string{"route": route})
	})
}

// metricsHandler 输出 Prometheus 文本格式指标。
func (h *Handlers) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(h.metrics.Render()))
}
