package format

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"mapserver/internal/models"
)

// WriteCSV 将要素导出为 CSV（几何列输出为 WKT）。
func WriteCSV(w io.Writer, fields []models.Field, features []Feature) error {
	cw := csv.NewWriter(w)
	header := []string{"fid"}
	for _, f := range fields {
		header = append(header, f.Name)
	}
	header = append(header, "geom")
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, feat := range features {
		row := make([]string, 0, len(header))
		row = append(row, fmt.Sprint(feat.ID))
		for _, f := range fields {
			row = append(row, csvValue(feat.Properties[f.Name]))
		}
		row = append(row, GeometryToWKT(feat.Geometry))
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func csvValue(v any) string {
	switch n := v.(type) {
	case nil:
		return ""
	case bool:
		if n {
			return "true"
		}
		return "false"
	case float64:
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

// ParseCSV 解析 CSV：属性列按名称匹配；几何取自 wkt/geom 列或 lon/lat 列。
func ParseCSV(r io.Reader, fields []models.Field) ([]ImportFeature, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV 解析失败: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV 为空")
	}

	header := rows[0]
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(row []string, keys ...string) string {
		for _, k := range keys {
			if i, ok := idx[k]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
		}
		return ""
	}

	out := make([]ImportFeature, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		props := map[string]any{}
		for _, f := range fields {
			i, ok := idx[strings.ToLower(f.Name)]
			if !ok || i >= len(row) {
				continue
			}
			if v := strings.TrimSpace(row[i]); v != "" {
				props[f.Name] = v
			}
		}

		feat := ImportFeature{Properties: props}
		wkt := get(row, "wkt", "geom", "geometry", "the_geom")
		if wkt != "" {
			feat.WKT = wkt
		} else {
			lon := get(row, "lon", "lng", "longitude", "x")
			lat := get(row, "lat", "latitude", "y")
			if lon != "" && lat != "" {
				feat.WKT = fmt.Sprintf("POINT(%s %s)", lon, lat)
			}
		}
		out = append(out, feat)
	}
	return out, nil
}
