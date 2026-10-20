package api

import "net/http"

// stats 输出缓存与限流相关的运行时统计。
func (h *Handlers) stats(w http.ResponseWriter, r *http.Request) {
	tileHits, tileMiss, tileSize, tileEvict := h.tiles.Stats()
	metaHits, metaMiss, metaSize, metaEvict := h.meta.Stats()
	diskFiles, diskBytes := h.disk.Stats()
	writeJSON(w, http.StatusOK, map[string]any{
		"tileCache": map[string]any{
			"hits":    tileHits,
			"misses":  tileMiss,
			"size":    tileSize,
			"evicted": tileEvict,
			"max":     h.cfg.TileCacheEntries,
		},
		"metaCache": map[string]any{
			"hits":    metaHits,
			"misses":  metaMiss,
			"size":    metaSize,
			"evicted": metaEvict,
		},
		"diskCache": map[string]any{
			"enabled":    h.disk.Enabled(),
			"dir":        h.disk.Root(),
			"files":      diskFiles,
			"bytes":      diskBytes,
			"limitBytes": int64(h.cfg.TileCacheMaxMB) * 1024 * 1024,
		},
	})
}

// purgeCache 清空全部缓存（管理员）。
func (h *Handlers) purgeCache(w http.ResponseWriter, r *http.Request) {
	h.tiles.Purge()
	h.meta.Purge()
	_ = h.disk.Purge()
	writeJSON(w, http.StatusOK, map[string]any{"status": "purged"})
}
