package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"mapserver/internal/format"
	"mapserver/internal/models"
	"mapserver/internal/store"
)

// importNewLayer 从上传文件自动推断 schema、创建图层并导入。
// 传 preview=true 时只返回推断结果，不写库。
func (h *Handlers) importNewLayer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, errString("缺少上传文件字段 file"))
		return
	}
	defer file.Close()

	filename := strings.ToLower(header.Filename)
	formatName := strings.ToLower(r.FormValue("format"))
	if formatName == "" {
		switch {
		case strings.HasSuffix(filename, ".zip"):
			formatName = "shapefile"
		case strings.HasSuffix(filename, ".csv"):
			formatName = "csv"
		case strings.HasSuffix(filename, ".gpkg"):
			formatName = "gpkg"
		default:
			formatName = "geojson"
		}
	}

	srid := 4326
	if v := r.FormValue("srid"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			srid = n
		}
	}

	features, formatName, err := parseImportFeatures(file, formatName, r.FormValue("table"), nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(features) == 0 {
		writeError(w, http.StatusBadRequest, errString("文件中没有可导入的要素"))
		return
	}

	geometryType := inferGeometryType(features)
	fields := inferFields(features)
	name := sanitizeLayerName(r.FormValue("name"))
	if name == "" {
		name = sanitizeLayerName(strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename)))
	}
	if name == "" {
		name = "imported"
	}
	name = h.uniqueLayerName(r, name)

	preview := strings.EqualFold(r.FormValue("preview"), "true") ||
		strings.EqualFold(r.FormValue("preview"), "1")
	sample := make([]map[string]any, 0, 3)
	for i, f := range features {
		if i >= 3 {
			break
		}
		sample = append(sample, f.Properties)
	}
	if preview {
		writeJSON(w, http.StatusOK, map[string]any{
			"preview":      true,
			"name":         name,
			"format":       formatName,
			"srid":         srid,
			"geometryType": geometryType,
			"fields":       fields,
			"count":        len(features),
			"sample":       sample,
		})
		return
	}

	layer, err := h.store.CreateLayer(r.Context(), models.CreateLayerRequest{
		Name:         name,
		Title:        firstNonEmpty(r.FormValue("title"), name),
		Description:  r.FormValue("description"),
		GeometryType: geometryType,
		SRID:         srid,
		Fields:       fields,
		Style:        defaultStyleFor(geometryType),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	inserted, failed := 0, make([]string, 0)
	for i, f := range features {
		g := store.GeomValue{WKT: f.WKT, WKB: f.WKB, SRID: srid}
		if f.GeoJSON != nil {
			if b, err := json.Marshal(f.GeoJSON); err == nil {
				g.GeoJSON = b
			}
		}
		if _, err := h.store.InsertFeatureGeom(r.Context(), layer, g, f.Properties); err != nil {
			failed = append(failed, fmt.Sprintf("第 %d 行: %v", i+1, err))
			continue
		}
		inserted++
	}
	h.invalidateLayer(layer.Name)
	status := http.StatusCreated
	if inserted == 0 {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]any{
		"layer":        layer,
		"imported":     inserted,
		"failed":       len(failed),
		"errors":       failed,
		"format":       formatName,
		"geometryType": geometryType,
	})
}

// parseImportFeatures 解析上传文件为统一导入要素。
func parseImportFeatures(file io.Reader, formatName, table string, fields []models.Field) ([]format.ImportFeature, string, error) {
	switch formatName {
	case "geojson", "json":
		data, err := readAllLimited(file, 256<<20)
		if err != nil {
			return nil, formatName, err
		}
		parsed, err := format.ParseFeatureCollection(data)
		if err != nil {
			return nil, formatName, err
		}
		out := make([]format.ImportFeature, 0, len(parsed))
		for _, f := range parsed {
			out = append(out, f.ToImport())
		}
		return out, "geojson", nil
	case "csv":
		parsed, err := format.ParseCSV(file, fields)
		return parsed, "csv", err
	case "shapefile", "shp", "zip":
		parsed, _, err := format.ReadShapefileZip(file)
		return parsed, "shapefile", err
	case "gpkg", "geopackage":
		parsed, err := format.ReadGeoPackageTable(file, table)
		return parsed, "gpkg", err
	default:
		return nil, formatName, fmt.Errorf("不支持的导入格式: %s", formatName)
	}
}

func inferGeometryType(features []format.ImportFeature) string {
	for _, f := range features {
		if f.GeoJSON != nil {
			if t, ok := f.GeoJSON["type"].(string); ok && t != "" {
				return strings.ToUpper(t)
			}
		}
		if f.WKT != "" {
			head := strings.TrimSpace(f.WKT)
			if idx := strings.IndexAny(head, "( "); idx > 0 {
				head = head[:idx]
			}
			// 去掉 EWKT 的 SRID=xxx; 前缀
			if idx := strings.LastIndex(head, ";"); idx >= 0 {
				head = head[idx+1:]
			}
			return strings.ToUpper(strings.TrimSpace(head))
		}
	}
	return "GEOMETRY"
}

func inferFields(features []format.ImportFeature) []models.Field {
	order := make([]string, 0)
	values := map[string][]any{}
	lowerNames := map[string]string{}
	for _, f := range features {
		for k, v := range f.Properties {
			key := strings.ToLower(strings.TrimSpace(k))
			if key == "" {
				continue
			}
			orig, seen := lowerNames[key]
			if !seen {
				orig = k
				lowerNames[key] = k
				order = append(order, key)
			}
			if len(values[key]) < 200 {
				values[key] = append(values[key], v)
				_ = orig
			}
		}
	}
	out := make([]models.Field, 0, len(order))
	used := map[string]bool{}
	for _, key := range order {
		name := sanitizeFieldName(key)
		if name == "" || used[name] || isReservedField(name) {
			continue
		}
		used[name] = true
		out = append(out, models.Field{Name: name, Type: inferFieldType(values[key])})
	}
	if len(out) == 0 {
		out = append(out, models.Field{Name: "name", Type: models.FieldText})
	}
	return out
}

func inferFieldType(vals []any) models.FieldType {
	nonNull := make([]string, 0, len(vals))
	for _, v := range vals {
		if v == nil {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" {
			continue
		}
		nonNull = append(nonNull, s)
	}
	if len(nonNull) == 0 {
		return models.FieldText
	}
	boolCount, intCount, floatCount := 0, 0, 0
	maxAbs := int64(0)
	for _, s := range nonNull {
		if _, err := strconv.ParseBool(s); err == nil || s == "0" || s == "1" {
			boolCount++
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			intCount++
			if n < 0 {
				n = -n
			}
			if n > maxAbs {
				maxAbs = n
			}
		}
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			floatCount++
		}
	}
	switch {
	case boolCount == len(nonNull):
		return models.FieldBoolean
	case intCount == len(nonNull):
		if maxAbs > 2147483647 {
			return models.FieldBigInt
		}
		return models.FieldInteger
	case floatCount == len(nonNull):
		return models.FieldDouble
	default:
		return models.FieldText
	}
}

func sanitizeLayerName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	prevUnderscore := false
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			prevUnderscore = false
		} else if !prevUnderscore {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	for len(out) > 0 && (out[0] < 'a' || out[0] > 'z') {
		out = out[1:]
	}
	return out
}

func sanitizeFieldName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	prevUnderscore := false
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			prevUnderscore = false
		} else if !prevUnderscore {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	for len(out) > 0 && (out[0] < 'a' || out[0] > 'z') {
		out = "f_" + out
	}
	return out
}

func isReservedField(name string) bool {
	switch name {
	case "fid", "geom", "created_at", "updated_at":
		return true
	}
	return false
}

func (h *Handlers) uniqueLayerName(r *http.Request, name string) string {
	candidate := name
	for i := 2; i < 100; i++ {
		if _, err := h.store.GetLayerByName(r.Context(), candidate); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return candidate
			}
			return candidate
		}
		candidate = fmt.Sprintf("%s_%d", name, i)
	}
	return candidate
}

func defaultStyleFor(geometryType string) json.RawMessage {
	switch {
	case strings.Contains(geometryType, "POINT"):
		return json.RawMessage(`{"type":"circle","paint":{"circle-radius":5,"circle-color":"#2563eb","circle-stroke-color":"#ffffff","circle-stroke-width":1.5}}`)
	case strings.Contains(geometryType, "LINE"):
		return json.RawMessage(`{"type":"line","paint":{"line-color":"#2563eb","line-width":2}}`)
	default:
		return json.RawMessage(`{"type":"fill","paint":{"fill-color":"#60a5fa","fill-opacity":0.5,"fill-outline-color":"#1d4ed8"}}`)
	}
}
