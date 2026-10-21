package api

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"mapserver/internal/models"
	"mapserver/internal/store"
	"mapserver/internal/tiles"
)

// vectorTile 生成并缓存矢量瓦片（内存 + 磁盘）。
func (h *Handlers) vectorTile(ctx context.Context, layer *models.Layer, grid tiles.Grid, z, x, y, extent int) ([]byte, bool, error) {
	key := layer.Name + "/" + grid.ID + "/" + strconv.Itoa(z) + "/" +
		strconv.Itoa(x) + "/" + strconv.Itoa(y) + "/" + strconv.Itoa(extent) + ".mvt"

	if data, ok := h.tiles.Get(key); ok {
		return data, true, nil
	}
	if data, ok := h.disk.Get(key); ok {
		h.tiles.Set(key, data)
		return data, true, nil
	}

	env := grid.Envelope(z, x, y)
	data, err := h.store.TileMVT(ctx, layer, extent, grid.SRID, z, h.cfg.TileMaxFeatures, h.cfg.TileSimplify, env)
	if err != nil {
		return nil, false, err
	}
	if len(data) > 0 {
		h.tiles.Set(key, data)
		_ = h.disk.Set(key, data)
	}
	return data, false, nil
}

// tileMVT 返回 Mapbox Vector Tile（带内存 LRU 缓存）。
// 可选路径参数 {tms} 指定瓦片矩阵集，默认 WebMercatorQuad。
func (h *Handlers) tileMVT(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}

	grid := tiles.Default()
	if id := chi.URLParam(r, "tms"); id != "" {
		g, ok := tiles.Get(id)
		if !ok {
			writeError(w, http.StatusBadRequest, errString("未知的瓦片矩阵集 "+id))
			return
		}
		grid = g
	}

	z, err1 := strconv.Atoi(chi.URLParam(r, "z"))
	x, err2 := strconv.Atoi(chi.URLParam(r, "x"))
	y, err3 := strconv.Atoi(chi.URLParam(r, "y"))
	if err1 != nil || err2 != nil || err3 != nil {
		writeError(w, http.StatusBadRequest, errString("瓦片坐标非法"))
		return
	}
	if z > h.cfg.TileMaxZoom || !grid.Contains(z, x, y) {
		writeError(w, http.StatusBadRequest, errString("瓦片坐标越界"))
		return
	}

	extent := 4096
	if v := r.URL.Query().Get("extent"); v != "" {
		if e, err := strconv.Atoi(v); err == nil && e > 0 {
			extent = e
		}
	}

	data, hit, err := h.vectorTile(r.Context(), layer, grid, z, x, y, extent)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if hit {
		w.Header().Set("X-Tile-Cache", "HIT")
	} else {
		w.Header().Set("X-Tile-Cache", "MISS")
	}
	serveTile(w, data)
}

func serveTile(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/vnd.mapbox-vector-tile")
	w.Header().Set("Content-Encoding", "identity")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if len(data) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// rasterTile 渲染并缓存栅格 PNG 瓦片（内存 + 磁盘）。
func (h *Handlers) rasterTile(ctx context.Context, layer *models.Layer, grid tiles.Grid, z, x, y int) ([]byte, bool, error) {
	key := layer.Name + "/" + grid.ID + "/" + strconv.Itoa(z) + "/" +
		strconv.Itoa(x) + "/" + strconv.Itoa(y) + ".png"

	if data, ok := h.tiles.Get(key); ok {
		return data, true, nil
	}
	if data, ok := h.disk.Get(key); ok {
		h.tiles.Set(key, data)
		return data, true, nil
	}

	size := grid.TileSize
	env := grid.Envelope(z, x, y)
	mask, err := h.store.RenderMask(ctx, layer, store.RenderRequest{
		BBox:    env,
		OutSRID: grid.SRID,
		Width:   size,
		Height:  size,
	})
	if err != nil {
		return nil, false, err
	}

	canvas := image.NewNRGBA(image.Rect(0, 0, size, size))
	blitMask(canvas, mask, layerColor(layer), size, size)

	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		return nil, false, err
	}
	h.tiles.Set(key, buf.Bytes())
	_ = h.disk.Set(key, buf.Bytes())
	return buf.Bytes(), false, nil
}

// tilePNG 返回栅格瓦片（PNG，透明背景），供 XYZ / OpenLayers / WMTS 使用。
func (h *Handlers) tilePNG(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}

	grid := tiles.Default()
	if id := chi.URLParam(r, "tms"); id != "" {
		g, ok := tiles.Get(id)
		if !ok {
			writeError(w, http.StatusBadRequest, errString("未知的瓦片矩阵集 "+id))
			return
		}
		grid = g
	}

	z, err1 := strconv.Atoi(chi.URLParam(r, "z"))
	x, err2 := strconv.Atoi(chi.URLParam(r, "x"))
	y, err3 := strconv.Atoi(chi.URLParam(r, "y"))
	if err1 != nil || err2 != nil || err3 != nil {
		writeError(w, http.StatusBadRequest, errString("瓦片坐标非法"))
		return
	}
	if z > h.cfg.TileMaxZoom || !grid.Contains(z, x, y) {
		writeError(w, http.StatusBadRequest, errString("瓦片坐标越界"))
		return
	}

	data, hit, err := h.rasterTile(r.Context(), layer, grid, z, x, y)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if hit {
		w.Header().Set("X-Tile-Cache", "HIT")
	} else {
		w.Header().Set("X-Tile-Cache", "MISS")
	}
	servePNG(w, data)
}

func servePNG(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// listTileMatrixSets 返回支持的瓦片矩阵集及其 URL 模板。
func (h *Handlers) listTileMatrixSets(w http.ResponseWriter, r *http.Request) {
	type tmsInfo struct {
		ID          string     `json:"id"`
		CRS         string     `json:"crs"`
		SRID        int        `json:"srid"`
		TileSize    int        `json:"tileSize"`
		Bounds      [4]float64 `json:"bounds"`
		Scheme      string     `json:"scheme"`
		TopLeft     bool       `json:"topLeftOrigin"`
		TileURL     string     `json:"tileUrl"`
		MatrixLevel int        `json:"maxZoom"`
	}
	list := make([]tmsInfo, 0)
	for _, g := range tiles.All() {
		list = append(list, tmsInfo{
			ID:       g.ID,
			CRS:      g.CRS,
			SRID:     g.SRID,
			TileSize: g.TileSize,
			Bounds:   g.Bounds,
			Scheme:   g.Scheme,
			TopLeft:  true,
			TileURL:  "/api/tiles/{layer}/" + g.ID + "/{z}/{x}/{y}.mvt",
			// 避免超过瓦片矩阵范围
			MatrixLevel: minInt(30, h.cfg.TileMaxZoom),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tileMatrixSets": list})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
