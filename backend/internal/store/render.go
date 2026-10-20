package store

import (
	"context"
	"fmt"
	"math"
	"strings"

	"mapserver/internal/models"
)

// RenderRequest 描述一次栅格出图请求。
type RenderRequest struct {
	BBox    [4]float64 // minx,miny,maxx,maxy（OutSRID 坐标系）
	OutSRID int        // 输出坐标系，默认 4326
	Width   int
	Height  int
	// 符号尺寸（像素），来自 SLD；<=0 时使用默认值。
	PointRadiusPx float64
	LineWidthPx   float64
	// 属性过滤（来自 SLD Rule 的 Filter）。
	Filters []Filter
}

// RenderMask 将图层几何栅格化为 Width×Height 的 0/255 掩膜（行优先）。
// 几何会先重投影到 OutSRID，因此支持任意输出坐标系。
// 不依赖 GDAL，掩膜在应用层着色合成 PNG。
func (s *Store) RenderMask(ctx context.Context, l *models.Layer, req RenderRequest) ([][]float64, error) {
	if !validIdent(l.TableName) {
		return nil, fmt.Errorf("invalid table name")
	}
	if req.Width <= 0 || req.Height <= 0 {
		return nil, fmt.Errorf("invalid raster size")
	}
	if req.OutSRID == 0 {
		req.OutSRID = 4326
	}

	scalex := (req.BBox[2] - req.BBox[0]) / float64(req.Width)
	scaley := -((req.BBox[3] - req.BBox[1]) / float64(req.Height))
	pixel := math.Abs(scalex)

	pointRadius := req.PointRadiusPx
	if pointRadius <= 0 {
		pointRadius = 2.5
	}
	lineWidth := req.LineWidthPx
	if lineWidth <= 0 {
		lineWidth = 0.7
	}

	// 先按瓦片范围裁剪几何，避免大几何在高缩放下产生超尺寸栅格。
	// ST_MakeValid 兼容自相交等非法几何。
	transformed := fmt.Sprintf("ST_MakeValid(ST_Transform(t.geom, %d))", req.OutSRID)
	intersect := fmt.Sprintf("ST_Intersection(%s, ext.g)", transformed)
	geoOut := intersect
	gt := strings.ToUpper(l.GeometryType)
	switch {
	case strings.Contains(gt, "POINT"):
		geoOut = fmt.Sprintf("ST_Buffer(%s, %g)", intersect, pixel*pointRadius)
	case strings.Contains(gt, "LINE"):
		geoOut = fmt.Sprintf("ST_Buffer(%s, %g)", intersect, pixel*lineWidth)
	}

	// 属性过滤条件（占位符从 $8 开始，避开栅格参数 $1..$7）。
	fb := &whereBuilder{n: 7}
	if len(req.Filters) > 0 {
		if err := buildWhere(l, &FeatureQuery{Filters: req.Filters}, fb); err != nil {
			return nil, err
		}
	}
	filterConds := append([]string{
		"t.geom IS NOT NULL",
		fmt.Sprintf("t.geom && ST_Transform(ext.g, %d)", l.SRID),
	}, fb.conds...)

	// ST_AsRaster 只覆盖几何本身的范围，故与整幅空白栅格做 ST_Union 聚合铺满画布。
	sql := fmt.Sprintf(`
		WITH ref AS (
			SELECT ST_AddBand(
				ST_MakeEmptyRaster($1::int, $2::int, $3::float8, $4::float8,
					$5::float8, $6::float8, 0::float8, 0::float8, $7::int),
				'8BUI'::text, 0::float8, 0::float8) AS rast
		), ext AS (
			SELECT rast::geometry AS g FROM ref
		), clipped AS (
			SELECT %s AS g
			FROM %s t, ext
			WHERE %s
		), burned AS (
			SELECT ST_AsRaster(ST_Collect(g), (SELECT rast FROM ref), '8BUI', 255::float8, 0::float8) AS r
			FROM clipped
			WHERE g IS NOT NULL AND NOT ST_IsEmpty(g)
		), comp AS (
			SELECT ST_Union(r) AS u FROM (
				SELECT rast AS r FROM ref
				UNION ALL
				SELECT r FROM burned WHERE r IS NOT NULL
			) s
		)
		SELECT ST_DumpValues(u, 1, false) FROM comp`,
		geoOut, quoteIdent(l.TableName), strings.Join(filterConds, " AND "))

	args := append([]any{
		req.Width, req.Height, req.BBox[0], req.BBox[3], scalex, scaley, req.OutSRID,
	}, fb.args...)

	var mask [][]float64
	if err := s.pool.QueryRow(ctx, sql, args...).Scan(&mask); err != nil {
		return nil, err
	}
	return mask, nil
}

// LayerBoundsIn 返回图层在指定坐标系下的范围。
func (s *Store) LayerBoundsIn(ctx context.Context, l *models.Layer, srid int) (*[4]float64, error) {
	if srid == 0 {
		srid = 4326
	}
	sql := fmt.Sprintf(
		"SELECT ST_XMin(e), ST_YMin(e), ST_XMax(e), ST_YMax(e) FROM (SELECT ST_Extent(ST_Transform(geom, %d)) AS e FROM %s) s",
		srid, quoteIdent(l.TableName))
	var minx, miny, maxx, maxy *float64
	if err := s.pool.QueryRow(ctx, sql).Scan(&minx, &miny, &maxx, &maxy); err != nil {
		return nil, err
	}
	if minx == nil {
		return nil, nil
	}
	return &[4]float64{*minx, *miny, *maxx, *maxy}, nil
}
