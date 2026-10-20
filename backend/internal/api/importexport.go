package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"mapserver/internal/format"
	"mapserver/internal/store"
)

// exportLayer 导出图层数据：geojson / csv / shapefile / gpkg。
func (h *Handlers) exportLayer(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	formatName := strings.ToLower(r.URL.Query().Get("format"))
	if formatName == "" {
		formatName = "geojson"
	}
	outSRID := 4326
	if v := r.URL.Query().Get("outSrid"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			outSRID = n
		}
	}

	raw, _, err := h.store.FeaturesGeoJSON(r.Context(), layer, store.FeatureQuery{
		Limit:   h.cfg.MaxFeatures,
		OutSRID: outSRID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	features, err := format.ParseFeatureCollection(raw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	switch formatName {
	case "geojson", "json":
		w.Header().Set("Content-Type", "application/geo+json")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.geojson"`, layer.Name))
		_, _ = w.Write(raw)

	case "csv":
		var buf bytes.Buffer
		if err := format.WriteCSV(&buf, layer.Fields, features); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, layer.Name))
		_, _ = w.Write(buf.Bytes())

	case "shp", "shapefile", "zip":
		prj, _ := h.store.CRSWKT(r.Context(), layer.SRID)
		var buf bytes.Buffer
		if err := format.WriteShapefileZip(&buf, layer.Name, layer.Fields, prj, features); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, layer.Name))
		_, _ = w.Write(buf.Bytes())

	case "gpkg", "geopackage":
		srsWKT, _ := h.store.CRSWKT(r.Context(), outSRID)
		w.Header().Set("Content-Type", "application/geopackage+sqlite3")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.gpkg"`, layer.Name))
		if err := format.WriteGeoPackageMulti(w, []format.GPKGSource{{
			Name:         layer.Name,
			SRID:         outSRID,
			GeometryType: layer.GeometryType,
			Fields:       layer.Fields,
			Features:     features,
			SRSWKT:       srsWKT,
		}}); err != nil {
			writeError(w, http.StatusInternalServerError, err)
		}
		return

	default:
		writeError(w, http.StatusBadRequest, errString("不支持的导出格式: "+formatName))
	}
}

// importLayer 从上传文件导入要素（GeoJSON / CSV / Shapefile zip / GeoPackage）。
func (h *Handlers) importLayer(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
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

	srid := 4326
	if v := r.FormValue("srid"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			srid = n
		}
	}

	name := strings.ToLower(header.Filename)
	formatName := strings.ToLower(r.FormValue("format"))
	if formatName == "" {
		switch {
		case strings.HasSuffix(name, ".zip"):
			formatName = "shapefile"
		case strings.HasSuffix(name, ".csv"):
			formatName = "csv"
		case strings.HasSuffix(name, ".gpkg"):
			formatName = "gpkg"
		default:
			formatName = "geojson"
		}
	}

	var features []format.ImportFeature
	switch formatName {
	case "geojson", "json":
		data, err := readAllLimited(file, 256<<20)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		parsed, err := format.ParseFeatureCollection(data)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		for _, f := range parsed {
			features = append(features, f.ToImport())
		}
	case "csv":
		parsed, err := format.ParseCSV(file, layer.Fields)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		features = parsed
	case "shapefile", "shp", "zip":
		parsed, _, err := format.ReadShapefileZip(file)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		features = parsed
	case "gpkg", "geopackage":
		parsed, err := format.ReadGeoPackageTable(file, r.FormValue("table"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		features = parsed
	default:
		writeError(w, http.StatusBadRequest, errString("不支持的导入格式: "+formatName))
		return
	}

	if len(features) == 0 {
		writeError(w, http.StatusBadRequest, errString("文件中没有可导入的要素"))
		return
	}

	inserted := 0
	for _, f := range features {
		g := store.GeomValue{WKT: f.WKT, WKB: f.WKB, SRID: srid}
		if f.GeoJSON != nil {
			if b, err := json.Marshal(f.GeoJSON); err == nil {
				g.GeoJSON = b
			}
		}
		if _, err := h.store.InsertFeatureGeom(r.Context(), layer, g, f.Properties); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("第 %d 个要素导入失败: %w", inserted+1, err))
			return
		}
		inserted++
	}

	h.invalidateLayer(layer.Name)
	writeJSON(w, http.StatusCreated, map[string]any{
		"layer":    layer.Name,
		"format":   formatName,
		"imported": inserted,
		"srid":     srid,
	})
}

// exportAll 将全部图层导出为单个 GeoPackage（每个图层一张表）。
func (h *Handlers) exportAll(w http.ResponseWriter, r *http.Request) {
	formatName := strings.ToLower(r.URL.Query().Get("format"))
	if formatName == "" {
		formatName = "gpkg"
	}
	if formatName != "gpkg" && formatName != "geopackage" {
		writeError(w, http.StatusBadRequest, errString("多图层导出仅支持 gpkg"))
		return
	}

	layers, err := h.store.ListLayers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	sources := make([]format.GPKGSource, 0, len(layers))
	for i := range layers {
		l := layers[i]
		raw, _, err := h.store.FeaturesGeoJSON(r.Context(), &l, store.FeatureQuery{
			Limit:   h.cfg.MaxFeatures,
			OutSRID: 4326,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		feats, err := format.ParseFeatureCollection(raw)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		srsWKT, _ := h.store.CRSWKT(r.Context(), 4326)
		sources = append(sources, format.GPKGSource{
			Name:         l.Name,
			SRID:         4326,
			GeometryType: l.GeometryType,
			Fields:       l.Fields,
			Features:     feats,
			SRSWKT:       srsWKT,
		})
	}

	w.Header().Set("Content-Type", "application/geopackage+sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="mapserver.gpkg"`)
	if err := format.WriteGeoPackageMulti(w, sources); err != nil {
		writeError(w, http.StatusInternalServerError, err)
	}
}

func readAllLimited(f interface{ Read([]byte) (int, error) }, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(&limitedReader{r: f, n: limit}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type limitedReader struct {
	r interface{ Read([]byte) (int, error) }
	n int64
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, fmt.Errorf("文件超过大小限制")
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}
