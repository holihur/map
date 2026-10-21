package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"mapserver/internal/models"
)

// Filter 表示一个属性过滤条件。
type Filter struct {
	Field string
	Op    string // eq, neq, gt, gte, lt, lte, like, notnull, isnull
	Value string
}

// FeatureQuery 描述要素查询参数。
type FeatureQuery struct {
	BBox       *[4]float64 // minx,miny,maxx,maxy
	BBoxSRID   int         // BBox 所在坐标系，默认 4326
	Filters    []Filter
	IDs        []int64 // 只返回指定 fid
	TextSearch string  // 在所有 text 字段中模糊搜索
	Limit      int
	Offset     int
	OutSRID    int // 输出坐标系，默认 4326
	Simplify   float64
	OrderBy    string
	OrderDesc  bool
}

type whereBuilder struct {
	conds []string
	args  []any
	n     int
}

func (w *whereBuilder) arg(v any) string {
	w.args = append(w.args, v)
	w.n++
	return "$" + strconv.Itoa(w.n)
}

func (w *whereBuilder) add(cond string) { w.conds = append(w.conds, cond) }

// buildWhere 根据查询参数构造 WHERE 子句（不含 WHERE 关键字）。
func buildWhere(l *models.Layer, q *FeatureQuery, w *whereBuilder) error {
	if q.BBox != nil {
		b := q.BBox
		bboxSRID := q.BBoxSRID
		if bboxSRID == 0 {
			bboxSRID = 4326
		}
		p1 := w.arg(b[0])
		p2 := w.arg(b[1])
		p3 := w.arg(b[2])
		p4 := w.arg(b[3])
		w.add(fmt.Sprintf(
			"t.geom IS NOT NULL AND t.geom && ST_Transform(ST_MakeEnvelope(%s,%s,%s,%s,%d), %d)",
			p1, p2, p3, p4, bboxSRID, l.SRID))
	}

	if len(q.IDs) > 0 {
		w.add("t.fid = ANY(" + w.arg(q.IDs) + ")")
	}

	if term := strings.TrimSpace(q.TextSearch); term != "" {
		ors := make([]string, 0)
		for _, f := range l.Fields {
			if f.Type == models.FieldText {
				ors = append(ors, "t."+quoteIdent(strings.ToLower(f.Name))+" ILIKE "+w.arg("%"+term+"%"))
			}
		}
		if len(ors) > 0 {
			w.add("(" + strings.Join(ors, " OR ") + ")")
		}
	}

	fieldTypes := map[string]models.FieldType{}
	for _, f := range l.Fields {
		fieldTypes[strings.ToLower(f.Name)] = f.Type
	}

	for _, f := range q.Filters {
		name := strings.ToLower(f.Field)
		ft, ok := fieldTypes[name]
		if !ok {
			return fmt.Errorf("unknown filter field %q", f.Field)
		}
		col := "t." + quoteIdent(name)
		switch strings.ToLower(f.Op) {
		case "", "eq":
			w.add(col + " = " + w.arg(mustConvert(ft, f.Value)))
		case "neq":
			w.add(col + " <> " + w.arg(mustConvert(ft, f.Value)))
		case "gt":
			w.add(col + " > " + w.arg(mustConvert(ft, f.Value)))
		case "gte":
			w.add(col + " >= " + w.arg(mustConvert(ft, f.Value)))
		case "lt":
			w.add(col + " < " + w.arg(mustConvert(ft, f.Value)))
		case "lte":
			w.add(col + " <= " + w.arg(mustConvert(ft, f.Value)))
		case "like":
			w.add(col + " ILIKE " + w.arg("%"+f.Value+"%"))
		case "in":
			vals := strings.Split(f.Value, ",")
			ors := make([]string, 0, len(vals))
			for _, v := range vals {
				ors = append(ors, col+" = "+w.arg(mustConvert(ft, strings.TrimSpace(v))))
			}
			if len(ors) > 0 {
				w.add("(" + strings.Join(ors, " OR ") + ")")
			}
		case "notnull":
			w.add(col + " IS NOT NULL")
		case "isnull":
			w.add(col + " IS NULL")
		default:
			return fmt.Errorf("unsupported operator %q", f.Op)
		}
	}
	return nil
}

// mustConvert 尽量把字符串值转换为字段类型；失败时回退为字符串。
func mustConvert(ft models.FieldType, v string) any {
	if val, err := convertValue(ft, v); err == nil {
		return val
	}
	return v
}

func convertValue(ft models.FieldType, v string) (any, error) {
	switch ft {
	case models.FieldInteger, models.FieldBigInt:
		return strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	case models.FieldDouble:
		return strconv.ParseFloat(strings.TrimSpace(v), 64)
	case models.FieldBoolean:
		return strconv.ParseBool(strings.TrimSpace(v))
	default:
		return v, nil
	}
}

func propsObjectExpr(l *models.Layer) string {
	if len(l.Fields) == 0 {
		return "'{}'::jsonb"
	}
	parts := make([]string, 0, len(l.Fields)*2)
	for _, f := range l.Fields {
		key := "'" + strings.ReplaceAll(f.Name, "'", "''") + "'"
		parts = append(parts, key, "t."+quoteIdent(strings.ToLower(f.Name)))
	}
	return "jsonb_build_object(" + strings.Join(parts, ", ") + ")"
}

func geomExpr(outSRID int, simplify float64, outPlaceholder string) string {
	inner := "t.geom"
	if simplify > 0 {
		inner = fmt.Sprintf("ST_SimplifyPreserveTopology(t.geom, %g)", simplify)
	}
	return fmt.Sprintf("(ST_AsGeoJSON(ST_Transform(%s, %s::int))::jsonb - 'crs')", inner, outPlaceholder)
}

// geomFromGeoJSONExpr 生成从 GeoJSON 到图层坐标系的表达式。
// inputSRID 为 GeoJSON 坐标的参考系（GeoJSON 规范默认 4326）。
func geomFromGeoJSONExpr(placeholder string, inputSRID, layerSRID int) string {
	if inputSRID <= 0 {
		inputSRID = 4326
	}
	if layerSRID <= 0 {
		layerSRID = 4326
	}
	expr := fmt.Sprintf("ST_SetSRID(ST_GeomFromGeoJSON(%s), %d)", placeholder, inputSRID)
	if inputSRID == layerSRID {
		return expr
	}
	return fmt.Sprintf("ST_Transform(%s, %d)", expr, layerSRID)
}

// FeaturesGeoJSON 查询要素并返回 GeoJSON FeatureCollection。
func (s *Store) FeaturesGeoJSON(ctx context.Context, l *models.Layer, q FeatureQuery) (json.RawMessage, int64, error) {
	table := quoteIdent(l.TableName)
	if !validIdent(l.TableName) {
		return nil, 0, fmt.Errorf("invalid table name")
	}
	if q.OutSRID == 0 {
		q.OutSRID = 4326
	}
	if q.Limit <= 0 || q.Limit > 100000 {
		q.Limit = 1000
	}

	// 总数
	cw := &whereBuilder{}
	if err := buildWhere(l, &q, cw); err != nil {
		return nil, 0, err
	}
	countSQL := "SELECT count(*) FROM " + table + " t"
	if len(cw.conds) > 0 {
		countSQL += " WHERE " + strings.Join(cw.conds, " AND ")
	}
	var total int64
	if err := s.pool.QueryRow(ctx, countSQL, cw.args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	w := &whereBuilder{}
	outPh := w.arg(q.OutSRID)
	if err := buildWhere(l, &q, w); err != nil {
		return nil, 0, err
	}

	order := "t.fid"
	if q.OrderBy != "" {
		field := strings.ToLower(q.OrderBy)
		for _, f := range l.Fields {
			if strings.ToLower(f.Name) == field {
				order = "t." + quoteIdent(field)
				break
			}
		}
	}
	if q.OrderDesc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	limitPh := w.arg(q.Limit)
	offsetPh := w.arg(q.Offset)

	sql := fmt.Sprintf(`
		SELECT COALESCE(jsonb_agg(sub.f ORDER BY sub.fid), '[]'::jsonb)
		FROM (
			SELECT t.fid AS fid, jsonb_build_object(
				'type', 'Feature',
				'id', t.fid,
				'geometry', %s,
				'properties', %s
			) AS f
			FROM %s t
			%s
			ORDER BY %s
			LIMIT %s OFFSET %s
		) sub`,
		geomExpr(q.OutSRID, q.Simplify, outPh),
		propsObjectExpr(l),
		table,
		whereClause(w.conds),
		order, limitPh, offsetPh)

	var features json.RawMessage
	if err := s.pool.QueryRow(ctx, sql, w.args...).Scan(&features); err != nil {
		return nil, 0, err
	}

	fc := struct {
		Type     string          `json:"type"`
		CRS      map[string]any  `json:"crs"`
		Features json.RawMessage `json:"features"`
		Total    int64           `json:"total"`
	}{
		Type:     "FeatureCollection",
		CRS:      map[string]any{"type": "name", "properties": map[string]any{"name": "EPSG:" + strconv.Itoa(q.OutSRID)}},
		Features: features,
		Total:    total,
	}

	out, err := json.Marshal(fc)
	return out, total, err
}

// GetFeature 返回单个要素。
func (s *Store) GetFeature(ctx context.Context, l *models.Layer, fid int64, outSRID int) (json.RawMessage, error) {
	if outSRID == 0 {
		outSRID = 4326
	}
	w := &whereBuilder{}
	outPh := w.arg(outSRID)
	fidPh := w.arg(fid)
	sql := fmt.Sprintf(`
		SELECT jsonb_build_object(
			'type','Feature','id',t.fid,
			'geometry', %s,
			'properties', %s
		)
		FROM %s t WHERE t.fid = %s`,
		geomExpr(outSRID, 0, outPh), propsObjectExpr(l), quoteIdent(l.TableName), fidPh)

	var feat json.RawMessage
	if err := s.pool.QueryRow(ctx, sql, w.args...).Scan(&feat); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return feat, nil
}

// propValue 大小写不敏感地取属性值（兼容 Shapefile/DBF 大写列名等）。
func propValue(props map[string]any, name string) any {
	if props == nil {
		return nil
	}
	if v, ok := props[name]; ok {
		return v
	}
	for k, v := range props {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return nil
}

// normalize 将 JSON 解码得到的值转换为适配列类型的值。
func normalize(ft models.FieldType, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch ft {
	case models.FieldInteger, models.FieldBigInt:
		switch n := v.(type) {
		case float64:
			return int64(n), nil
		case int64:
			return n, nil
		case int:
			return int64(n), nil
		case json.Number:
			return n.Int64()
		case string:
			return strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		}
	case models.FieldDouble:
		switch n := v.(type) {
		case float64:
			return n, nil
		case int64:
			return float64(n), nil
		case json.Number:
			return n.Float64()
		case string:
			return strconv.ParseFloat(strings.TrimSpace(n), 64)
		}
	case models.FieldBoolean:
		switch b := v.(type) {
		case bool:
			return b, nil
		case int64:
			return b != 0, nil
		case int:
			return b != 0, nil
		case float64:
			return b != 0, nil
		case string:
			s := strings.TrimSpace(b)
			switch {
			case s == "1", strings.EqualFold(s, "t"), strings.EqualFold(s, "true"):
				return true, nil
			case s == "0", strings.EqualFold(s, "f"), strings.EqualFold(s, "false"):
				return false, nil
			}
			return strconv.ParseBool(s)
		}
	case models.FieldTimestamp, models.FieldDate:
		if sv, ok := v.(string); ok {
			return sv, nil
		}
	case models.FieldText:
		if sv, ok := v.(string); ok {
			return sv, nil
		}
		return fmt.Sprint(v), nil
	}
	return nil, fmt.Errorf("无法将 %v 转换为字段类型 %s", v, ft)
}

// GeomValue 表示待写入的几何（GeoJSON / WKT / WKB 三选一）。
type GeomValue struct {
	GeoJSON []byte
	WKT     string
	WKB     []byte
	SRID    int // 输入坐标系，默认 4326
}

// geomValueExpr 生成将 GeomValue 转换到图层坐标系的 SQL 表达式。
func geomValueExpr(placeholder string, g GeomValue, layerSRID int) string {
	srid := g.SRID
	if srid <= 0 {
		srid = 4326
	}
	if layerSRID <= 0 {
		layerSRID = 4326
	}
	var base string
	switch {
	case len(g.WKB) > 0:
		base = fmt.Sprintf("ST_SetSRID(ST_GeomFromWKB(%s), %d)", placeholder, srid)
	case g.WKT != "":
		base = fmt.Sprintf("ST_SetSRID(ST_GeomFromText(%s), %d)", placeholder, srid)
	default:
		base = fmt.Sprintf("ST_SetSRID(ST_GeomFromGeoJSON(%s), %d)", placeholder, srid)
	}
	if srid == layerSRID {
		return base
	}
	return fmt.Sprintf("ST_Transform(%s, %d)", base, layerSRID)
}

// InsertFeatureGeom 插入一个要素（几何可为 GeoJSON / WKT / WKB）。
func (s *Store) InsertFeatureGeom(ctx context.Context, l *models.Layer, g GeomValue, props map[string]any) (int64, error) {
	return insertFeatureGeom(ctx, s.pool, l, g, props)
}

// InsertFeatureGeomTx 在事务中插入要素。
func (s *Store) InsertFeatureGeomTx(ctx context.Context, tx pgx.Tx, l *models.Layer, g GeomValue, props map[string]any) (int64, error) {
	return insertFeatureGeom(ctx, tx, l, g, props)
}

func insertFeatureGeom(ctx context.Context, q execer, l *models.Layer, g GeomValue, props map[string]any) (int64, error) {
	cols := make([]string, 0, len(l.Fields))
	places := make([]string, 0, len(l.Fields)+1)
	args := make([]any, 0, len(l.Fields)+1)
	for _, f := range l.Fields {
		val, err := normalize(f.Type, propValue(props, f.Name))
		if err != nil {
			return 0, fmt.Errorf("字段 %s: %w", f.Name, err)
		}
		args = append(args, val)
		cols = append(cols, quoteIdent(strings.ToLower(f.Name)))
		places = append(places, "$"+strconv.Itoa(len(args)))
	}

	var geomArg any
	switch {
	case len(g.WKB) > 0:
		geomArg = g.WKB
	case g.WKT != "":
		geomArg = g.WKT
	default:
		if len(g.GeoJSON) > 0 && string(g.GeoJSON) != "null" {
			geomArg = string(g.GeoJSON)
		}
	}
	args = append(args, geomArg)
	geomPh := "$" + strconv.Itoa(len(args))
	cols = append(cols, "geom")
	places = append(places, geomValueExpr(geomPh, g, l.SRID))

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING fid",
		quoteIdent(l.TableName), strings.Join(cols, ", "), strings.Join(places, ", "))

	var fid int64
	if err := q.QueryRow(ctx, sql, args...).Scan(&fid); err != nil {
		return 0, err
	}
	return fid, nil
}

// InsertFeature 插入一个要素（GeoJSON），返回 fid。inputSRID 为输入几何的坐标系（默认 4326）。
func (s *Store) InsertFeature(ctx context.Context, l *models.Layer, geom json.RawMessage, props map[string]any, inputSRID int) (int64, error) {
	return s.InsertFeatureGeom(ctx, l, GeomValue{GeoJSON: geom, SRID: inputSRID}, props)
}

// UpdateFeature 更新一个要素的属性与几何。inputSRID 为输入几何的坐标系（默认 4326）。
func (s *Store) UpdateFeature(ctx context.Context, l *models.Layer, fid int64, geom json.RawMessage, props map[string]any, inputSRID int) error {
	sets := make([]string, 0, len(l.Fields)+2)
	args := make([]any, 0, len(l.Fields)+2)
	for _, f := range l.Fields {
		val, err := normalize(f.Type, propValue(props, f.Name))
		if err != nil {
			return fmt.Errorf("字段 %s: %w", f.Name, err)
		}
		args = append(args, val)
		sets = append(sets, quoteIdent(strings.ToLower(f.Name))+" = $"+strconv.Itoa(len(args)))
	}
	if geom != nil {
		args = append(args, string(geom))
		sets = append(sets, fmt.Sprintf("geom = %s", geomFromGeoJSONExpr("$"+strconv.Itoa(len(args)), inputSRID, l.SRID)))
	}
	sets = append(sets, "updated_at = now()")
	args = append(args, fid)

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE fid = $%d",
		quoteIdent(l.TableName), strings.Join(sets, ", "), len(args))

	tag, err := s.pool.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteFeature 删除一个要素。
func (s *Store) DeleteFeature(ctx context.Context, l *models.Layer, fid int64) error {
	tag, err := s.pool.Exec(ctx,
		"DELETE FROM "+quoteIdent(l.TableName)+" WHERE fid=$1", fid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// TileMVT 生成指定瓦片的 Mapbox Vector Tile。
// gridSRID 为瓦片矩阵集的坐标系（通常 3857 或 4326），env 为瓦片在该坐标系下的范围。
func (s *Store) TileMVT(ctx context.Context, l *models.Layer, extent, gridSRID, zoom, maxFeatures int, simplify bool, env [4]float64) ([]byte, error) {
	if !validIdent(l.TableName) {
		return nil, fmt.Errorf("invalid table name")
	}
	if extent <= 0 {
		extent = 4096
	}
	if gridSRID == 0 {
		gridSRID = 3857
	}

	cols := []string{"t.fid AS fid"}
	for _, f := range l.Fields {
		c := quoteIdent(strings.ToLower(f.Name))
		cols = append(cols, "t."+c+" AS "+c)
	}
	colList := strings.Join(cols, ", ")

	// 低缩放级别：按瓦片像素（extent）的一半作为简化容差，避免把全量节点编码进瓦片。
	width := env[2] - env[0]
	if width <= 0 {
		width = 1
	}
	tolerance := 0.0
	if simplify {
		tolerance = width / float64(extent) * 0.5
	}

	// 每个瓦片最多编码 maxFeatures 个要素；低缩放级别进一步收紧。
	if maxFeatures <= 0 {
		maxFeatures = 5000
	}
	if zoom <= 4 && maxFeatures > 1500 {
		maxFeatures = 1500
	}

	sql := fmt.Sprintf(`
		WITH bounds AS (
			SELECT ST_MakeEnvelope($1, $2, $3, $4, %d) AS geom
		), src AS (
			SELECT %s,
				ST_Transform(t.geom, %d) AS g
			FROM %s t, bounds
			WHERE t.geom IS NOT NULL
			  AND t.geom && ST_Transform(bounds.geom, %d)
			ORDER BY ST_Area(ST_Transform(t.geom, %d)) DESC, ST_NPoints(t.geom) DESC
			LIMIT %d
		), mvt AS (
			SELECT %s,
				ST_AsMVTGeom(ST_SimplifyPreserveTopology(g, %g), bounds.geom, $5::int, 64, true) AS geom
			FROM src t, bounds
		)
		SELECT COALESCE(ST_AsMVT(mvt.*, $6::text, $5::int, 'geom', 'fid'), ''::bytea) FROM mvt`,
		gridSRID, colList, gridSRID, quoteIdent(l.TableName), l.SRID, gridSRID, maxFeatures,
		colList, tolerance)

	var data []byte
	if err := s.pool.QueryRow(ctx, sql, env[0], env[1], env[2], env[3], extent, l.Name).Scan(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// DeleteFeatures 按过滤器删除要素，返回删除数量。
func (s *Store) DeleteFeatures(ctx context.Context, l *models.Layer, filters []Filter) (int64, error) {
	return deleteFeatures(ctx, s.pool, l, filters)
}

// DeleteFeaturesTx 在事务中按过滤器删除要素。
func (s *Store) DeleteFeaturesTx(ctx context.Context, tx pgx.Tx, l *models.Layer, filters []Filter) (int64, error) {
	return deleteFeatures(ctx, tx, l, filters)
}

func deleteFeatures(ctx context.Context, q execer, l *models.Layer, filters []Filter) (int64, error) {
	if !validIdent(l.TableName) {
		return 0, fmt.Errorf("invalid table name")
	}
	w := &whereBuilder{}
	if err := buildWhere(l, &FeatureQuery{Filters: filters}, w); err != nil {
		return 0, err
	}
	sql := "DELETE FROM " + quoteIdent(l.TableName) + " t " + whereClause(w.conds)
	tag, err := q.Exec(ctx, sql, w.args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// UpdateFeatures 按过滤器更新要素属性，返回更新数量。
func (s *Store) UpdateFeatures(ctx context.Context, l *models.Layer, props map[string]any, filters []Filter) (int64, error) {
	return updateFeatures(ctx, s.pool, l, props, filters)
}

// UpdateFeaturesTx 在事务中按过滤器更新要素属性。
func (s *Store) UpdateFeaturesTx(ctx context.Context, tx pgx.Tx, l *models.Layer, props map[string]any, filters []Filter) (int64, error) {
	return updateFeatures(ctx, tx, l, props, filters)
}

func updateFeatures(ctx context.Context, q execer, l *models.Layer, props map[string]any, filters []Filter) (int64, error) {
	if !validIdent(l.TableName) {
		return 0, fmt.Errorf("invalid table name")
	}
	if len(props) == 0 {
		return 0, fmt.Errorf("没有要更新的属性")
	}

	w := &whereBuilder{}
	if err := buildWhere(l, &FeatureQuery{Filters: filters}, w); err != nil {
		return 0, err
	}

	sets := make([]string, 0, len(props)+1)
	for _, f := range l.Fields {
		v, ok := props[f.Name]
		if !ok {
			continue
		}
		val, err := normalize(f.Type, v)
		if err != nil {
			return 0, fmt.Errorf("字段 %s: %w", f.Name, err)
		}
		sets = append(sets, quoteIdent(strings.ToLower(f.Name))+" = "+w.arg(val))
	}
	if len(sets) == 0 {
		return 0, fmt.Errorf("属性中没有匹配的字段")
	}
	sets = append(sets, "updated_at = now()")

	sql := "UPDATE " + quoteIdent(l.TableName) + " t SET " + strings.Join(sets, ", ") + " " + whereClause(w.conds)
	tag, err := q.Exec(ctx, sql, w.args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// LayerBounds 计算图层数据的空间范围（EPSG:4326）。
func (s *Store) LayerBounds(ctx context.Context, l *models.Layer) (*[4]float64, error) {
	sql := fmt.Sprintf(
		"SELECT ST_XMin(e), ST_YMin(e), ST_XMax(e), ST_YMax(e) FROM (SELECT ST_Extent(ST_Transform(geom, 4326)) AS e FROM %s) s",
		quoteIdent(l.TableName))
	var minx, miny, maxx, maxy *float64
	if err := s.pool.QueryRow(ctx, sql).Scan(&minx, &miny, &maxx, &maxy); err != nil {
		return nil, err
	}
	if minx == nil {
		return nil, nil
	}
	return &[4]float64{*minx, *miny, *maxx, *maxy}, nil
}

func whereClause(conds []string) string {
	if len(conds) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(conds, " AND ")
}

// DeleteFeaturesWhere 按完整查询条件（ids / bbox / filters / text）删除要素。
func (s *Store) DeleteFeaturesWhere(ctx context.Context, l *models.Layer, q FeatureQuery) (int64, error) {
	if !validIdent(l.TableName) {
		return 0, fmt.Errorf("invalid table name")
	}
	w := &whereBuilder{}
	if err := buildWhere(l, &q, w); err != nil {
		return 0, err
	}
	if len(w.conds) == 0 {
		return 0, fmt.Errorf("拒绝删除全部要素：请提供筛选条件")
	}
	sql := "DELETE FROM " + quoteIdent(l.TableName) + " t " + whereClause(w.conds)
	tag, err := s.pool.Exec(ctx, sql, w.args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// UpdateFeaturesWhere 按完整查询条件批量更新属性。
func (s *Store) UpdateFeaturesWhere(ctx context.Context, l *models.Layer, q FeatureQuery, props map[string]any) (int64, error) {
	if !validIdent(l.TableName) {
		return 0, fmt.Errorf("invalid table name")
	}
	if len(props) == 0 {
		return 0, fmt.Errorf("没有要更新的属性")
	}
	w := &whereBuilder{}
	if err := buildWhere(l, &q, w); err != nil {
		return 0, err
	}
	if len(w.conds) == 0 {
		return 0, fmt.Errorf("拒绝更新全部要素：请提供筛选条件")
	}
	sets := make([]string, 0, len(props)+1)
	for _, f := range l.Fields {
		v, ok := props[f.Name]
		if !ok {
			continue
		}
		val, err := normalize(f.Type, v)
		if err != nil {
			return 0, fmt.Errorf("字段 %s: %w", f.Name, err)
		}
		sets = append(sets, quoteIdent(strings.ToLower(f.Name))+" = "+w.arg(val))
	}
	if len(sets) == 0 {
		return 0, fmt.Errorf("属性中没有匹配的字段")
	}
	sets = append(sets, "updated_at = now()")
	sql := "UPDATE " + quoteIdent(l.TableName) + " t SET " + strings.Join(sets, ", ") + " " + whereClause(w.conds)
	tag, err := s.pool.Exec(ctx, sql, w.args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// FieldStat 描述一个属性字段的统计信息（用于样式分级 / 数据概览）。
type FieldStat struct {
	Field    string       `json:"field"`
	Type     string       `json:"type"`
	Count    int64        `json:"count"`
	Nulls    int64        `json:"nulls"`
	Distinct int64        `json:"distinct"`
	Min      any          `json:"min,omitempty"`
	Max      any          `json:"max,omitempty"`
	Top      []ValueCount `json:"top,omitempty"`
}

// ValueCount 一个取值及其出现次数。
type ValueCount struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// FieldStats 计算图层各字段的统计信息。
func (s *Store) FieldStats(ctx context.Context, l *models.Layer) ([]FieldStat, error) {
	if !validIdent(l.TableName) {
		return nil, fmt.Errorf("invalid table name")
	}
	out := make([]FieldStat, 0, len(l.Fields))
	for _, f := range l.Fields {
		col := quoteIdent(strings.ToLower(f.Name))
		st := FieldStat{Field: f.Name, Type: string(f.Type)}
		row := s.pool.QueryRow(ctx, fmt.Sprintf(
			`SELECT count(*), count(*) FILTER (WHERE %s IS NULL), count(DISTINCT %s) FROM %s`,
			col, col, quoteIdent(l.TableName)))
		if err := row.Scan(&st.Count, &st.Nulls, &st.Distinct); err != nil {
			return nil, err
		}
		switch f.Type {
		case models.FieldInteger, models.FieldBigInt, models.FieldDouble, models.FieldTimestamp, models.FieldDate:
			var minv, maxv any
			_ = s.pool.QueryRow(ctx, fmt.Sprintf(
				`SELECT min(%s), max(%s) FROM %s`, col, col, quoteIdent(l.TableName))).Scan(&minv, &maxv)
			st.Min, st.Max = minv, maxv
		}
		if st.Distinct <= 50 {
			rows, err := s.pool.Query(ctx, fmt.Sprintf(
				`SELECT %s::text, count(*) FROM %s WHERE %s IS NOT NULL GROUP BY 1 ORDER BY 2 DESC LIMIT 20`,
				col, quoteIdent(l.TableName), col))
			if err == nil {
				for rows.Next() {
					var vc ValueCount
					if err := rows.Scan(&vc.Value, &vc.Count); err == nil {
						st.Top = append(st.Top, vc)
					}
				}
				rows.Close()
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// AnalyzeRequest 描述一次空间分析（不落库，返回 GeoJSON）。
type AnalyzeRequest struct {
	Op        string  // buffer / centroid / convexhull / simplify / envelope
	Distance  float64 // buffer 距离（图层坐标系单位）
	Tolerance float64 // simplify 容差（图层坐标系单位）
	OutSRID   int
	Limit     int
}

// AnalyzeFeatures 对查询结果逐要素做空间运算，返回 GeoJSON FeatureCollection。
func (s *Store) AnalyzeFeatures(ctx context.Context, l *models.Layer, q FeatureQuery, req AnalyzeRequest) (json.RawMessage, int64, error) {
	if !validIdent(l.TableName) {
		return nil, 0, fmt.Errorf("invalid table name")
	}
	if req.OutSRID == 0 {
		req.OutSRID = 4326
	}
	if req.Limit <= 0 || req.Limit > 100000 {
		req.Limit = 10000
	}
	var expr string
	switch req.Op {
	case "buffer":
		expr = fmt.Sprintf("ST_Buffer(t.geom, %g)", req.Distance)
	case "centroid":
		expr = "ST_Centroid(t.geom)"
	case "convexhull":
		expr = "ST_ConvexHull(t.geom)"
	case "envelope":
		expr = "ST_Envelope(t.geom)"
	case "simplify":
		expr = fmt.Sprintf("ST_SimplifyPreserveTopology(t.geom, %g)", req.Tolerance)
	default:
		return nil, 0, fmt.Errorf("不支持的空间分析操作 %q", req.Op)
	}

	cw := &whereBuilder{}
	if err := buildWhere(l, &q, cw); err != nil {
		return nil, 0, err
	}
	countSQL := "SELECT count(*) FROM " + quoteIdent(l.TableName) + " t"
	if len(cw.conds) > 0 {
		countSQL += " WHERE " + strings.Join(cw.conds, " AND ")
	}
	var total int64
	if err := s.pool.QueryRow(ctx, countSQL, cw.args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	w := &whereBuilder{}
	if err := buildWhere(l, &q, w); err != nil {
		return nil, 0, err
	}
	limitPh := w.arg(req.Limit)
	sql := fmt.Sprintf(`
		SELECT COALESCE(jsonb_agg(jsonb_build_object(
			'type','Feature',
			'id', sub.fid,
			'geometry', (ST_AsGeoJSON(ST_Transform(sub.g, %d))::jsonb - 'crs'),
			'properties', sub.props
		) ORDER BY sub.fid), '[]'::jsonb)
		FROM (
			SELECT t.fid, %s AS g, %s AS props
			FROM %s t
			%s
			ORDER BY t.fid
			LIMIT %s
		) sub`,
		req.OutSRID, expr, propsObjectExpr(l), quoteIdent(l.TableName), whereClause(w.conds), limitPh)

	var features json.RawMessage
	if err := s.pool.QueryRow(ctx, sql, w.args...).Scan(&features); err != nil {
		return nil, 0, err
	}
	out, err := json.Marshal(map[string]any{
		"type":     "FeatureCollection",
		"crs":      map[string]any{"type": "name", "properties": map[string]any{"name": fmt.Sprintf("EPSG:%d", req.OutSRID)}},
		"features": features,
		"total":    total,
	})
	return out, total, err
}
