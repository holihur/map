package api

import "net/http"

// capabilities 输出服务能力文档（参考 WMS/WFS GetCapabilities 的 JSON 形式）。
func (h *Handlers) capabilities(w http.ResponseWriter, r *http.Request) {
	{
		layers, err := h.readableLayers(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		styles, _ := h.store.ListStyles(r.Context())

		type layerInfo struct {
			Name         string `json:"name"`
			Title        string `json:"title"`
			GeometryType string `json:"geometryType"`
			SRID         int    `json:"srid"`
			Fields       any    `json:"fields"`
			MinZoom      int    `json:"minZoom"`
			MaxZoom      int    `json:"maxZoom"`
			Bounds       any    `json:"bounds,omitempty"`
			FeatureCount int64  `json:"featureCount"`
			Tiles        string `json:"tiles"`
			Features     string `json:"features"`
		}
		infos := make([]layerInfo, 0, len(layers))
		for _, l := range layers {
			infos = append(infos, layerInfo{
				Name:         l.Name,
				Title:        l.Title,
				GeometryType: l.GeometryType,
				SRID:         l.SRID,
				Fields:       l.Fields,
				MinZoom:      l.MinZoom,
				MaxZoom:      l.MaxZoom,
				Bounds:       l.Bounds,
				FeatureCount: l.FeatureCount,
				Tiles:        "/api/tiles/" + l.Name + "/{z}/{x}/{y}.mvt",
				Features:     "/api/layers/" + l.Name + "/features",
			})
		}

		external, _ := h.readableExternalLayers(r.Context())
		type externalInfo struct {
			Name       string `json:"name"`
			Title      string `json:"title"`
			SourceType string `json:"sourceType"`
			MinZoom    int    `json:"minZoom"`
			MaxZoom    int    `json:"maxZoom"`
			Visibility string `json:"visibility"`
		}
		extInfos := make([]externalInfo, 0, len(external))
		for _, l := range external {
			extInfos = append(extInfos, externalInfo{
				Name: l.Name, Title: l.Title, SourceType: l.SourceType,
				MinZoom: l.MinZoom, MaxZoom: l.MaxZoom, Visibility: l.Visibility,
			})
		}

		doc := map[string]any{
			"service":     "MapServer",
			"version":     "1.0.0",
			"description": "参考 Mapbox 与 GeoServer 的矢量瓦片地图服务",
			"auth": map[string]any{
				"scheme": "Bearer JWT",
				"roles":  []string{"admin", "editor", "viewer"},
			},
			"services": map[string]any{
				"wms":             "/api/wms?SERVICE=WMS&REQUEST=GetCapabilities",
				"wmsGetMap":       "/api/wms?SERVICE=WMS&VERSION=1.3.0&REQUEST=GetMap&LAYERS={layer}&CRS=EPSG:4326&BBOX={miny},{minx},{maxy},{maxx}&WIDTH=600&HEIGHT=300&FORMAT=image/png",
				"tileMatrixSets":  "/api/tilematrixsets",
				"vectorTiles":     "/api/tiles/{layer}/{z}/{x}/{y}.mvt",
				"vectorTilesTMS":  "/api/tiles/{layer}/{tms}/{z}/{x}/{y}.mvt",
				"featuresGeoJSON": "/api/layers/{layer}/features",
			},
			"formats": map[string]any{
				"tiles":    []string{"application/vnd.mapbox-vector-tile"},
				"features": []string{"application/geo+json"},
				"images":   []string{"image/png"},
			},
			"layers":         infos,
			"externalLayers": extInfos,
			"styles":         styles,
		}
		writeJSON(w, http.StatusOK, doc)
	}
}
