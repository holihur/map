package api

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

	"mapserver/internal/models"
	"mapserver/internal/store"
)

// wfsTransaction 处理 WFS-T Transaction（Insert / Update / Delete）。
// 整个请求在单个数据库事务中执行，任一步失败则全部回滚。
func (h *Handlers) wfsTransaction(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		owsException(w, "NoApplicableCode", err.Error())
		return
	}
	var root fesNode
	if err := xml.Unmarshal(body, &root); err != nil {
		owsException(w, "InvalidParameterValue", "Transaction XML 解析失败: "+err.Error())
		return
	}
	if root.XMLName.Local != "Transaction" {
		owsException(w, "InvalidParameterValue", "根元素必须是 wfs:Transaction")
		return
	}

	var inserted, updated, deleted int
	touched := map[string]bool{}

	err = h.store.Transaction(r.Context(), func(tx store.Tx) error {
		for _, action := range root.Children {
			var n int
			var err error
			switch action.XMLName.Local {
			case "Insert":
				n, err = h.wfsInsert(r, tx, action, touched)
				inserted += n
			case "Update":
				n, err = h.wfsUpdate(r, tx, action, touched)
				updated += n
			case "Delete":
				n, err = h.wfsDelete(r, tx, action, touched)
				deleted += n
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		owsException(w, "NoApplicableCode", err.Error())
		return
	}

	// 仅在事务提交后使缓存失效
	for name := range touched {
		h.invalidateLayer(name)
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><wfs:TransactionResponse xmlns:wfs="http://www.opengis.net/wfs/2.0" version="2.0.0"><wfs:TransactionSummary><wfs:totalInserted>%d</wfs:totalInserted><wfs:totalUpdated>%d</wfs:totalUpdated><wfs:totalDeleted>%d</wfs:totalDeleted></wfs:TransactionSummary></wfs:TransactionResponse>`,
		inserted, updated, deleted)
}

func (h *Handlers) wfsInsert(r *http.Request, tx store.Tx, action fesNode, touched map[string]bool) (int, error) {
	count := 0
	for _, feature := range action.Children {
		layerName := feature.XMLName.Local
		layer, err := h.store.GetLayerByName(r.Context(), layerName)
		if err != nil {
			return count, fmt.Errorf("图层不存在: %s", layerName)
		}
		if !h.layerWriteAllowed(r.Context(), layer) {
			return count, fmt.Errorf("没有图层 %s 的写入权限", layerName)
		}
		g, props := featureToGeomProps(layer, feature)
		if _, err := h.store.InsertFeatureGeomTx(r.Context(), tx, layer, g, props); err != nil {
			return count, err
		}
		touched[layer.Name] = true
		count++
	}
	return count, nil
}

func (h *Handlers) wfsUpdate(r *http.Request, tx store.Tx, action fesNode, touched map[string]bool) (int, error) {
	layer, err := h.layerFromTypeName(r, action)
	if err != nil {
		return 0, err
	}
	props := map[string]any{}
	for _, prop := range action.Children {
		if prop.XMLName.Local != "Property" {
			continue
		}
		name := firstNonEmpty(childText(prop, "ValueReference"), childText(prop, "Name"))
		name = stripPrefix(name)
		value := childText(prop, "Value")
		if name != "" {
			props[name] = value
		}
	}
	if len(props) == 0 {
		return 0, fmt.Errorf("Update 中未找到属性值")
	}
	filters, err := filtersOf(action)
	if err != nil {
		return 0, err
	}
	n, err := h.store.UpdateFeaturesTx(r.Context(), tx, layer, props, filters)
	if err != nil {
		return 0, err
	}
	touched[layer.Name] = true
	return int(n), nil
}

func (h *Handlers) wfsDelete(r *http.Request, tx store.Tx, action fesNode, touched map[string]bool) (int, error) {
	layer, err := h.layerFromTypeName(r, action)
	if err != nil {
		return 0, err
	}
	filters, err := filtersOf(action)
	if err != nil {
		return 0, err
	}
	n, err := h.store.DeleteFeaturesTx(r.Context(), tx, layer, filters)
	if err != nil {
		return 0, err
	}
	touched[layer.Name] = true
	return int(n), nil
}

func (h *Handlers) layerFromTypeName(r *http.Request, action fesNode) (*models.Layer, error) {
	typeName := stripPrefix(firstNonEmpty(action.attr("typeName"), action.attr("typeNames")))
	if typeName == "" {
		return nil, fmt.Errorf("缺少 typeName 属性")
	}
	layer, err := h.store.GetLayerByName(r.Context(), typeName)
	if err != nil {
		return nil, fmt.Errorf("图层不存在: %s", typeName)
	}
	if !h.layerWriteAllowed(r.Context(), layer) {
		return nil, fmt.Errorf("没有图层 %s 的写入权限", typeName)
	}
	return layer, nil
}

func filtersOf(action fesNode) ([]store.Filter, error) {
	filter := childByName(action, "Filter")
	if filter.XMLName.Local == "" {
		return nil, fmt.Errorf("缺少 Filter 元素")
	}
	return parseOGCFilter(nodeToXML(filter))
}

// featureToGeomProps 从 WFS-T Insert 的要素节点中提取几何与属性。
func featureToGeomProps(layer *models.Layer, feature fesNode) (store.GeomValue, map[string]any) {
	props := map[string]any{}
	g := store.GeomValue{SRID: layer.SRID}

	fieldNames := map[string]bool{}
	for _, f := range layer.Fields {
		fieldNames[strings.ToLower(f.Name)] = true
	}

	for _, child := range feature.Children {
		name := stripPrefix(child.XMLName.Local)
		switch {
		case name == "geom" || name == "geometry" || name == "the_geom":
			if len(child.Children) > 0 {
				g.WKT = gmlToWKT(child.Children[0])
			}
		case name == "wkt":
			g.WKT = strings.TrimSpace(child.Content)
		case fieldNames[strings.ToLower(name)]:
			if len(child.Children) == 0 {
				props[name] = strings.TrimSpace(child.Content)
			}
		}
	}
	return g, props
}

// ── GML → WKT ───────────────────────────────────────────────────

func gmlToWKT(n fesNode) string {
	switch n.XMLName.Local {
	case "Point":
		if c := gmlPoint(n); c != "" {
			return "POINT(" + c + ")"
		}
	case "LineString":
		if c := gmlPosList(n); c != "" {
			return "LINESTRING(" + c + ")"
		}
	case "LinearRing":
		if c := gmlPosList(n); c != "" {
			return "(" + c + ")"
		}
	case "Polygon":
		var rings []string
		for _, child := range n.Children {
			if child.XMLName.Local == "exterior" || child.XMLName.Local == "interior" {
				ring := childByName(child, "LinearRing")
				if ring.XMLName.Local == "" {
					ring = child
				}
				if c := gmlPosList(ring); c != "" {
					rings = append(rings, "("+c+")")
				}
			}
		}
		if len(rings) > 0 {
			return "POLYGON(" + strings.Join(rings, ",") + ")"
		}
	case "MultiPoint", "MultiCurve", "MultiSurface", "MultiGeometry":
		var parts []string
		for _, child := range n.Children {
			for _, inner := range child.Children {
				if w := gmlToWKT(inner); w != "" {
					parts = append(parts, trimType(w))
				}
			}
			if w := gmlToWKT(child); w != "" && len(child.Children) == 0 {
				parts = append(parts, trimType(w))
			}
		}
		if len(parts) > 0 {
			kind := map[string]string{
				"MultiPoint":   "MULTIPOINT",
				"MultiCurve":   "MULTILINESTRING",
				"MultiSurface": "MULTIPOLYGON",
			}[n.XMLName.Local]
			if kind == "" {
				kind = "GEOMETRYCOLLECTION"
			}
			if kind == "MULTIPOINT" {
				return kind + "(" + strings.Join(parts, ",") + ")"
			}
			return kind + "(" + strings.Join(parts, ",") + ")"
		}
	}
	return ""
}

// trimType 去掉 WKT 的类型前缀，仅保留坐标括号部分。
func trimType(w string) string {
	if i := strings.IndexByte(w, '('); i >= 0 {
		return w[i:]
	}
	return w
}

func gmlPoint(n fesNode) string {
	if c := childText(n, "pos"); c != "" {
		return normalizePos(c)
	}
	if c := childText(n, "coordinates"); c != "" {
		return normalizePos(strings.ReplaceAll(c, ",", " "))
	}
	// 兼容 <gml:coord><gml:X>..</gml:X><gml:Y>..</gml:Y></gml:coord>
	if coord := childByName(n, "coord"); coord.XMLName.Local != "" {
		x := childText(coord, "X")
		y := childText(coord, "Y")
		if x != "" && y != "" {
			return x + " " + y
		}
	}
	return ""
}

func gmlPosList(n fesNode) string {
	if list := childText(n, "posList"); list != "" {
		return normalizePosList(list)
	}
	var pts []string
	for _, child := range n.Children {
		if child.XMLName.Local == "pos" {
			pts = append(pts, normalizePos(strings.TrimSpace(child.Content)))
		}
	}
	if len(pts) > 0 {
		return strings.Join(pts, ",")
	}
	if c := childText(n, "coordinates"); c != "" {
		var pts2 []string
		for _, pair := range strings.FieldsFunc(c, func(r rune) bool { return r == ' ' }) {
			parts := strings.Split(pair, ",")
			if len(parts) >= 2 {
				pts2 = append(pts2, parts[0]+" "+parts[1])
			}
		}
		return strings.Join(pts2, ",")
	}
	return ""
}

// normalizePos 将 "x y" 或 "x,y" 归一化为 "x y"。
func normalizePos(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' || r == '\n' })
	if len(fields) < 2 {
		return ""
	}
	return fields[0] + " " + fields[1]
}

// normalizePosList 将扁平的坐标序列转为 "x y,x y,..."。
func normalizePosList(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) < 4 {
		return ""
	}
	// 判断维度：若数量能被 3 整除且不能被 2 整除则按 3 维处理，否则按 2 维
	dim := 2
	if len(fields)%2 != 0 && len(fields)%3 == 0 {
		dim = 3
	}
	var pts []string
	for i := 0; i+dim <= len(fields); i += dim {
		pts = append(pts, fields[i]+" "+fields[i+1])
	}
	return strings.Join(pts, ",")
}

func stripPrefix(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, ":"); i >= 0 {
		return s[i+1:]
	}
	return s
}
