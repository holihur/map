package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"mapserver/internal/store"
)

type geoJSONFeature struct {
	Type       string          `json:"type"`
	Geometry   json.RawMessage `json:"geometry"`
	Properties map[string]any  `json:"properties"`
}

type geoJSONFeatureCollection struct {
	Type     string           `json:"type"`
	Features []geoJSONFeature `json:"features"`
}

func (h *Handlers) listFeatures(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	q := store.FeatureQuery{
		Limit:     h.cfg.MaxFeatures,
		OutSRID:   4326,
		BBoxSRID:  4326,
		OrderBy:   r.URL.Query().Get("orderBy"),
		OrderDesc: r.URL.Query().Get("order") == "desc",
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Offset = n
		}
	}
	if v := r.URL.Query().Get("outSrid"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.OutSRID = n
		}
	}
	if v := r.URL.Query().Get("bboxSrid"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.BBoxSRID = n
		}
	}
	if v := r.URL.Query().Get("simplify"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			q.Simplify = f
		}
	}
	if v := r.URL.Query().Get("bbox"); v != "" {
		parts := strings.Split(v, ",")
		if len(parts) != 4 {
			writeError(w, http.StatusBadRequest, errString("bbox 需要 4 个逗号分隔的数字"))
			return
		}
		var b [4]float64
		for i, p := range parts {
			f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, errString("bbox 解析失败"))
				return
			}
			b[i] = f
		}
		q.BBox = &b
	}
	// 属性过滤：f.<field> 或 f.<field>.<op>
	for key, vals := range r.URL.Query() {
		if !strings.HasPrefix(key, "f.") || len(vals) == 0 {
			continue
		}
		rest := strings.TrimPrefix(key, "f.")
		field, op := rest, "eq"
		if idx := strings.LastIndex(rest, "."); idx > 0 {
			field, op = rest[:idx], rest[idx+1:]
		}
		q.Filters = append(q.Filters, store.Filter{Field: field, Op: op, Value: vals[0]})
	}

	data, _, err := h.store.FeaturesGeoJSON(r.Context(), layer, q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handlers) getFeature(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	fid, err := strconv.ParseInt(chi.URLParam(r, "fid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errString("fid 非法"))
		return
	}
	outSrid := 4326
	if v := r.URL.Query().Get("outSrid"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			outSrid = n
		}
	}
	data, err := h.store.GetFeature(r.Context(), layer, fid, outSrid)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(data)
}

func (h *Handlers) createFeatures(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	var raw json.RawMessage
	if err := decodeJSON(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// 判断是单个 Feature 还是 FeatureCollection。
	var probe struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(raw, &probe)

	var features []geoJSONFeature
	if probe.Type == "FeatureCollection" {
		var fc geoJSONFeatureCollection
		if err := json.Unmarshal(raw, &fc); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		features = fc.Features
	} else {
		var f geoJSONFeature
		if err := json.Unmarshal(raw, &f); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		features = []geoJSONFeature{f}
	}
	if len(features) == 0 {
		writeError(w, http.StatusBadRequest, errString("没有可插入的要素"))
		return
	}

	ids := make([]int64, 0, len(features))
	inputSrid := inputSRID(r)
	for _, f := range features {
		fid, err := h.store.InsertFeature(r.Context(), layer, f.Geometry, f.Properties, inputSrid)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		ids = append(ids, fid)
	}
	h.invalidateLayer(layer.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"ids": ids, "count": len(ids)})
}

func (h *Handlers) updateFeature(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	fid, err := strconv.ParseInt(chi.URLParam(r, "fid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errString("fid 非法"))
		return
	}
	var f geoJSONFeature
	if err := decodeJSON(r, &f); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.store.UpdateFeature(r.Context(), layer, fid, f.Geometry, f.Properties, inputSRID(r)); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	h.invalidateLayer(layer.Name)
	data, err := h.store.GetFeature(r.Context(), layer, fid, 4326)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(data)
}

func (h *Handlers) deleteFeature(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	fid, err := strconv.ParseInt(chi.URLParam(r, "fid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errString("fid 非法"))
		return
	}
	if err := h.store.DeleteFeature(r.Context(), layer, fid); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	h.invalidateLayer(layer.Name)
	w.WriteHeader(http.StatusNoContent)
}

// inputSRID 从查询参数读取输入几何的坐标系，默认 4326（GeoJSON 规范）。
func inputSRID(r *http.Request) int {
	if v := r.URL.Query().Get("inputSrid"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 4326
}
