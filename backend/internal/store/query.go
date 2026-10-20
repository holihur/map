package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"mapserver/internal/models"
)

// geomLiteral 输出几何为 GeoJSON（去除 PostGIS 自带的 crs 成员）。
func geomLiteral(outSRID int) string {
	return fmt.Sprintf("(ST_AsGeoJSON(ST_Transform(t.geom, %d))::jsonb - 'crs')", outSRID)
}

// GMLFeature 供 WFS GML 输出使用。
type GMLFeature struct {
	ID    int64          `json:"id"`
	GML   string         `json:"gml"`
	Props map[string]any `json:"props"`
}

// QueryGML 返回要素的 GML 几何与属性（忽略分页总数）。
func (s *Store) QueryGML(ctx context.Context, l *models.Layer, q FeatureQuery) ([]GMLFeature, int64, error) {
	if !validIdent(l.TableName) {
		return nil, 0, fmt.Errorf("invalid table name")
	}
	if q.OutSRID == 0 {
		q.OutSRID = 4326
	}
	if q.Limit <= 0 || q.Limit > 100000 {
		q.Limit = 1000
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
	geomPh := w.arg(q.OutSRID)
	if err := buildWhere(l, &q, w); err != nil {
		return nil, 0, err
	}
	limitPh := w.arg(q.Limit)
	offsetPh := w.arg(q.Offset)

	sql := fmt.Sprintf(`
		SELECT COALESCE(jsonb_agg(jsonb_build_object(
			'id', sub.fid,
			'gml', ST_AsGML(3, ST_Transform(sub.geom, %s::int), 9),
			'props', sub.props
		) ORDER BY sub.fid), '[]'::jsonb)
		FROM (
			SELECT t.fid, t.geom, %s AS props
			FROM %s t
			%s
			ORDER BY t.fid
			LIMIT %s OFFSET %s
		) sub`, geomPh, propsObjectExpr(l), quoteIdent(l.TableName), whereClause(w.conds), limitPh, offsetPh)

	var raw json.RawMessage
	if err := s.pool.QueryRow(ctx, sql, w.args...).Scan(&raw); err != nil {
		return nil, 0, err
	}
	var out []GMLFeature
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// QueryPoint 查询距离某点最近的要素（用于 WMS GetFeatureInfo）。
// x,y 为请求坐标系（inSRID）下的坐标；offset 为容差（同一坐标系下的长度，
// 通常取若干像素对应的地图单位）。
func (s *Store) QueryPoint(
	ctx context.Context,
	l *models.Layer,
	x, y float64,
	inSRID int,
	offset float64,
	outSRID int,
	limit int,
) (json.RawMessage, error) {
	if !validIdent(l.TableName) {
		return nil, fmt.Errorf("invalid table name")
	}
	if inSRID == 0 {
		inSRID = 4326
	}
	if outSRID == 0 {
		outSRID = 4326
	}
	if offset <= 0 {
		offset = 1e-9
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// 容差在图层坐标系下计算，避免跨投影后单位不一致。
	sql := fmt.Sprintf(`
		WITH p AS (
			SELECT
				ST_Transform(ST_SetSRID(ST_MakePoint($1, $2), %d), %d) AS g,
				ST_Transform(ST_SetSRID(ST_MakePoint($1 + $3, $2 + $3), %d), %d) AS g2
		)
		SELECT COALESCE(jsonb_agg(sub.f ORDER BY sub.fid), '[]'::jsonb)
		FROM (
			SELECT t.fid AS fid, jsonb_build_object(
				'type', 'Feature',
				'id', t.fid,
				'geometry', %s,
				'properties', %s
			) AS f
			FROM %s t, p
			WHERE t.geom IS NOT NULL
			  AND ST_DWithin(t.geom, p.g, GREATEST(ST_Distance(p.g, p.g2), 1e-12))
			ORDER BY ST_Distance(t.geom, p.g)
			LIMIT $4
		) sub`,
		inSRID, l.SRID, inSRID, l.SRID,
		geomLiteral(outSRID), propsObjectExpr(l), quoteIdent(l.TableName))

	var features json.RawMessage
	if err := s.pool.QueryRow(ctx, sql, x, y, offset, limit).Scan(&features); err != nil {
		return nil, err
	}

	fc := struct {
		Type     string          `json:"type"`
		CRS      map[string]any  `json:"crs"`
		Features json.RawMessage `json:"features"`
		Total    int64           `json:"total"`
	}{
		Type:     "FeatureCollection",
		CRS:      map[string]any{"type": "name", "properties": map[string]any{"name": fmt.Sprintf("EPSG:%d", outSRID)}},
		Features: features,
	}
	return json.Marshal(fc)
}
