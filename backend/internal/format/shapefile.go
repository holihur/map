package format

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"mapserver/internal/models"
)

// shapeType 常量。
const (
	shpPoint      = 1
	shpPolyLine   = 3
	shpPolygon    = 5
	shpMultiPoint = 8

	shpPointZ      = 11
	shpPolyLineZ   = 13
	shpPolygonZ    = 15
	shpMultiPointZ = 18
)

type shpShape struct {
	Type   int32
	Points [][2]float64
	Parts  []int32
}

// ── 导出 ────────────────────────────────────────────────────────

// WriteShapefileZip 将要素写为 Shapefile（.shp/.shx/.dbf/.prj）zip 包。
func WriteShapefileZip(w io.Writer, name string, fields []models.Field, prjWKT string, features []Feature) error {
	if len(features) == 0 {
		return fmt.Errorf("没有可导出的要素")
	}

	shapes := make([]shpShape, 0, len(features))
	shapeType := int32(0)
	for _, f := range features {
		s, ok := geometryToShape(f.Geometry)
		if !ok {
			continue
		}
		if shapeType == 0 {
			shapeType = s.Type
		}
		if s.Type != shapeType {
			// 几何类型混用时做降级处理（点视为点，其余按各自类型写入）
		}
		shapes = append(shapes, s)
	}
	if len(shapes) == 0 {
		return fmt.Errorf("没有可导出的几何")
	}

	shp, shx := buildSHP(shapes, shapeType)
	dbf := buildDBF(fields, features)

	zw := zip.NewWriter(w)
	files := map[string][]byte{
		name + ".shp": shp,
		name + ".shx": shx,
		name + ".dbf": dbf,
	}
	if prjWKT != "" {
		files[name+".prj"] = []byte(prjWKT)
	}
	for fn, data := range files {
		fw, err := zw.Create(fn)
		if err != nil {
			return err
		}
		if _, err := fw.Write(data); err != nil {
			return err
		}
	}
	return zw.Close()
}

func geometryToShape(g map[string]any) (shpShape, bool) {
	if g == nil {
		return shpShape{}, false
	}
	typ, _ := g["type"].(string)
	switch typ {
	case "Point":
		c := coord(g["coordinates"])
		if len(c) < 2 {
			return shpShape{}, false
		}
		return shpShape{Type: shpPoint, Points: [][2]float64{{fnum(c[0]), fnum(c[1])}}}, true
	case "MultiPoint":
		cs := coords(g["coordinates"])
		pts := make([][2]float64, 0, len(cs))
		for _, c := range cs {
			if len(c) >= 2 {
				pts = append(pts, [2]float64{fnum(c[0]), fnum(c[1])})
			}
		}
		return shpShape{Type: shpPoint, Points: pts}, len(pts) > 0
	case "LineString":
		return shapeFromLines([][][]any{coords(g["coordinates"])}, shpPolyLine)
	case "MultiLineString":
		return shapeFromLines(coords2(g["coordinates"]), shpPolyLine)
	case "Polygon":
		return shapeFromLines(coords2(g["coordinates"]), shpPolygon)
	case "MultiPolygon":
		rings := [][][]any{}
		for _, poly := range coords3(g["coordinates"]) {
			rings = append(rings, poly...)
		}
		return shapeFromLines(rings, shpPolygon)
	default:
		return shpShape{}, false
	}
}

func shapeFromLines(lines [][][]any, typ int32) (shpShape, bool) {
	s := shpShape{Type: typ}
	for _, line := range lines {
		s.Parts = append(s.Parts, int32(len(s.Points)))
		// Shapefile 规范：外环顺时针、内环逆时针；GeoJSON 相反，故整体反转。
		if typ == shpPolygon {
			line = reverseRing(line)
		}
		for _, c := range line {
			if len(c) >= 2 {
				s.Points = append(s.Points, [2]float64{fnum(c[0]), fnum(c[1])})
			}
		}
	}
	return s, len(s.Points) > 0
}

// reverseRing 返回反转后的环（保持首尾不强制闭合）。
func reverseRing(ring [][]any) [][]any {
	out := make([][]any, 0, len(ring))
	for i := len(ring) - 1; i >= 0; i-- {
		out = append(out, ring[i])
	}
	return out
}

func fnum(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}

func buildSHP(shapes []shpShape, shapeType int32) (shp []byte, shx []byte) {
	bbox := computeBBox(shapes)

	var body bytes.Buffer
	records := make([][2]int32, 0, len(shapes)) // 偏移(字)、长度(字)
	offsetWords := int32(50)
	for i, s := range shapes {
		content := encodeShape(s)
		contentWords := int32(len(content) / 2)
		var rec bytes.Buffer
		_ = binary.Write(&rec, binary.BigEndian, int32(i+1))
		_ = binary.Write(&rec, binary.BigEndian, contentWords)
		rec.Write(content)
		body.Write(rec.Bytes())
		records = append(records, [2]int32{offsetWords, contentWords})
		offsetWords += int32(rec.Len() / 2)
	}

	fileLenWords := 50 + int32(body.Len()/2)
	shp = append(shp, shpHeader(shapeType, fileLenWords, bbox)...)
	shp = append(shp, body.Bytes()...)

	shxLenWords := 50 + int32(len(records))*4
	shx = append(shx, shpHeader(shapeType, shxLenWords, bbox)...)
	for _, r := range records {
		var b8 bytes.Buffer
		_ = binary.Write(&b8, binary.BigEndian, r[0])
		_ = binary.Write(&b8, binary.BigEndian, r[1])
		shx = append(shx, b8.Bytes()...)
	}
	return shp, shx
}

func shpHeader(shapeType, fileLenWords int32, bbox [4]float64) []byte {
	buf := make([]byte, 100)
	binary.BigEndian.PutUint32(buf[0:], 9994)
	binary.BigEndian.PutUint32(buf[24:], uint32(fileLenWords))
	binary.LittleEndian.PutUint32(buf[28:], 1000)
	binary.LittleEndian.PutUint32(buf[32:], uint32(shapeType))
	for i, v := range bbox {
		binary.LittleEndian.PutUint64(buf[36+i*8:], math.Float64bits(v))
	}
	return buf
}

func computeBBox(shapes []shpShape) [4]float64 {
	bbox := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, s := range shapes {
		for _, p := range s.Points {
			bbox[0] = math.Min(bbox[0], p[0])
			bbox[1] = math.Min(bbox[1], p[1])
			bbox[2] = math.Max(bbox[2], p[0])
			bbox[3] = math.Max(bbox[3], p[1])
		}
	}
	if math.IsInf(bbox[0], 1) {
		return [4]float64{0, 0, 0, 0}
	}
	return bbox
}

func encodeShape(s shpShape) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, s.Type)
	switch s.Type {
	case shpPoint:
		if len(s.Points) > 0 {
			writePoint(&b, s.Points[0])
		}
	case shpPolyLine, shpPolygon:
		bbox := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
		for _, p := range s.Points {
			bbox[0] = math.Min(bbox[0], p[0])
			bbox[1] = math.Min(bbox[1], p[1])
			bbox[2] = math.Max(bbox[2], p[0])
			bbox[3] = math.Max(bbox[3], p[1])
		}
		for _, v := range bbox {
			_ = binary.Write(&b, binary.LittleEndian, v)
		}
		_ = binary.Write(&b, binary.LittleEndian, int32(len(s.Parts)))
		_ = binary.Write(&b, binary.LittleEndian, int32(len(s.Points)))
		for _, p := range s.Parts {
			_ = binary.Write(&b, binary.LittleEndian, p)
		}
		for _, p := range s.Points {
			writePoint(&b, p)
		}
	}
	return b.Bytes()
}

func writePoint(b *bytes.Buffer, p [2]float64) {
	_ = binary.Write(b, binary.LittleEndian, p[0])
	_ = binary.Write(b, binary.LittleEndian, p[1])
}

// ── DBF ─────────────────────────────────────────────────────────

type dbfField struct {
	name     string
	typ      byte
	length   int
	decimals int
}

func dbfFieldsOf(fields []models.Field) []dbfField {
	out := make([]dbfField, 0, len(fields))
	for _, f := range fields {
		df := dbfField{name: strings.ToUpper(f.Name), length: 32}
		switch strings.ToLower(string(f.Type)) {
		case "integer", "bigint":
			df.typ, df.length, df.decimals = 'N', 18, 0
		case "double":
			df.typ, df.length, df.decimals = 'N', 18, 6
		case "boolean":
			df.typ, df.length = 'L', 1
		case "date":
			df.typ, df.length = 'D', 8
		case "timestamptz":
			df.typ, df.length = 'C', 24
		default:
			df.typ, df.length = 'C', 254
		}
		if len(df.name) > 10 {
			df.name = df.name[:10]
		}
		out = append(out, df)
	}
	return out
}

func buildDBF(fields []models.Field, features []Feature) []byte {
	defs := dbfFieldsOf(fields)
	headerLen := 32 + 32*len(defs) + 1
	recordLen := 1
	for _, d := range defs {
		recordLen += d.length
	}

	var b bytes.Buffer
	head := make([]byte, 32)
	head[0] = 0x03
	now := time.Now()
	head[1], head[2], head[3] = byte(now.Year()-1900), byte(now.Month()), byte(now.Day())
	binary.LittleEndian.PutUint32(head[4:], uint32(len(features)))
	binary.LittleEndian.PutUint16(head[8:], uint16(headerLen))
	binary.LittleEndian.PutUint16(head[10:], uint16(recordLen))
	b.Write(head)

	for _, d := range defs {
		fd := make([]byte, 32)
		copy(fd[0:11], []byte(d.name))
		fd[11] = d.typ
		fd[16] = byte(d.length)
		fd[17] = byte(d.decimals)
		b.Write(fd)
	}
	b.WriteByte(0x0D)

	for _, feat := range features {
		b.WriteByte(' ') // 未删除
		for _, d := range defs {
			val := dbfEncode(feat.Properties[dbfOriginalName(fields, d.name)], d)
			for len(val) < d.length {
				val = " " + val
			}
			if len(val) > d.length {
				val = val[:d.length]
			}
			b.WriteString(val)
		}
	}
	return b.Bytes()
}

// dbfOriginalName 将截断的字段名映射回原始字段名（尽量匹配）。
func dbfOriginalName(fields []models.Field, dbfName string) string {
	for _, f := range fields {
		if strings.ToUpper(f.Name) == dbfName || strings.HasPrefix(strings.ToUpper(f.Name), dbfName) {
			return f.Name
		}
	}
	return dbfName
}

func dbfEncode(v any, d dbfField) string {
	switch d.typ {
	case 'N':
		if v == nil {
			return ""
		}
		switch n := v.(type) {
		case float64:
			if d.decimals > 0 {
				return strconv.FormatFloat(n, 'f', d.decimals, 64)
			}
			return strconv.FormatInt(int64(n), 10)
		default:
			return fmt.Sprint(v)
		}
	case 'L':
		if b, ok := v.(bool); ok {
			if b {
				return "T"
			}
			return "F"
		}
		return "?"
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

// ── 导入 ────────────────────────────────────────────────────────

// ReadShapefileZip 读取 Shapefile zip 包，返回要素与字段名。
func ReadShapefileZip(r io.Reader) ([]ImportFeature, []string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("zip 解析失败: %w", err)
	}

	var shpBytes, dbfBytes []byte
	for _, f := range zr.File {
		lower := strings.ToLower(f.Name)
		rc, err := f.Open()
		if err != nil {
			return nil, nil, err
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, nil, err
		}
		switch {
		case strings.HasSuffix(lower, ".shp"):
			shpBytes = content
		case strings.HasSuffix(lower, ".dbf"):
			dbfBytes = content
		}
	}
	if shpBytes == nil {
		return nil, nil, fmt.Errorf("zip 中未找到 .shp 文件")
	}

	shapes, err := parseSHP(shpBytes)
	if err != nil {
		return nil, nil, err
	}
	fieldNames, records, err := parseDBF(dbfBytes)
	if err != nil {
		// DBF 可选
		fieldNames, records = nil, nil
	}

	out := make([]ImportFeature, 0, len(shapes))
	for i, s := range shapes {
		props := map[string]any{}
		if i < len(records) {
			for k, v := range records[i] {
				if v != "" {
					props[k] = v
				}
			}
		}
		out = append(out, ImportFeature{Properties: props, WKT: shapeToWKT(s)})
	}
	return out, fieldNames, nil
}

func parseSHP(data []byte) ([]shpShape, error) {
	if len(data) < 100 {
		return nil, fmt.Errorf("SHP 文件过短")
	}
	var shapes []shpShape
	pos := 100
	for pos+8 <= len(data) {
		contentLen := int(binary.BigEndian.Uint32(data[pos+4:])) * 2
		pos += 8
		if pos+contentLen > len(data) {
			break
		}
		content := data[pos : pos+contentLen]
		pos += contentLen
		if len(content) < 4 {
			continue
		}
		s, ok := decodeShape(content)
		if ok {
			shapes = append(shapes, s)
		}
	}
	return shapes, nil
}

func decodeShape(c []byte) (shpShape, bool) {
	typ := int32(binary.LittleEndian.Uint32(c[0:4]))
	switch typ {
	case shpPoint, shpPointZ:
		if len(c) < 20 {
			return shpShape{}, false
		}
		x := math.Float64frombits(binary.LittleEndian.Uint64(c[4:]))
		y := math.Float64frombits(binary.LittleEndian.Uint64(c[12:]))
		return shpShape{Type: shpPoint, Points: [][2]float64{{x, y}}}, true
	case shpPolyLine, shpPolygon, shpPolyLineZ, shpPolygonZ:
		if len(c) < 44 {
			return shpShape{}, false
		}
		numParts := int(int32(binary.LittleEndian.Uint32(c[36:])))
		numPoints := int(int32(binary.LittleEndian.Uint32(c[40:])))
		if numParts < 0 || numPoints < 0 {
			return shpShape{}, false
		}
		base := 44
		if base+numParts*4+numPoints*16 > len(c) {
			return shpShape{}, false
		}
		parts := make([]int32, numParts)
		for i := 0; i < numParts; i++ {
			parts[i] = int32(binary.LittleEndian.Uint32(c[base+i*4:]))
		}
		pbase := base + numParts*4
		points := make([][2]float64, numPoints)
		for i := 0; i < numPoints; i++ {
			x := math.Float64frombits(binary.LittleEndian.Uint64(c[pbase+i*16:]))
			y := math.Float64frombits(binary.LittleEndian.Uint64(c[pbase+i*16+8:]))
			points[i] = [2]float64{x, y}
		}
		t := int32(shpPolyLine)
		if typ == shpPolygon || typ == shpPolygonZ {
			t = shpPolygon
		}
		return shpShape{Type: t, Parts: parts, Points: points}, true
	case shpMultiPoint, shpMultiPointZ:
		if len(c) < 40 {
			return shpShape{}, false
		}
		numPoints := int(int32(binary.LittleEndian.Uint32(c[36:])))
		if numPoints < 0 || 40+numPoints*16 > len(c) {
			return shpShape{}, false
		}
		points := make([][2]float64, numPoints)
		for i := 0; i < numPoints; i++ {
			x := math.Float64frombits(binary.LittleEndian.Uint64(c[40+i*16:]))
			y := math.Float64frombits(binary.LittleEndian.Uint64(c[40+i*16+8:]))
			points[i] = [2]float64{x, y}
		}
		return shpShape{Type: shpPoint, Points: points}, true
	default:
		return shpShape{}, false
	}
}

func shapeToWKT(s shpShape) string {
	pt := func(p [2]float64) string {
		return strconv.FormatFloat(p[0], 'f', -1, 64) + " " + strconv.FormatFloat(p[1], 'f', -1, 64)
	}
	switch s.Type {
	case shpPoint:
		if len(s.Points) == 1 {
			return "POINT(" + pt(s.Points[0]) + ")"
		}
		parts := make([]string, 0, len(s.Points))
		for _, p := range s.Points {
			parts = append(parts, "("+pt(p)+")")
		}
		return "MULTIPOINT(" + strings.Join(parts, ",") + ")"
	case shpPolyLine:
		rings := splitParts(s)
		if len(rings) == 1 {
			return "LINESTRING(" + strings.Join(rings[0], ",") + ")"
		}
		wrapped := make([]string, 0, len(rings))
		for _, r := range rings {
			wrapped = append(wrapped, "("+strings.Join(r, ",")+")")
		}
		return "MULTILINESTRING(" + strings.Join(wrapped, ",") + ")"
	default: // polygon：按环方向分组为多边形（外环顺时针 → 有向面积 < 0）
		return polygonRingsToWKT(splitParts(s))
	}
}

// polygonRingsToWKT 依据环的走向将环分组：
// 顺时针视为外环，逆时针视为内环（孔洞），组合为 POLYGON / MULTIPOLYGON。
func polygonRingsToWKT(rings [][]string) string {
	type poly struct {
		outer string
		holes []string
	}
	var polys []*poly
	for _, ring := range rings {
		if len(ring) < 3 {
			continue
		}
		wktRing := "(" + strings.Join(ring, ",") + ")"
		if signedArea(ring) <= 0 { // 顺时针 → 外环
			polys = append(polys, &poly{outer: wktRing})
		} else if len(polys) > 0 {
			polys[len(polys)-1].holes = append(polys[len(polys)-1].holes, wktRing)
		} else {
			// 首个环是逆时针时也当作外环，避免丢弃数据
			polys = append(polys, &poly{outer: wktRing})
		}
	}
	if len(polys) == 0 {
		return ""
	}
	if len(polys) == 1 {
		return "POLYGON(" + polys[0].outer + strings.Join(polys[0].holes, "") + ")"
	}
	parts := make([]string, 0, len(polys))
	for _, p := range polys {
		parts = append(parts, "("+p.outer+strings.Join(p.holes, "")+")")
	}
	return "MULTIPOLYGON(" + strings.Join(parts, ",") + ")"
}

// signedArea 用鞋带公式计算环的有向面积（正=逆时针）。
func signedArea(ring []string) float64 {
	var sum float64
	n := len(ring)
	for i := 0; i < n; i++ {
		x1, y1, ok1 := parseXY(ring[i])
		x2, y2, ok2 := parseXY(ring[(i+1)%n])
		if !ok1 || !ok2 {
			continue
		}
		sum += x1*y2 - x2*y1
	}
	return sum / 2
}

func parseXY(s string) (float64, float64, bool) {
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return 0, 0, false
	}
	x, err1 := strconv.ParseFloat(parts[0], 64)
	y, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return x, y, true
}

func splitParts(s shpShape) [][]string {
	if len(s.Parts) == 0 {
		return nil
	}
	out := make([][]string, 0, len(s.Parts))
	for i, start := range s.Parts {
		end := int32(len(s.Points))
		if i+1 < len(s.Parts) {
			end = s.Parts[i+1]
		}
		line := make([]string, 0, end-start)
		for j := start; j < end && int(j) < len(s.Points); j++ {
			p := s.Points[j]
			line = append(line, strconv.FormatFloat(p[0], 'f', -1, 64)+" "+strconv.FormatFloat(p[1], 'f', -1, 64))
		}
		out = append(out, line)
	}
	return out
}

func parseDBF(data []byte) ([]string, []map[string]string, error) {
	if len(data) < 32 {
		return nil, nil, fmt.Errorf("DBF 文件过短")
	}
	numRecords := int(binary.LittleEndian.Uint32(data[4:]))
	headerLen := int(binary.LittleEndian.Uint16(data[8:]))
	recordLen := int(binary.LittleEndian.Uint16(data[10:]))
	if headerLen < 32 || headerLen > len(data) {
		return nil, nil, fmt.Errorf("DBF 头非法")
	}

	type field struct {
		name   string
		length int
		offset int
	}
	var fields []field
	off := 1 // 跳过删除标记
	for p := 32; p+32 <= headerLen-1; p += 32 {
		if data[p] == 0x0D {
			break
		}
		name := strings.TrimRight(string(data[p:p+11]), "\x00")
		length := int(data[p+16])
		fields = append(fields, field{name: name, length: length, offset: off})
		off += length
	}

	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.name)
	}

	records := make([]map[string]string, 0, numRecords)
	for i := 0; i < numRecords; i++ {
		start := headerLen + i*recordLen
		if start+recordLen > len(data) {
			break
		}
		if data[start] == '*' { // 已删除
			records = append(records, map[string]string{})
			continue
		}
		rec := map[string]string{}
		for _, f := range fields {
			s, e := start+f.offset, start+f.offset+f.length
			if e > len(data) {
				e = len(data)
			}
			rec[f.name] = strings.TrimSpace(string(data[s:e]))
		}
		records = append(records, rec)
	}
	return names, records, nil
}
