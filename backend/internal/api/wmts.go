package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"mapserver/internal/tiles"
)

// wmtsKVP 处理 WMTS 1.0.0 KVP 请求：GetCapabilities / GetTile。
func (h *Handlers) wmtsKVP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch strings.ToUpper(q.Get("REQUEST")) {
	case "GETCAPABILITIES", "":
		h.wmtsCapabilities(w, r)
	case "GETTILE":
		layer := firstNonEmpty(q.Get("LAYER"), q.Get("LAYERS"))
		tmsID := q.Get("TILEMATRIXSET")
		z := atoiDefault(q.Get("TILEMATRIX"), -1)
		y := atoiDefault(q.Get("TILEROW"), -1)
		x := atoiDefault(q.Get("TILECOL"), -1)
		h.serveWMSTile(w, r, layer, tmsID, z, y, x, q.Get("FORMAT"))
	default:
		wmtsException(w, "OperationNotSupported", "仅支持 GetCapabilities 与 GetTile")
	}
}

// wmtsREST 处理 WMTS RESTful 请求：/api/wmts/rest/{layer}/{style}/{tms}/{z}/{y}/{x}.{ext}
func (h *Handlers) wmtsREST(w http.ResponseWriter, r *http.Request) {
	layer := chi.URLParam(r, "layer")
	tmsID := chi.URLParam(r, "tms")
	z := atoiDefault(chi.URLParam(r, "z"), -1)
	y := atoiDefault(chi.URLParam(r, "y"), -1)
	x := atoiDefault(chi.URLParam(r, "x"), -1)
	ext := chi.URLParam(r, "ext")
	format := "image/png"
	if ext == "mvt" || ext == "pbf" {
		format = "application/vnd.mapbox-vector-tile"
	}
	h.serveWMSTile(w, r, layer, tmsID, z, y, x, format)
}

func (h *Handlers) serveWMSTile(w http.ResponseWriter, r *http.Request, layerName, tmsID string, z, y, x int, format string) {
	layer, err := h.store.GetLayerByName(r.Context(), layerName)
	if err != nil {
		wmtsException(w, "TileOutOfRange", "图层不存在: "+layerName)
		return
	}
	if !h.layerReadAllowed(r.Context(), layer) {
		wmtsException(w, "PermissionDenied", "没有该图层的访问权限")
		return
	}
	grid := tiles.Default()
	if tmsID != "" {
		g, ok := tiles.Get(tmsID)
		if !ok {
			wmtsException(w, "InvalidParameterValue", "未知的 TileMatrixSet "+tmsID)
			return
		}
		grid = g
	}
	if z < 0 || !grid.Contains(z, x, y) {
		wmtsException(w, "TileOutOfRange", "瓦片坐标越界")
		return
	}

	env := grid.Envelope(z, x, y)
	lower := strings.ToLower(format)

	if strings.Contains(lower, "png") || format == "" {
		data, _, err := h.rasterTile(r.Context(), layer, grid, z, x, y)
		if err != nil {
			wmtsException(w, "NoApplicableCode", err.Error())
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
		return
	}

	if strings.Contains(lower, "vector") || strings.Contains(lower, "mvt") || strings.Contains(lower, "pbf") {
		data, err := h.store.TileMVT(r.Context(), layer, 4096, grid.SRID, z, h.cfg.TileMaxFeatures, h.cfg.TileSimplify, env)
		if err != nil {
			wmtsException(w, "NoApplicableCode", err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/vnd.mapbox-vector-tile")
		if len(data) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write(data)
		return
	}

	wmtsException(w, "InvalidParameterValue", "不支持的 FORMAT: "+format)
}

func (h *Handlers) wmtsCapabilities(w http.ResponseWriter, r *http.Request) {
	base := requestBaseURL(r)
	layers, err := h.readableLayers(r.Context())
	if err != nil {
		wmtsException(w, "NoApplicableCode", err.Error())
		return
	}
	maxZoom := h.cfg.TileMaxZoom

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<Capabilities xmlns="http://www.opengis.net/wmts/1.0" xmlns:ows="http://www.opengis.net/ows/1.1" xmlns:xlink="http://www.w3.org/1999/xlink" version="1.0.0">`)
	fmt.Fprintf(&b, `<ows:ServiceIdentification><ows:Title>MapServer WMTS</ows:Title><ows:ServiceType>OGC WMTS</ows:ServiceType><ows:ServiceTypeVersion>1.0.0</ows:ServiceTypeVersion></ows:ServiceIdentification>`)
	fmt.Fprintf(&b, `<ows:OperationsMetadata>`)
	fmt.Fprintf(&b, `<ows:Operation name="GetCapabilities"><ows:DCP><ows:HTTP><ows:Get xlink:href="%s/api/wmts?"/></ows:HTTP></ows:DCP></ows:Operation>`, base)
	fmt.Fprintf(&b, `<ows:Operation name="GetTile"><ows:DCP><ows:HTTP><ows:Get xlink:href="%s/api/wmts?"/></ows:HTTP></ows:DCP></ows:Operation>`, base)
	fmt.Fprintf(&b, `</ows:OperationsMetadata>`)

	fmt.Fprintf(&b, `<Contents>`)
	for _, l := range layers {
		title := l.Title
		if title == "" {
			title = l.Name
		}
		fmt.Fprintf(&b, `<Layer><ows:Title>%s</ows:Title><ows:Identifier>%s</ows:Identifier>`, xmlEscape(title), xmlEscape(l.Name))
		fmt.Fprintf(&b, `<Style isDefault="true"><ows:Identifier>default</ows:Identifier></Style>`)
		fmt.Fprintf(&b, `<Format>image/png</Format><Format>application/vnd.mapbox-vector-tile</Format>`)
		for _, g := range tiles.All() {
			bounds := l.Bounds
			if bounds != nil && g.SRID == 3857 {
				bounds = &[4]float64{
					lonToMercator(bounds[0]), latToMercator(bounds[1]),
					lonToMercator(bounds[2]), latToMercator(bounds[3]),
				}
			}
			if bounds != nil {
				fmt.Fprintf(&b, `<ows:WGS84BoundingBox><ows:LowerCorner>%g %g</ows:LowerCorner><ows:UpperCorner>%g %g</ows:UpperCorner></ows:WGS84BoundingBox>`,
					bounds[0], bounds[1], bounds[2], bounds[3])
			}
			fmt.Fprintf(&b, `<TileMatrixSetLink><TileMatrixSet>%s</TileMatrixSet></TileMatrixSetLink>`, g.ID)
		}
		fmt.Fprintf(&b, `</Layer>`)
	}
	for _, g := range tiles.All() {
		fmt.Fprintf(&b, `<TileMatrixSet><ows:Identifier>%s</ows:Identifier><ows:SupportedCRS>urn:ogc:def:crs:%s</ows:SupportedCRS>`,
			g.ID, strings.Replace(g.CRS, ":", "::", 1))
		metersPerUnit := 1.0
		if g.SRID == 4326 {
			metersPerUnit = 111319.4907932736
		}
		for z := 0; z <= maxZoom; z++ {
			scale := g.Resolution(z) * metersPerUnit / 0.00028
			fmt.Fprintf(&b, `<TileMatrix><ows:Identifier>%d</ows:Identifier><ScaleDenominator>%.6f</ScaleDenominator>`, z, scale)
			fmt.Fprintf(&b, `<TopLeftCorner>%g %g</TopLeftCorner>`, g.Bounds[0], g.Bounds[3])
			fmt.Fprintf(&b, `<TileWidth>%d</TileWidth><TileHeight>%d</TileHeight>`, g.TileSize, g.TileSize)
			fmt.Fprintf(&b, `<MatrixWidth>%d</MatrixWidth><MatrixHeight>%d</MatrixHeight></TileMatrix>`, g.MatrixWidth(z), g.MatrixHeight(z))
		}
		fmt.Fprintf(&b, `</TileMatrixSet>`)
	}
	fmt.Fprintf(&b, `</Contents></Capabilities>`)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write([]byte(b.String()))
}

func wmtsException(w http.ResponseWriter, code, msg string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><ExceptionReport version="1.0.0" xmlns="http://www.opengis.net/ows/1.1"><Exception exceptionCode="%s">%s</Exception></ExceptionReport>`,
		xmlEscape(code), xmlEscape(msg))
}
