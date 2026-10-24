package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"mapserver/internal/models"
	"mapserver/internal/store"
)

// search 在图层元数据与文本属性中搜索，供地图页快速定位。
func (h *Handlers) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"layers": []any{}, "features": []any{}})
		return
	}
	limit := 5
	if v := r.URL.Query().Get("limit"); v != "" {
		if n := atoiDefault(v, 5); n > 0 && n <= 50 {
			limit = n
		}
	}
	layerName := r.URL.Query().Get("layer")

	all, err := h.store.ListLayers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	layers := make([]models.Layer, 0)
	for i := range all {
		if h.layerReadAllowed(r.Context(), &all[i]) {
			layers = append(layers, all[i])
		}
	}

	lower := strings.ToLower(q)
	layerHits := make([]map[string]any, 0)
	for _, l := range layers {
		if strings.Contains(strings.ToLower(l.Name), lower) ||
			strings.Contains(strings.ToLower(l.Title), lower) ||
			strings.Contains(strings.ToLower(l.Description), lower) {
			layerHits = append(layerHits, map[string]any{
				"name": l.Name, "title": l.Title, "geometryType": l.GeometryType,
				"bounds": l.Bounds, "featureCount": l.FeatureCount,
			})
		}
	}
	if external, err := h.readableExternalLayers(r.Context()); err == nil {
		for _, l := range external {
			if strings.Contains(strings.ToLower(l.Name), lower) ||
				strings.Contains(strings.ToLower(l.Title), lower) ||
				strings.Contains(strings.ToLower(l.Description), lower) {
				layerHits = append(layerHits, map[string]any{
					"name": l.Name, "title": l.Title, "geometryType": "RASTER",
					"featureCount": 0, "external": true,
				})
			}
		}
	}

	type featureHit struct {
		Layer      string          `json:"layer"`
		LayerTitle string          `json:"layerTitle"`
		Feature    json.RawMessage `json:"feature"`
	}
	featureHits := make([]featureHit, 0)
	scanned := 0
	for i := range layers {
		l := &layers[i]
		if layerName != "" && l.Name != layerName {
			continue
		}
		if scanned >= 20 {
			break
		}
		hasText := false
		for _, f := range l.Fields {
			if f.Type == models.FieldText {
				hasText = true
				break
			}
		}
		if !hasText {
			continue
		}
		scanned++
		raw, _, err := h.store.FeaturesGeoJSON(r.Context(), l, store.FeatureQuery{
			Limit: limit, OutSRID: 4326, TextSearch: q,
		})
		if err != nil {
			continue
		}
		var fc struct {
			Features []json.RawMessage `json:"features"`
		}
		if json.Unmarshal(raw, &fc) != nil {
			continue
		}
		for _, f := range fc.Features {
			featureHits = append(featureHits, featureHit{
				Layer: l.Name, LayerTitle: l.Title, Feature: f,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"layers": layerHits, "features": featureHits})
}
