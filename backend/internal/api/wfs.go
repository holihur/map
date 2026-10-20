package api

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"mapserver/internal/models"
	"mapserver/internal/store"
)

const wfsVersion = "2.0.0"

// wfs 是极简 WFS 2.0.0：GetCapabilities / DescribeFeatureType / GetFeature（GeoJSON 或 GML）。
func (h *Handlers) wfs(w http.ResponseWriter, r *http.Request) {
	switch strings.ToUpper(r.URL.Query().Get("REQUEST")) {
	case "GETCAPABILITIES", "":
		h.wfsCapabilities(w, r)
	case "DESCRIBEFEATURETYPE":
		h.wfsDescribeFeatureType(w, r)
	case "GETFEATURE":
		h.wfsGetFeature(w, r)
	default:
		owsException(w, "OperationNotSupported", "仅支持 GetCapabilities / DescribeFeatureType / GetFeature")
	}
}

func (h *Handlers) wfsCapabilities(w http.ResponseWriter, r *http.Request) {
	base := requestBaseURL(r)
	layers, err := h.store.ListLayers(r.Context())
	if err != nil {
		owsException(w, "NoApplicableCode", err.Error())
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<wfs:WFS_Capabilities version="%s" xmlns:wfs="http://www.opengis.net/wfs/2.0" xmlns:ows="http://www.opengis.net/ows/1.1" xmlns:fes="http://www.opengis.net/fes/2.0" xmlns:xlink="http://www.w3.org/1999/xlink">`, wfsVersion)
	fmt.Fprintf(&b, `<ows:ServiceIdentification><ows:Title>MapServer WFS</ows:Title><ows:ServiceType>WFS</ows:ServiceType><ows:ServiceTypeVersion>2.0.0</ows:ServiceTypeVersion></ows:ServiceIdentification>`)
	fmt.Fprintf(&b, `<ows:OperationsMetadata>`)
	for _, op := range []string{"GetCapabilities", "DescribeFeatureType", "GetFeature"} {
		fmt.Fprintf(&b, `<ows:Operation name="%s"><ows:DCP><ows:HTTP><ows:Get xlink:href="%s/api/wfs?"/></ows:HTTP></ows:DCP></ows:Operation>`, op, base)
	}
	fmt.Fprintf(&b, `</ows:OperationsMetadata>`)
	fmt.Fprintf(&b, `<wfs:FeatureTypeList>`)
	for _, l := range layers {
		title := l.Title
		if title == "" {
			title = l.Name
		}
		fmt.Fprintf(&b, `<wfs:FeatureType><wfs:Name>%s</wfs:Name><wfs:Title>%s</wfs:Title><wfs:DefaultCRS>urn:ogc:def:crs:EPSG::%d</wfs:DefaultCRS>`,
			xmlEscape(l.Name), xmlEscape(title), l.SRID)
		if l.Bounds != nil {
			fmt.Fprintf(&b, `<ows:WGS84BoundingBox><ows:LowerCorner>%g %g</ows:LowerCorner><ows:UpperCorner>%g %g</ows:UpperCorner></ows:WGS84BoundingBox>`,
				l.Bounds[0], l.Bounds[1], l.Bounds[2], l.Bounds[3])
		}
		fmt.Fprintf(&b, `</wfs:FeatureType>`)
	}
	fmt.Fprintf(&b, `</wfs:FeatureTypeList>`)
	fmt.Fprintf(&b, `<fes:Filter_Capabilities/>`)
	fmt.Fprintf(&b, `</wfs:WFS_Capabilities>`)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func (h *Handlers) wfsDescribeFeatureType(w http.ResponseWriter, r *http.Request) {
	name := firstNonEmpty(
		strings.Split(r.URL.Query().Get("TYPENAMES"), ",")[0],
		strings.Split(r.URL.Query().Get("TYPENAME"), ",")[0],
	)
	name = strings.TrimSpace(name)
	if name == "" {
		owsException(w, "MissingParameterValue", "缺少 TYPENAMES")
		return
	}
	layer, err := h.store.GetLayerByName(r.Context(), name)
	if err != nil {
		owsException(w, "InvalidParameterValue", "图层不存在: "+name)
		return
	}

	// 支持 JSON Schema 输出
	if strings.Contains(strings.ToLower(r.URL.Query().Get("OUTPUTFORMAT")), "json") {
		props := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"fid": map[string]any{"type": "integer"},
			},
		}
		for _, f := range layer.Fields {
			props["properties"].(map[string]any)[f.Name] = map[string]any{"type": jsonSchemaType(f.Type)}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"title":        layer.Name,
			"type":         "object",
			"geometryType": layer.GeometryType,
			"srid":         layer.SRID,
			"schema":       props,
		})
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:gml="http://www.opengis.net/gml/3.2" xmlns:ms="http://mapserver.local/ns" targetNamespace="http://mapserver.local/ns" elementFormDefault="qualified">`)
	fmt.Fprintf(&b, `<xsd:import namespace="http://www.opengis.net/gml/3.2" schemaLocation="http://schemas.opengis.net/gml/3.2.1/gml.xsd"/>`)
	fmt.Fprintf(&b, `<xsd:element name="%s" type="ms:%sType" substitutionGroup="gml:AbstractFeature"/>`, layer.Name, layer.Name)
	fmt.Fprintf(&b, `<xsd:complexType name="%sType"><xsd:complexContent><xsd:extension base="gml:AbstractFeatureType"><xsd:sequence>`, layer.Name)
	fmt.Fprintf(&b, `<xsd:element name="geom" type="gml:GeometryPropertyType" minOccurs="0"/>`)
	for _, f := range layer.Fields {
		fmt.Fprintf(&b, `<xsd:element name="%s" type="%s" minOccurs="0"/>`, xmlEscape(f.Name), xsdType(f.Type))
	}
	fmt.Fprintf(&b, `</xsd:sequence></xsd:extension></xsd:complexContent></xsd:complexType></xsd:schema>`)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func (h *Handlers) wfsGetFeature(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	typeNames := splitComma(firstNonEmpty(q.Get("TYPENAMES"), q.Get("TYPENAME")))
	if len(typeNames) == 0 {
		owsException(w, "MissingParameterValue", "缺少 TYPENAMES")
		return
	}

	outSRID := parseURNCRS(firstNonEmpty(q.Get("SRSNAME"), q.Get("CRS")))
	limit := atoiDefault(firstNonEmpty(q.Get("COUNT"), q.Get("MAXFEATURES")), 1000)
	offset := atoiDefault(q.Get("STARTINDEX"), 0)
	hitsOnly := strings.EqualFold(q.Get("RESULTTYPE"), "hits")

	query := store.FeatureQuery{Limit: limit, Offset: offset, OutSRID: outSRID, BBoxSRID: outSRID}

	if v := q.Get("BBOX"); v != "" {
		parts := splitComma(v)
		if len(parts) >= 4 {
			var b [4]float64
			for i := 0; i < 4; i++ {
				f, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
				if err != nil {
					owsException(w, "InvalidParameterValue", "BBOX 解析失败")
					return
				}
				b[i] = f
			}
			query.BBox = &b
			if len(parts) >= 5 {
				query.BBoxSRID = parseURNCRS(parts[4])
			}
		}
	}

	if filter := q.Get("FILTER"); filter != "" {
		filters, err := parseOGCFilter(filter)
		if err != nil {
			owsException(w, "InvalidParameterValue", err.Error())
			return
		}
		query.Filters = filters
	}

	outFormat := strings.ToLower(q.Get("OUTPUTFORMAT"))
	wantGML := strings.Contains(outFormat, "gml") || strings.Contains(outFormat, "xml")
	if outFormat == "" {
		outFormat = "application/json"
	}

	if len(typeNames) != 1 {
		owsException(w, "OperationNotSupported", "暂仅支持单个 TYPENAMES")
		return
	}
	layer, err := h.store.GetLayerByName(r.Context(), typeNames[0])
	if err != nil {
		owsException(w, "InvalidParameterValue", "图层不存在: "+typeNames[0])
		return
	}

	if wantGML {
		h.wfsGetFeatureGML(w, r, layer, query, hitsOnly, outSRID)
		return
	}

	raw, total, err := h.store.FeaturesGeoJSON(r.Context(), layer, query)
	if err != nil {
		owsException(w, "InvalidParameterValue", err.Error())
		return
	}
	if hitsOnly {
		writeJSON(w, http.StatusOK, map[string]any{"type": "FeatureCollection", "features": []any{}, "total": total, "numberMatched": total, "numberReturned": 0})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-WFS-NumberMatched", strconv.FormatInt(total, 10))
	_, _ = w.Write(raw)
}

func (h *Handlers) wfsGetFeatureGML(w http.ResponseWriter, r *http.Request, layer *models.Layer, query store.FeatureQuery, hitsOnly bool, outSRID int) {
	feats, total, err := h.store.QueryGML(r.Context(), layer, query)
	if err != nil {
		owsException(w, "InvalidParameterValue", err.Error())
		return
	}
	if hitsOnly {
		feats = nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<wfs:FeatureCollection xmlns:wfs="http://www.opengis.net/wfs/2.0" xmlns:gml="http://www.opengis.net/gml/3.2" xmlns:ms="http://mapserver.local/ns" numberMatched="%d" numberReturned="%d">`,
		total, len(feats))
	for _, f := range feats {
		fmt.Fprintf(&b, `<wfs:member><ms:%s gml:id="%s.%d"><ms:geom>%s</ms:geom>`,
			layer.Name, layer.Name, f.ID, f.GML)
		keys := make([]string, 0, len(f.Props))
		for k := range f.Props {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, `<ms:%s>%s</ms:%s>`, xmlEscape(k), xmlEscape(gmlValue(f.Props[k])), xmlEscape(k))
		}
		fmt.Fprintf(&b, `</ms:%s></wfs:member>`, layer.Name)
	}
	fmt.Fprintf(&b, `</wfs:FeatureCollection>`)

	w.Header().Set("Content-Type", "application/gml+xml; charset=utf-8")
	w.Header().Set("X-WFS-NumberMatched", strconv.FormatInt(total, 10))
	_, _ = w.Write([]byte(b.String()))
}

// ── 工具 ────────────────────────────────────────────────────────

type fesNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Content  string     `xml:",chardata"`
	Children []fesNode  `xml:",any"`
}

// attr 按键名（忽略命名空间）取属性值。
func (n fesNode) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// nodeToXML 将解析树重新序列化为 XML（用于 FILTER 二次解析）。
func nodeToXML(n fesNode) string {
	b, err := xml.Marshal(n)
	if err != nil {
		return ""
	}
	return string(b)
}

// parseOGCFilter 解析 OGC/FES Filter：支持相等/不等/大小比较/Like/Between（AND 组合）。
func parseOGCFilter(raw string) ([]store.Filter, error) {
	var root fesNode
	if err := xml.Unmarshal([]byte(raw), &root); err != nil {
		return nil, fmt.Errorf("FILTER 解析失败: %w", err)
	}

	opMap := map[string]string{
		"PropertyIsEqualTo":              "eq",
		"PropertyIsNotEqualTo":           "neq",
		"PropertyIsLessThan":             "lt",
		"PropertyIsGreaterThan":          "gt",
		"PropertyIsLessThanOrEqualTo":    "lte",
		"PropertyIsGreaterThanOrEqualTo": "gte",
		"PropertyIsLike":                 "like",
	}

	var filters []store.Filter
	var walk func(n fesNode)
	walk = func(n fesNode) {
		local := n.XMLName.Local
		if op, ok := opMap[local]; ok {
			field := childText(n, "PropertyName")
			value := childText(n, "Literal")
			if local == "PropertyIsLike" {
				value = strings.Trim(value, "%_")
			}
			if field != "" {
				filters = append(filters, store.Filter{Field: field, Op: op, Value: value})
			}
		} else if local == "PropertyIsBetween" {
			field := childText(n, "PropertyName")
			if lower := firstLiteral(childByName(n, "LowerBoundary")); lower != "" {
				filters = append(filters, store.Filter{Field: field, Op: "gte", Value: lower})
			}
			if upper := firstLiteral(childByName(n, "UpperBoundary")); upper != "" {
				filters = append(filters, store.Filter{Field: field, Op: "lte", Value: upper})
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	if len(filters) == 0 {
		return nil, fmt.Errorf("FILTER 中未找到可识别的比较表达式")
	}
	return filters, nil
}

func childByName(n fesNode, local string) fesNode {
	for _, c := range n.Children {
		if c.XMLName.Local == local {
			return c
		}
	}
	return fesNode{}
}

func childText(n fesNode, local string) string {
	return strings.TrimSpace(childByName(n, local).Content)
}

func firstLiteral(n fesNode) string {
	if n.XMLName.Local == "Literal" {
		return strings.TrimSpace(n.Content)
	}
	for _, c := range n.Children {
		if v := firstLiteral(c); v != "" {
			return v
		}
	}
	return ""
}

func parseURNCRS(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 4326
	}
	if i := strings.LastIndex(v, ":"); i >= 0 {
		v = v[i+1:]
	}
	v = strings.TrimPrefix(v, "EPSG::")
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	return 4326
}

func xsdType(t models.FieldType) string {
	switch t {
	case models.FieldInteger, models.FieldBigInt:
		return "xsd:long"
	case models.FieldDouble:
		return "xsd:double"
	case models.FieldBoolean:
		return "xsd:boolean"
	case models.FieldDate:
		return "xsd:date"
	case models.FieldTimestamp:
		return "xsd:dateTime"
	default:
		return "xsd:string"
	}
}

func jsonSchemaType(t models.FieldType) string {
	switch t {
	case models.FieldInteger, models.FieldBigInt:
		return "integer"
	case models.FieldDouble:
		return "number"
	case models.FieldBoolean:
		return "boolean"
	default:
		return "string"
	}
}

func owsException(w http.ResponseWriter, code, msg string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><ows:ExceptionReport xmlns:ows="http://www.opengis.net/ows/1.1" version="2.0.0"><ows:Exception exceptionCode="%s"><ows:ExceptionText>%s</ows:ExceptionText></ows:Exception></ows:ExceptionReport>`,
		xmlEscape(code), xmlEscape(msg))
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// gmlValue 将 JSON 数值格式化为可读字符串（整数不带科学计数法）。
func gmlValue(v any) string {
	if f, ok := v.(float64); ok {
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}
