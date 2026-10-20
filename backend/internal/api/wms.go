package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"net/http"
	"strconv"
	"strings"

	"mapserver/internal/models"
	"mapserver/internal/sld"
	"mapserver/internal/store"
)

const wmsVersion = "1.3.0"

// wms 是极简 WMS 入口：支持 GetCapabilities / GetMap / GetFeatureInfo。
func (h *Handlers) wms(w http.ResponseWriter, r *http.Request) {
	switch strings.ToUpper(r.URL.Query().Get("REQUEST")) {
	case "GETCAPABILITIES":
		h.wmsCapabilities(w, r)
	case "GETMAP":
		h.wmsGetMap(w, r)
	case "GETFEATUREINFO":
		h.wmsGetFeatureInfo(w, r)
	default:
		wmsException(w, "OperationNotSupported", "仅支持 GetCapabilities / GetMap / GetFeatureInfo")
	}
}

func (h *Handlers) wmsCapabilities(w http.ResponseWriter, r *http.Request) {
	base := requestBaseURL(r)
	layers, err := h.store.ListLayers(r.Context())
	if err != nil {
		wmsException(w, "NoApplicableCode", err.Error())
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<WMS_Capabilities version="%s" xmlns="http://www.opengis.net/wms" xmlns:xlink="http://www.w3.org/1999/xlink">`, wmsVersion)
	fmt.Fprintf(&b, `<Service><Name>WMS</Name><Title>MapServer</Title><Abstract>MapServer 栅格地图服务</Abstract><OnlineResource xlink:href="%s/api/wms"/></Service>`, base)
	fmt.Fprintf(&b, `<Capability><Request>`)
	fmt.Fprintf(&b, `<GetCapabilities><Format>text/xml</Format><DCPType><HTTP><Get><OnlineResource xlink:href="%s/api/wms?"/></Get></HTTP></DCPType></GetCapabilities>`, base)
	fmt.Fprintf(&b, `<GetMap><Format>image/png</Format><Format>image/png; mode=8bit</Format><DCPType><HTTP><Get><OnlineResource xlink:href="%s/api/wms?"/></Get></HTTP></DCPType></GetMap>`, base)
	fmt.Fprintf(&b, `</Request><Exception><Format>XML</Format></Exception>`)

	// 全局支持的坐标系
	fmt.Fprintf(&b, `<Layer><Title>MapServer Layers</Title><CRS>EPSG:4326</CRS><CRS>EPSG:3857</CRS>`)
	for _, l := range layers {
		bounds := l.Bounds
		if bounds == nil {
			if b2, err := h.store.LayerBounds(r.Context(), &l); err == nil && b2 != nil {
				bounds = b2
			}
		}
		title := l.Title
		if title == "" {
			title = l.Name
		}
		var geoBox string
		if bounds != nil {
			geoBox = fmt.Sprintf(
				`<EX_GeographicBoundingBox><westBoundLongitude>%g</westBoundLongitude><eastBoundLongitude>%g</eastBoundLongitude><southBoundLatitude>%g</southBoundLatitude><northBoundLatitude>%g</northBoundLatitude></EX_GeographicBoundingBox>`,
				bounds[0], bounds[2], bounds[1], bounds[3])
		}
		fmt.Fprintf(&b, `<Layer queryable="1"><Name>%s</Name><Title>%s</Title>%s`, xmlEscape(l.Name), xmlEscape(title), geoBox)
		if bounds != nil {
			fmt.Fprintf(&b, `<BoundingBox CRS="EPSG:4326" minx="%g" miny="%g" maxx="%g" maxy="%g"/>`, bounds[0], bounds[1], bounds[2], bounds[3])
			fmt.Fprintf(&b, `<BoundingBox CRS="EPSG:3857" minx="%g" miny="%g" maxx="%g" maxy="%g"/>`,
				lonToMercator(bounds[0]), latToMercator(bounds[1]), lonToMercator(bounds[2]), latToMercator(bounds[3]))
		}
		fmt.Fprintf(&b, `<CRS>EPSG:%d</CRS>`, l.SRID)
		fmt.Fprintf(&b, `<Style><Name>default</Name><Title>默认样式</Title></Style></Layer>`)
	}
	fmt.Fprintf(&b, `</Layer></Capability></WMS_Capabilities>`)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write([]byte(b.String()))
}

func (h *Handlers) wmsGetMap(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	version := q.Get("VERSION")
	if version == "" {
		version = wmsVersion
	}

	layerNames := splitComma(q.Get("LAYERS"))
	if len(layerNames) == 0 {
		wmsException(w, "MissingParameterValue", "缺少 LAYERS 参数")
		return
	}

	srid := parseCRS(firstNonEmpty(q.Get("CRS"), q.Get("SRS")))
	bbox, ok := parseBBox(q.Get("BBOX"))
	if !ok {
		wmsException(w, "InvalidParameterValue", "BBOX 需为 4 个数字")
		return
	}
	// WMS 1.3.0 中 EPSG:4326 的轴顺序为 lat,lon
	if version >= "1.3.0" && srid == 4326 {
		bbox = [4]float64{bbox[1], bbox[0], bbox[3], bbox[2]}
	}
	if bbox[2] <= bbox[0] || bbox[3] <= bbox[1] {
		wmsException(w, "InvalidParameterValue", "BBOX 范围非法")
		return
	}

	width, _ := strconv.Atoi(q.Get("WIDTH"))
	height, _ := strconv.Atoi(q.Get("HEIGHT"))
	if width <= 0 || height <= 0 || width > 4096 || height > 4096 {
		wmsException(w, "InvalidParameterValue", "WIDTH/HEIGHT 必须在 1..4096")
		return
	}

	format := q.Get("FORMAT")
	if format != "" && !strings.Contains(strings.ToLower(format), "png") {
		wmsException(w, "InvalidFormat", "仅支持 image/png")
		return
	}

	transparent := strings.EqualFold(q.Get("TRANSPARENT"), "TRUE")
	bg := color.RGBA{255, 255, 255, 255}
	if transparent {
		bg = color.RGBA{0, 0, 0, 0}
	}
	if c := q.Get("BGCOLOR"); c != "" {
		bg = parseHexColor(strings.TrimPrefix(c, "0x"), 255)
	}

	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	// SLD_BODY：完整解析规则（含比例尺范围与符号样式）。
	var sldDoc *sld.Document
	if body := q.Get("SLD_BODY"); body != "" {
		if doc, err := sld.Parse([]byte(body)); err == nil {
			sldDoc = doc
		} else {
			wmsException(w, "InvalidParameterValue", "SLD_BODY 解析失败: "+err.Error())
			return
		}
	}

	// STYLES=<名称>：加载样式库中的 SLD。
	if sldDoc == nil {
		for _, sn := range splitComma(q.Get("STYLES")) {
			if strings.EqualFold(sn, "default") {
				continue
			}
			st, err := h.store.GetStyleByName(r.Context(), sn)
			if err != nil || st.SLD == "" {
				continue
			}
			if doc, err := sld.Parse([]byte(st.SLD)); err == nil {
				sldDoc = doc
				break
			}
		}
	}

	// 图层默认样式（layers.style_name）缓存
	layerStyles := map[string]*sld.Document{}

	// 比例尺分母（用于 SLD 的 MinScaleDenominator / MaxScaleDenominator）
	metersPerUnit := 1.0
	if srid == 4326 {
		metersPerUnit = 111319.4907932736
	}
	resolution := (bbox[2] - bbox[0]) / float64(width)
	scaleDenominator := resolution * metersPerUnit / 0.00028

	rendered := 0
	for _, name := range layerNames {
		layer, err := h.store.GetLayerByName(r.Context(), name)
		if err != nil {
			wmsException(w, "LayerNotDefined", "图层不存在: "+name)
			return
		}

		rgba := layerColor(layer)
		req := store.RenderRequest{BBox: bbox, OutSRID: srid, Width: width, Height: height}
		doc := sldDoc
		if doc == nil && layer.StyleName != "" {
			if cached, ok := layerStyles[layer.StyleName]; ok {
				doc = cached
			} else if st, err := h.store.GetStyleByName(r.Context(), layer.StyleName); err == nil && st.SLD != "" {
				if parsed, err := sld.Parse([]byte(st.SLD)); err == nil {
					doc = parsed
				}
				layerStyles[layer.StyleName] = doc
			}
		}
		if doc != nil {
			if rule := sld.Select(doc.RulesFor(name), scaleDenominator); rule != nil {
				switch {
				case rule.Fill != nil:
					rgba = color.RGBA{rule.Fill.R, rule.Fill.G, rule.Fill.B, rule.Fill.A}
				case rule.Stroke != nil:
					rgba = color.RGBA{rule.Stroke.R, rule.Stroke.G, rule.Stroke.B, rule.Stroke.A}
				}
				if rule.PointRadius != nil {
					req.PointRadiusPx = *rule.PointRadius
				}
				if rule.StrokeWidth != nil {
					req.LineWidthPx = *rule.StrokeWidth
				}
				// SLD Rule 的 Filter 转译为属性过滤（仅支持可识别的比较表达式）
				if rule.HasFilter && rule.Filter != "" {
					if fs, err := parseOGCFilter(rule.Filter); err == nil {
						req.Filters = fs
					} else {
						h.log.Warn("SLD 规则 Filter 无法转译，已忽略", "layer", name, "err", err)
					}
				}
			}
		}

		mask, err := h.store.RenderMask(r.Context(), layer, req)
		if err != nil {
			wmsException(w, "NoApplicableCode", err.Error())
			return
		}
		if len(mask) == 0 {
			continue
		}
		blitMask(canvas, mask, rgba, width, height)
		rendered++
	}

	var out bytes.Buffer
	if err := png.Encode(&out, canvas); err != nil {
		wmsException(w, "NoApplicableCode", err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Header().Set("X-WMS-Layers", strconv.Itoa(rendered))
	_, _ = w.Write(out.Bytes())
}

// ── 工具函数 ────────────────────────────────────────────────────

// wmsGetFeatureInfo 返回点击位置附近的要素（GeoJSON 或纯文本）。
func (h *Handlers) wmsGetFeatureInfo(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	version := q.Get("VERSION")
	if version == "" {
		version = wmsVersion
	}
	layerNames := splitComma(firstNonEmpty(q.Get("QUERY_LAYERS"), q.Get("LAYERS")))
	if len(layerNames) == 0 {
		wmsException(w, "MissingParameterValue", "缺少 QUERY_LAYERS 参数")
		return
	}
	srid := parseCRS(firstNonEmpty(q.Get("CRS"), q.Get("SRS")))
	bbox, ok := parseBBox(q.Get("BBOX"))
	if !ok {
		wmsException(w, "InvalidParameterValue", "BBOX 需为 4 个数字")
		return
	}
	if version >= "1.3.0" && srid == 4326 {
		bbox = [4]float64{bbox[1], bbox[0], bbox[3], bbox[2]}
	}
	width := atoiDefault(q.Get("WIDTH"), 0)
	height := atoiDefault(q.Get("HEIGHT"), 0)
	i := atoiDefault(firstNonEmpty(q.Get("I"), q.Get("X")), -1)
	j := atoiDefault(firstNonEmpty(q.Get("J"), q.Get("Y")), -1)
	if width <= 0 || height <= 0 || i < 0 || j < 0 {
		wmsException(w, "InvalidParameterValue", "需要合法的 WIDTH/HEIGHT/I/J")
		return
	}

	dx := (bbox[2] - bbox[0]) / float64(width)
	dy := (bbox[3] - bbox[1]) / float64(height)
	x := bbox[0] + (float64(i)+0.5)*dx
	y := bbox[3] - (float64(j)+0.5)*dy
	// 5 像素容差（请求坐标系下的地图单位）
	tol := 5 * math.Max(math.Abs(dx), math.Abs(dy))

	type fcDoc struct {
		Features []json.RawMessage `json:"features"`
	}
	combined := make([]json.RawMessage, 0)
	for _, name := range layerNames {
		layer, err := h.store.GetLayerByName(r.Context(), name)
		if err != nil {
			wmsException(w, "LayerNotDefined", "图层不存在: "+name)
			return
		}
		raw, err := h.store.QueryPoint(r.Context(), layer, x, y, srid, tol, 4326, 20)
		if err != nil {
			wmsException(w, "NoApplicableCode", err.Error())
			return
		}
		var doc fcDoc
		if err := json.Unmarshal(raw, &doc); err == nil {
			combined = append(combined, doc.Features...)
		}
	}

	out := map[string]any{
		"type":     "FeatureCollection",
		"crs":      map[string]any{"type": "name", "properties": map[string]any{"name": "EPSG:4326"}},
		"features": combined,
		"total":    len(combined),
	}
	if strings.Contains(strings.ToLower(q.Get("INFO_FORMAT")), "json") || q.Get("INFO_FORMAT") == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	// text/plain 简化输出
	var sb strings.Builder
	for _, f := range combined {
		sb.Write(f)
		sb.WriteByte('\n')
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(sb.String()))
}

func atoiDefault(v string, def int) int {
	if v == "" {
		return def
	}
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return n
	}
	return def
}

// blitMask 将 0/255 掩膜按给定颜色绘制到 NRGBA 画布（NRGBA 使用非预乘 alpha，颜色保真）。
func blitMask(dst *image.NRGBA, mask [][]float64, c color.RGBA, width, height int) {
	nc := color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A}
	for y := 0; y < len(mask) && y < height; y++ {
		row := mask[y]
		for x := 0; x < len(row) && x < width; x++ {
			if row[x] > 0 {
				dst.SetNRGBA(x, y, nc)
			}
		}
	}
}

func wmsException(w http.ResponseWriter, code, msg string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><ServiceExceptionReport version="%s" xmlns="http://www.opengis.net/ogc"><ServiceException code="%s">%s</ServiceException></ServiceExceptionReport>`,
		wmsVersion, xmlEscape(code), xmlEscape(msg))
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	return scheme + "://" + r.Host
}

func parseCRS(v string) int {
	v = strings.TrimSpace(strings.ToUpper(v))
	v = strings.TrimPrefix(v, "EPSG:")
	switch strings.TrimPrefix(v, "CRS:") {
	case "84", "4326", "CRS84":
		return 4326
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	return 4326
}

func parseBBox(v string) ([4]float64, bool) {
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' })
	if len(parts) != 4 {
		return [4]float64{}, false
	}
	var out [4]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return [4]float64{}, false
		}
		out[i] = f
	}
	return out, true
}

func splitComma(v string) []string {
	out := make([]string, 0)
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

// layerColor 从图层样式中解析填充色，缺省按几何类型取色。
func layerColor(l *models.Layer) color.RGBA {
	var doc struct {
		Type  string         `json:"type"`
		Paint map[string]any `json:"paint"`
	}
	_ = json.Unmarshal(l.Style, &doc)

	key := "fill-color"
	fallback := color.RGBA{59, 130, 246, 200}
	switch doc.Type {
	case "circle":
		key, fallback = "circle-color", color.RGBA{249, 115, 22, 230}
	case "line":
		key, fallback = "line-color", color.RGBA{37, 99, 235, 235}
	case "fill":
		key, fallback = "fill-color", color.RGBA{99, 102, 241, 170}
	default:
		gt := strings.ToUpper(l.GeometryType)
		switch {
		case strings.Contains(gt, "POINT"):
			fallback = color.RGBA{249, 115, 22, 230}
		case strings.Contains(gt, "LINE"):
			fallback = color.RGBA{37, 99, 235, 235}
		}
	}

	alpha := fallback.A
	switch doc.Type {
	case "fill":
		if v, ok := doc.Paint["fill-opacity"].(float64); ok {
			alpha = uint8(clampFloat(v, 0, 1) * 255)
		}
	}
	if v, ok := doc.Paint[key].(string); ok && strings.HasPrefix(v, "#") {
		return parseHexColor(strings.TrimPrefix(v, "#"), alpha)
	}
	return fallback
}

func parseHexColor(hex string, alpha uint8) color.RGBA {
	hex = strings.TrimPrefix(hex, "#")
	switch len(hex) {
	case 3:
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	case 6:
	default:
		return color.RGBA{128, 128, 128, alpha}
	}
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return color.RGBA{128, 128, 128, alpha}
	}
	return color.RGBA{uint8(n >> 16), uint8(n >> 8 & 0xff), uint8(n & 0xff), alpha}
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

const earthRadius = 6378137.0

func lonToMercator(lon float64) float64 {
	return lon * math.Pi / 180 * earthRadius
}

func latToMercator(lat float64) float64 {
	if lat > 89.9 {
		lat = 89.9
	}
	if lat < -89.9 {
		lat = -89.9
	}
	phi := lat * math.Pi / 180
	return earthRadius * math.Log(math.Tan(math.Pi/4+phi/2))
}
