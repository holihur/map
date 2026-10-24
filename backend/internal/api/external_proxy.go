package api

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"mapserver/internal/models"
)

var externalHTTPClient = &http.Client{Timeout: 25 * time.Second}

var wmsBBoxRe = regexp.MustCompile(`(?i)(BBOX=)[^&]*`)

// externalTile 代理第三方瓦片，顺带解决浏览器 CORS 限制。
// 仅使用图层已保存的 sourceUrl，不接收任意 URL，避免 SSRF。
func (h *Handlers) externalTile(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	layer, err := h.store.GetExternalLayerByName(r.Context(), name)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	if !h.externalReadAllowed(r.Context(), layer) {
		writeError(w, http.StatusForbidden, errString("没有该图层的访问权限"))
		return
	}
	z, err1 := strconv.Atoi(chi.URLParam(r, "z"))
	x, err2 := strconv.Atoi(chi.URLParam(r, "x"))
	y, err3 := strconv.Atoi(chi.URLParam(r, "y"))
	if err1 != nil || err2 != nil || err3 != nil || z < 0 || z > 30 || x < 0 || y < 0 {
		writeError(w, http.StatusBadRequest, errString("瓦片坐标非法"))
		return
	}
	if int64(x) >= int64(1)<<uint(z) || int64(y) >= int64(1)<<uint(z) {
		writeError(w, http.StatusBadRequest, errString("瓦片坐标越界"))
		return
	}

	key := fmt.Sprintf("ext/%s/%d/%d/%d", layer.Name, z, x, y)
	if data, ok := h.tiles.Get(key); ok {
		w.Header().Set("X-Tile-Cache", "HIT")
		writeProxyBody(w, data, upstreamContentType(layer))
		return
	}

	target, err := buildExternalTileURL(layer, z, x, y)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status, contentType, data, err := fetchExternal(r, target, 32<<20)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if status >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Errorf("上游瓦片返回 %d", status))
		return
	}
	if len(data) > 0 {
		h.tiles.Set(key, data)
	}
	w.Header().Set("X-Tile-Cache", "MISS")
	writeProxyBody(w, data, contentType)
}

// externalData 代理第三方 GeoJSON 等数据地址。
func (h *Handlers) externalData(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	layer, err := h.store.GetExternalLayerByName(r.Context(), name)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	if !h.externalReadAllowed(r.Context(), layer) {
		writeError(w, http.StatusForbidden, errString("没有该图层的访问权限"))
		return
	}
	key := "extdata/" + layer.Name
	if data, ok := h.tiles.Get(key); ok {
		w.Header().Set("X-Tile-Cache", "HIT")
		writeProxyBody(w, data, "application/geo+json")
		return
	}
	status, contentType, data, err := fetchExternal(r, layer.SourceURL, 64<<20)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if status >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Errorf("上游数据返回 %d", status))
		return
	}
	h.tiles.Set(key, data)
	w.Header().Set("X-Tile-Cache", "MISS")
	writeProxyBody(w, data, contentType)
}

func writeProxyBody(w http.ResponseWriter, data []byte, contentType string) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func upstreamContentType(l *models.ExternalLayer) string {
	switch l.SourceType {
	case "mvt":
		return "application/vnd.mapbox-vector-tile"
	case "geojson":
		return "application/geo+json"
	default:
		return "image/png"
	}
}

// buildExternalTileURL 将 z/x/y 与 Web Mercator bbox 代入第三方 URL 模板。
func buildExternalTileURL(l *models.ExternalLayer, z, x, y int) (string, error) {
	raw := strings.TrimSpace(l.SourceURL)
	if raw == "" {
		return "", fmt.Errorf("图层没有配置 sourceUrl")
	}
	target := fillTileTemplate(raw, z, x, y)
	if l.SourceType == "wms" {
		bbox := fillTileTemplate("{minx},{miny},{maxx},{maxy}", z, x, y)
		if wmsBBoxRe.MatchString(target) {
			target = wmsBBoxRe.ReplaceAllString(target, "${1}"+bbox)
		} else {
			sep := "?"
			if strings.Contains(target, "?") {
				sep = "&"
			}
			target += sep + "BBOX=" + url.QueryEscape(bbox)
		}
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("第三方 URL 非法")
	}
	return target, nil
}

func fillTileTemplate(raw string, z, x, y int) string {
	world := 20037508.342789244
	n := math.Exp2(float64(z))
	size := 2 * world / n
	minx := -world + float64(x)*size
	maxy := world - float64(y)*size
	maxx := minx + size
	miny := maxy - size
	bbox := fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", minx, miny, maxx, maxy)
	tmsY := (1 << uint(z)) - 1 - y
	replacer := strings.NewReplacer(
		"{z}", strconv.Itoa(z),
		"{x}", strconv.Itoa(x),
		"{y}", strconv.Itoa(y),
		"{-y}", strconv.Itoa(tmsY),
		"{bbox-epsg-3857}", bbox,
		"{minx}", fmt.Sprintf("%.6f", minx),
		"{miny}", fmt.Sprintf("%.6f", miny),
		"{maxx}", fmt.Sprintf("%.6f", maxx),
		"{maxy}", fmt.Sprintf("%.6f", maxy),
		"{width}", "256",
		"{height}", "256",
	)
	return replacer.Replace(raw)
}

// 内置底图代理：白名单，避免任意 URL 代理。
var builtinBasemapTiles = map[string]string{
	"amap":             "https://webrd01.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}",
	"amap-satellite":   "https://webst01.is.autonavi.com/appmaptile?style=6&x={x}&y={y}&z={z}",
	"google":           "https://mt1.google.com/vt/lyrs=m&x={x}&y={y}&z={z}",
	"google-satellite": "https://mt1.google.com/vt/lyrs=s&x={x}&y={y}&z={z}",
	"google-hybrid":    "https://mt1.google.com/vt/lyrs=y&x={x}&y={y}&z={z}",
}

func (h *Handlers) basemapTile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	raw, ok := builtinBasemapTiles[id]
	if !ok {
		writeError(w, http.StatusNotFound, errString("未知的内置底图"))
		return
	}
	z, err1 := strconv.Atoi(chi.URLParam(r, "z"))
	x, err2 := strconv.Atoi(chi.URLParam(r, "x"))
	y, err3 := strconv.Atoi(chi.URLParam(r, "y"))
	if err1 != nil || err2 != nil || err3 != nil || z < 0 || z > 22 || x < 0 || y < 0 {
		writeError(w, http.StatusBadRequest, errString("瓦片坐标非法"))
		return
	}
	key := fmt.Sprintf("basemap/%s/%d/%d/%d", id, z, x, y)
	if data, ok := h.tiles.Get(key); ok {
		w.Header().Set("X-Tile-Cache", "HIT")
		writeProxyBody(w, data, "image/png")
		return
	}
	target := fillTileTemplate(raw, z, x, y)
	status, contentType, data, err := fetchExternal(r, target, 8<<20)
	if err != nil || status >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Errorf("上游底图请求失败"))
		return
	}
	h.tiles.Set(key, data)
	w.Header().Set("X-Tile-Cache", "MISS")
	writeProxyBody(w, data, contentType)
}

func fetchExternal(r *http.Request, target string, limit int64) (int, string, []byte, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		return 0, "", nil, err
	}
	req.Header.Set("User-Agent", "MapServer/1.0 (third-party layer proxy)")
	req.Header.Set("Accept", "*/*")
	resp, err := externalHTTPClient.Do(req)
	if err != nil {
		return 0, "", nil, fmt.Errorf("请求第三方服务失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return 0, "", nil, err
	}
	if int64(len(data)) > limit {
		return 0, "", nil, fmt.Errorf("第三方响应超过大小限制")
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), data, nil
}
