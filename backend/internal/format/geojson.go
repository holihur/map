// Package format 提供矢量数据的导入导出（GeoJSON / CSV / Shapefile / GeoPackage）。
package format

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Feature 表示一个待导出的要素（几何为 GeoJSON 几何对象）。
type Feature struct {
	ID         any
	Geometry   map[string]any
	Properties map[string]any
}

// ImportFeature 表示一个待导入的要素（几何载体三选一）。
type ImportFeature struct {
	Properties map[string]any
	GeoJSON    map[string]any // GeoJSON 几何
	WKT        string
	WKB        []byte
}

// ParseFeatureCollection 解析 GeoJSON Feature / FeatureCollection。
func ParseFeatureCollection(data []byte) ([]Feature, error) {
	var probe struct {
		Type     string            `json:"type"`
		Features []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("GeoJSON 解析失败: %w", err)
	}

	var raws []json.RawMessage
	switch probe.Type {
	case "FeatureCollection":
		raws = probe.Features
	case "Feature":
		raws = []json.RawMessage{data}
	case "Point", "LineString", "Polygon", "MultiPoint", "MultiLineString", "MultiPolygon", "GeometryCollection":
		// 纯几何对象
		raws = []json.RawMessage{data}
	default:
		return nil, fmt.Errorf("不支持的 GeoJSON 类型 %q", probe.Type)
	}

	out := make([]Feature, 0, len(raws))
	for _, raw := range raws {
		var f struct {
			Type       string         `json:"type"`
			ID         any            `json:"id"`
			Geometry   map[string]any `json:"geometry"`
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, err
		}
		if f.Type != "Feature" {
			// 纯几何：作为 geometry 处理
			var g map[string]any
			if err := json.Unmarshal(raw, &g); err != nil {
				return nil, err
			}
			out = append(out, Feature{Geometry: g, Properties: map[string]any{}})
			continue
		}
		out = append(out, Feature{ID: f.ID, Geometry: f.Geometry, Properties: f.Properties})
	}
	return out, nil
}

// ToImport 将导出的 Feature 转为导入要素（GeoJSON 几何）。
func (f Feature) ToImport() ImportFeature {
	return ImportFeature{Properties: f.Properties, GeoJSON: f.Geometry}
}

// ── 几何 → WKT ──────────────────────────────────────────────────

// GeometryToWKT 将 GeoJSON 几何转为 WKT。
func GeometryToWKT(g map[string]any) string {
	if g == nil {
		return ""
	}
	typ, _ := g["type"].(string)
	switch typ {
	case "Point":
		return "POINT" + pointWKT(coord(g["coordinates"]))
	case "MultiPoint":
		return "MULTIPOINT" + multiPointWKT(coords(g["coordinates"]))
	case "LineString":
		return "LINESTRING" + lineWKT(coords(g["coordinates"]))
	case "MultiLineString":
		return "MULTILINESTRING" + multiLineWKT(coords2(g["coordinates"]))
	case "Polygon":
		return "POLYGON" + polygonWKT(coords2(g["coordinates"]))
	case "MultiPolygon":
		var parts []string
		for _, poly := range coords3(g["coordinates"]) {
			parts = append(parts, polygonWKT(poly))
		}
		return "MULTIPOLYGON(" + strings.Join(parts, ",") + ")"
	default:
		return ""
	}
}

func num(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64)
	case json.Number:
		return n.String()
	default:
		return "0"
	}
}

func coord(v any) []any {
	if a, ok := v.([]any); ok {
		return a
	}
	return nil
}

func coords(v any) [][]any {
	if a, ok := v.([]any); ok {
		out := make([][]any, 0, len(a))
		for _, p := range a {
			out = append(out, coord(p))
		}
		return out
	}
	return nil
}

func coords2(v any) [][][]any {
	if a, ok := v.([]any); ok {
		out := make([][][]any, 0, len(a))
		for _, r := range a {
			out = append(out, coords(r))
		}
		return out
	}
	return nil
}

func coords3(v any) [][][][]any {
	if a, ok := v.([]any); ok {
		out := make([][][][]any, 0, len(a))
		for _, p := range a {
			out = append(out, coords2(p))
		}
		return out
	}
	return nil
}

func pointWKT(c []any) string {
	if len(c) < 2 {
		return "EMPTY"
	}
	return "(" + num(c[0]) + " " + num(c[1]) + ")"
}

func multiPointWKT(cs [][]any) string {
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		parts = append(parts, pointWKT(c))
	}
	return "(" + strings.Join(parts, ",") + ")"
}

func lineWKT(cs [][]any) string {
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		if len(c) >= 2 {
			parts = append(parts, num(c[0])+" "+num(c[1]))
		}
	}
	return "(" + strings.Join(parts, ",") + ")"
}

func multiLineWKT(ls [][][]any) string {
	parts := make([]string, 0, len(ls))
	for _, l := range ls {
		parts = append(parts, lineWKT(l))
	}
	return "(" + strings.Join(parts, ",") + ")"
}

func polygonWKT(rings [][][]any) string {
	parts := make([]string, 0, len(rings))
	for _, r := range rings {
		parts = append(parts, lineWKT(r))
	}
	return "(" + strings.Join(parts, ",") + ")"
}
