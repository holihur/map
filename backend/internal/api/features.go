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
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !h.requireLayerWrite(w, r, layer) {
		return
	}
	q, err := h.parseFeatureQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
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

// batchFeatures 批量删除 / 更新（按 ids 或过滤条件）。
func (h *Handlers) batchFeatures(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !h.requireLayerWrite(w, r, layer) {
		return
	}
	var req struct {
		Action     string            `json:"action"`
		IDs        []int64           `json:"ids"`
		Filters    map[string]string `json:"filters"`
		BBox       *[4]float64       `json:"bbox"`
		Properties map[string]any    `json:"properties"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	q := store.FeatureQuery{IDs: req.IDs, BBox: req.BBox, BBoxSRID: 4326}
	for k, v := range req.Filters {
		field, op := k, "eq"
		if idx := strings.LastIndex(k, "."); idx > 0 {
			field, op = k[:idx], k[idx+1:]
		}
		q.Filters = append(q.Filters, store.Filter{Field: field, Op: op, Value: v})
	}
	var (
		affected int64
		err      error
	)
	switch strings.ToLower(req.Action) {
	case "delete":
		affected, err = h.store.DeleteFeaturesWhere(r.Context(), layer, q)
	case "update":
		affected, err = h.store.UpdateFeaturesWhere(r.Context(), layer, q, req.Properties)
	default:
		writeError(w, http.StatusBadRequest, errString("action 必须是 delete 或 update"))
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.invalidateLayer(layer.Name)
	writeJSON(w, http.StatusOK, map[string]any{"affected": affected})
}

// fieldStats 返回图层字段统计（供样式分级 / 数据概览）。
func (h *Handlers) fieldStats(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !h.requireLayerWrite(w, r, layer) {
		return
	}
	stats, err := h.store.FieldStats(r.Context(), layer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": stats})
}

func (h *Handlers) getFeature(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !h.requireLayerWrite(w, r, layer) {
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
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !h.requireLayerWrite(w, r, layer) {
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
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !h.requireLayerWrite(w, r, layer) {
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
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !h.requireLayerWrite(w, r, layer) {
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

// analyzeFeatures 对查询结果做空间分析并返回 GeoJSON（不写库）。
func (h *Handlers) analyzeFeatures(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	var req struct {
		Op        string            `json:"op"`
		Distance  float64           `json:"distance"`
		Tolerance float64           `json:"tolerance"`
		IDs       []int64           `json:"ids"`
		BBox      *[4]float64       `json:"bbox"`
		Filters   map[string]string `json:"filters"`
		OutSRID   int               `json:"outSrid"`
		Limit     int               `json:"limit"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	q := store.FeatureQuery{IDs: req.IDs, BBox: req.BBox, BBoxSRID: 4326, OutSRID: req.OutSRID}
	for k, v := range req.Filters {
		field, op := k, "eq"
		if idx := strings.LastIndex(k, "."); idx > 0 {
			field, op = k[:idx], k[idx+1:]
		}
		q.Filters = append(q.Filters, store.Filter{Field: field, Op: op, Value: v})
	}
	data, total, err := h.store.AnalyzeFeatures(r.Context(), layer, q, store.AnalyzeRequest{
		Op: req.Op, Distance: req.Distance, Tolerance: req.Tolerance,
		OutSRID: req.OutSRID, Limit: req.Limit,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "application/geo+json")
	w.Header().Set("X-Total-Count", strconv.FormatInt(total, 10))
	_, _ = w.Write(data)
}
