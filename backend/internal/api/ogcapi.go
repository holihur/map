package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"mapserver/internal/models"
	"mapserver/internal/store"
)

// OGC API - Features (Part 1: Core + GeoJSON) 的最小完整实现。
// 与 WFS 并存，提供现代 REST/JSON 接口。

type ogcLink struct {
	Href  string `json:"href"`
	Rel   string `json:"rel"`
	Type  string `json:"type,omitempty"`
	Title string `json:"title,omitempty"`
}

func (h *Handlers) ogcLanding(w http.ResponseWriter, r *http.Request) {
	base := requestBaseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"title":       "MapServer OGC API Features",
		"description": "MapServer 的 OGC API - Features 服务（GeoJSON）",
		"links": []ogcLink{
			{Href: base + "/ogcapi", Rel: "self", Type: "application/json", Title: "本页"},
			{Href: base + "/ogcapi/conformance", Rel: "conformance", Type: "application/json"},
			{Href: base + "/ogcapi/collections", Rel: "data", Type: "application/json", Title: "要素集合"},
			{Href: base + "/api/wfs?SERVICE=WFS&REQUEST=GetCapabilities", Rel: "alternate", Type: "application/xml", Title: "WFS 2.0"},
		},
	})
}

func (h *Handlers) ogcConformance(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"conformsTo": []string{
			"http://www.opengis.net/spec/ogcapi-common-1/1.0/conf/core",
			"http://www.opengis.net/spec/ogcapi-common-1/1.0/conf/landing-page",
			"http://www.opengis.net/spec/ogcapi-common-1/1.0/conf/json",
			"http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/core",
			"http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/geojson",
			"http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/oas30",
		},
	})
}

func (h *Handlers) ogcCollections(w http.ResponseWriter, r *http.Request) {
	base := requestBaseURL(r)
	layers, err := h.readableLayers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	collections := make([]map[string]any, 0, len(layers))
	for i := range layers {
		collections = append(collections, ogcCollectionDoc(base, &layers[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"links": []ogcLink{
			{Href: base + "/ogcapi/collections", Rel: "self", Type: "application/json"},
			{Href: base + "/ogcapi", Rel: "root", Type: "application/json"},
		},
		"collections": collections,
	})
}

func ogcCollectionDoc(base string, l *models.Layer) map[string]any {
	colURL := base + "/ogcapi/collections/" + url.PathEscape(l.Name)
	doc := map[string]any{
		"id":          l.Name,
		"title":       firstNonEmpty(l.Title, l.Name),
		"description": l.Description,
		"itemType":    "feature",
		"crs":         []string{"http://www.opengis.net/def/crs/OGC/1.3/CRS84"},
		"storageCrs":  fmt.Sprintf("http://www.opengis.net/def/crs/EPSG/0/%d", l.SRID),
		"links": []ogcLink{
			{Href: colURL, Rel: "self", Type: "application/json", Title: l.Title},
			{Href: colURL + "/items", Rel: "items", Type: "application/geo+json", Title: "要素"},
		},
	}
	if l.Bounds != nil {
		doc["extent"] = map[string]any{
			"spatial": map[string]any{
				"bbox": [][4]float64{{l.Bounds[0], l.Bounds[1], l.Bounds[2], l.Bounds[3]}},
				"crs":  "http://www.opengis.net/def/crs/OGC/1.3/CRS84",
			},
		}
	}
	return doc
}

func (h *Handlers) ogcCollection(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.ogcLookupLayer(w, r)
	if !ok {
		return
	}
	base := requestBaseURL(r)
	writeJSON(w, http.StatusOK, ogcCollectionDoc(base, layer))
}

func (h *Handlers) ogcItems(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.ogcLookupLayer(w, r)
	if !ok {
		return
	}
	q, err := h.parseOGCQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	raw, total, err := h.store.FeaturesGeoJSON(r.Context(), layer, q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	base := requestBaseURL(r)
	returned := 0
	if features, ok := doc["features"].([]any); ok {
		returned = len(features)
	}
	doc["numberMatched"] = total
	doc["numberReturned"] = returned
	doc["timeStamp"] = time.Now().UTC().Format(time.RFC3339)
	self := base + "/ogcapi/collections/" + url.PathEscape(layer.Name) + "/items"
	if r.URL.RawQuery != "" {
		self += "?" + r.URL.RawQuery
	}
	links := []ogcLink{
		{Href: self, Rel: "self", Type: "application/geo+json"},
		{Href: base + "/ogcapi/collections/" + url.PathEscape(layer.Name), Rel: "collection", Type: "application/json"},
	}
	if int64(q.Offset+q.Limit) < total {
		next := *r.URL
		qv := next.Query()
		qv.Set("offset", strconv.Itoa(q.Offset+q.Limit))
		next.RawQuery = qv.Encode()
		links = append(links, ogcLink{Href: base + next.RequestURI(), Rel: "next", Type: "application/geo+json"})
	}
	if q.Offset > 0 {
		prev := *r.URL
		qv := prev.Query()
		prevOffset := q.Offset - q.Limit
		if prevOffset < 0 {
			prevOffset = 0
		}
		qv.Set("offset", strconv.Itoa(prevOffset))
		prev.RawQuery = qv.Encode()
		links = append(links, ogcLink{Href: base + prev.RequestURI(), Rel: "prev", Type: "application/geo+json"})
	}
	doc["links"] = links
	delete(doc, "total")
	w.Header().Set("Content-Type", "application/geo+json; charset=utf-8")
	w.Header().Set("Content-Crs", "<http://www.opengis.net/def/crs/OGC/1.3/CRS84>")
	w.Header().Set("X-Total-Count", strconv.FormatInt(total, 10))
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(doc)
}

func (h *Handlers) ogcItem(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.ogcLookupLayer(w, r)
	if !ok {
		return
	}
	fid, err := strconv.ParseInt(chi.URLParam(r, "fid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errString("fid 非法"))
		return
	}
	raw, err := h.store.GetFeature(r.Context(), layer, fid, 4326)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	var feat map[string]any
	if err := json.Unmarshal(raw, &feat); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	base := requestBaseURL(r)
	feat["links"] = []ogcLink{
		{Href: base + "/ogcapi/collections/" + url.PathEscape(layer.Name) + "/items/" + strconv.FormatInt(fid, 10), Rel: "self", Type: "application/geo+json"},
		{Href: base + "/ogcapi/collections/" + url.PathEscape(layer.Name), Rel: "collection", Type: "application/json"},
	}
	w.Header().Set("Content-Type", "application/geo+json; charset=utf-8")
	w.Header().Set("Content-Crs", "<http://www.opengis.net/def/crs/OGC/1.3/CRS84>")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(feat)
}

func (h *Handlers) ogcLookupLayer(w http.ResponseWriter, r *http.Request) (*models.Layer, bool) {
	name := chi.URLParam(r, "name")
	layer, err := h.store.GetLayerByName(r.Context(), name)
	if err != nil {
		writeError(w, statusForError(err), err)
		return nil, false
	}
	if !h.layerReadAllowed(r.Context(), layer) {
		writeError(w, http.StatusForbidden, errString("没有该图层的访问权限"))
		return nil, false
	}
	return layer, true
}

func (h *Handlers) parseOGCQuery(r *http.Request) (store.FeatureQuery, error) {
	q := store.FeatureQuery{
		Limit:    10,
		OutSRID:  4326,
		BBoxSRID: 4326,
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Limit = n
		}
	}
	if q.Limit < 1 {
		q.Limit = 10
	}
	if q.Limit > h.cfg.MaxFeatures {
		q.Limit = h.cfg.MaxFeatures
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			q.Offset = n
		}
	}
	if v := r.URL.Query().Get("bbox"); v != "" {
		parts := strings.Split(v, ",")
		if len(parts) != 4 && len(parts) != 6 {
			return q, errString("bbox 需要 4 或 6 个数字")
		}
		b := [4]float64{}
		for i := 0; i < 4; i++ {
			f, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
			if err != nil {
				return q, errString("bbox 解析失败")
			}
			b[i] = f
		}
		q.BBox = &b
	}
	if v := r.URL.Query().Get("bbox-crs"); v != "" {
		if srid, ok := parseOGCBBoxCRS(v); ok {
			q.BBoxSRID = srid
		} else {
			return q, errString("无法识别的 bbox-crs")
		}
	}
	if v := r.URL.Query().Get("sortby"); v != "" {
		field := strings.TrimSpace(v)
		desc := false
		if strings.HasPrefix(field, "-") {
			desc = true
			field = field[1:]
		} else if strings.HasPrefix(field, "+") {
			field = field[1:]
		}
		q.OrderBy = field
		q.OrderDesc = desc
	}
	q.TextSearch = strings.TrimSpace(r.URL.Query().Get("q"))
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
	return q, nil
}

func parseOGCBBoxCRS(v string) (int, bool) {
	v = strings.TrimSpace(v)
	if strings.EqualFold(v, "CRS84") || strings.Contains(v, "/CRS84") {
		return 4326, true
	}
	if strings.HasPrefix(strings.ToUpper(v), "EPSG:") {
		n, err := strconv.Atoi(v[5:])
		return n, err == nil
	}
	// http://www.opengis.net/def/crs/EPSG/0/3857
	if idx := strings.LastIndex(v, "/"); idx >= 0 {
		n, err := strconv.Atoi(v[idx+1:])
		return n, err == nil
	}
	return 0, false
}
