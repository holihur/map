package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"mapserver/internal/models"
)

const layerCols = `id, name, title, description, geometry_type, srid, table_name,
	fields, min_zoom, max_zoom,
	ST_XMin(bounds), ST_YMin(bounds), ST_XMax(bounds), ST_YMax(bounds),
	style, style_name, metadata, created_at, updated_at`

func scanLayer(row pgx.Row, l *models.Layer) error {
	var fieldsRaw, styleRaw, metaRaw []byte
	var minx, miny, maxx, maxy *float64
	var styleName *string
	err := row.Scan(&l.ID, &l.Name, &l.Title, &l.Description, &l.GeometryType, &l.SRID, &l.TableName,
		&fieldsRaw, &l.MinZoom, &l.MaxZoom, &minx, &miny, &maxx, &maxy, &styleRaw, &styleName, &metaRaw, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return err
	}
	if styleName != nil {
		l.StyleName = *styleName
	}
	if len(fieldsRaw) > 0 {
		_ = json.Unmarshal(fieldsRaw, &l.Fields)
	}
	if len(styleRaw) > 0 {
		l.Style = json.RawMessage(styleRaw)
	}
	if len(metaRaw) > 0 {
		l.Metadata = json.RawMessage(metaRaw)
	}
	if minx != nil && miny != nil && maxx != nil && maxy != nil {
		l.Bounds = &[4]float64{*minx, *miny, *maxx, *maxy}
	}
	return nil
}

// ListLayers 返回全部图层（含要素数量，使用单次批量统计避免 N+1）。
func (s *Store) ListLayers(ctx context.Context) ([]models.Layer, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+layerCols+` FROM layers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	layers := make([]models.Layer, 0)
	tables := make([]string, 0)
	for rows.Next() {
		var l models.Layer
		if err := scanLayer(rows, &l); err != nil {
			return nil, err
		}
		layers = append(layers, l)
		tables = append(tables, l.TableName)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	counts, err := s.countFeaturesBulk(ctx, tables)
	if err == nil {
		for i := range layers {
			if n, ok := counts[layers[i].TableName]; ok {
				layers[i].FeatureCount = n
			}
		}
	}
	return layers, nil
}

// countFeaturesBulk 用一条 UNION ALL 查询统计多张表的行数。
func (s *Store) countFeaturesBulk(ctx context.Context, tables []string) (map[string]int64, error) {
	out := map[string]int64{}
	parts := make([]string, 0, len(tables))
	for _, t := range tables {
		if !validIdent(t) {
			continue
		}
		// t 为校验过的标识符（[a-z][a-z0-9_]*），可直接作为字面量。
		parts = append(parts, "SELECT '"+t+"' AS t, count(*) AS n FROM "+quoteIdent(t))
	}
	if len(parts) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, strings.Join(parts, " UNION ALL "))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var n int64
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		out[name] = n
	}
	return out, rows.Err()
}

func (s *Store) countFeatures(ctx context.Context, table string) (int64, error) {
	if !validIdent(table) {
		return 0, fmt.Errorf("invalid table %q", table)
	}
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM `+quoteIdent(table)).Scan(&n)
	return n, err
}

// GetLayerByName 按名称查询图层。
func (s *Store) GetLayerByName(ctx context.Context, name string) (*models.Layer, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+layerCols+` FROM layers WHERE name=$1`, name)
	var l models.Layer
	if err := scanLayer(row, &l); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if n, err := s.countFeatures(ctx, l.TableName); err == nil {
		l.FeatureCount = n
	}
	return &l, nil
}

// GetLayerByID 按 ID 查询图层。
func (s *Store) GetLayerByID(ctx context.Context, id int64) (*models.Layer, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+layerCols+` FROM layers WHERE id=$1`, id)
	var l models.Layer
	if err := scanLayer(row, &l); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if n, err := s.countFeatures(ctx, l.TableName); err == nil {
		l.FeatureCount = n
	}
	return &l, nil
}

// CreateLayer 创建图层元数据与物理空间表。
func (s *Store) CreateLayer(ctx context.Context, req models.CreateLayerRequest) (*models.Layer, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !validIdent(name) {
		return nil, fmt.Errorf("invalid layer name %q (只允许小写字母、数字、下划线，且以字母开头)", req.Name)
	}
	geometryType := strings.ToUpper(strings.TrimSpace(req.GeometryType))
	if geometryType == "" {
		geometryType = "GEOMETRY"
	}
	switch geometryType {
	case "GEOMETRY", "POINT", "LINESTRING", "POLYGON", "MULTIPOINT", "MULTILINESTRING", "MULTIPOLYGON", "GEOMETRYCOLLECTION":
	default:
		return nil, fmt.Errorf("invalid geometry type %q", geometryType)
	}
	srid := req.SRID
	if srid == 0 {
		srid = 4326
	}
	if ok, err := s.SRIDExists(ctx, srid); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("未知的 SRID %d（不在 spatial_ref_sys 中）", srid)
	}

	seen := map[string]bool{}
	for _, f := range req.Fields {
		fname := strings.ToLower(strings.TrimSpace(f.Name))
		if !validIdent(fname) {
			return nil, fmt.Errorf("invalid field name %q", f.Name)
		}
		if fname == "fid" || fname == "geom" || fname == "created_at" || fname == "updated_at" {
			return nil, fmt.Errorf("field name %q 为保留字段", f.Name)
		}
		if seen[fname] {
			return nil, fmt.Errorf("duplicate field %q", fname)
		}
		seen[fname] = true
		if _, err := columnType(string(f.Type)); err != nil {
			return nil, err
		}
	}

	table := "layer_" + name
	minZoom, maxZoom := 0, 22
	if req.MinZoom != nil {
		minZoom = *req.MinZoom
	}
	if req.MaxZoom != nil {
		maxZoom = *req.MaxZoom
	}
	style := req.Style
	if len(style) == 0 {
		style = json.RawMessage(`{}`)
	}
	meta := req.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	layer := models.Layer{GeometryType: geometryType, SRID: srid}
	var ddl strings.Builder
	ddl.WriteString("CREATE TABLE " + quoteIdent(table) + " (fid BIGSERIAL PRIMARY KEY")
	for _, f := range req.Fields {
		colType, _ := columnType(string(f.Type))
		notNull := ""
		if !f.Nullable {
			notNull = ""
		}
		ddl.WriteString(", " + quoteIdent(strings.ToLower(f.Name)) + " " + colType + notNull)
	}
	ddl.WriteString(", geom " + layer.GeomTypeSQL())
	ddl.WriteString(", created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now())")

	if _, err := tx.Exec(ctx, ddl.String()); err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}
	if _, err := tx.Exec(ctx, "CREATE INDEX "+quoteIdent(table+"_geom_gix")+" ON "+quoteIdent(table)+" USING GIST (geom)"); err != nil {
		return nil, fmt.Errorf("create spatial index: %w", err)
	}

	fieldsJSON, _ := json.Marshal(req.Fields)
	row := tx.QueryRow(ctx, `
		INSERT INTO layers (name, title, description, geometry_type, srid, table_name, fields, min_zoom, max_zoom, style, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING `+layerCols,
		name, req.Title, req.Description, geometryType, srid, table, fieldsJSON, minZoom, maxZoom, style, meta)

	var l models.Layer
	if err := scanLayer(row, &l); err != nil {
		return nil, fmt.Errorf("insert layer: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &l, nil
}

// UpdateLayer 更新图层可编辑的元数据。
func (s *Store) UpdateLayer(ctx context.Context, id int64, req models.UpdateLayerRequest) (*models.Layer, error) {
	sets := []string{"updated_at = now()"}
	args := []any{}
	add := func(expr string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf(expr, len(args)))
	}
	if req.Title != nil {
		add("title = $%d", *req.Title)
	}
	if req.Description != nil {
		add("description = $%d", *req.Description)
	}
	if req.MinZoom != nil {
		add("min_zoom = $%d", *req.MinZoom)
	}
	if req.MaxZoom != nil {
		add("max_zoom = $%d", *req.MaxZoom)
	}
	if len(req.Style) > 0 {
		add("style = $%d", []byte(req.Style))
	}
	if req.StyleName != nil {
		if *req.StyleName == "" {
			add("style_name = $%d", nil)
		} else {
			add("style_name = $%d", *req.StyleName)
		}
	}
	if len(req.Metadata) > 0 {
		add("metadata = $%d", []byte(req.Metadata))
	}
	args = append(args, id)
	row := s.pool.QueryRow(ctx,
		`UPDATE layers SET `+strings.Join(sets, ", ")+` WHERE id=$`+fmt.Sprint(len(args))+` RETURNING `+layerCols, args...)

	var l models.Layer
	if err := scanLayer(row, &l); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if n, err := s.countFeatures(ctx, l.TableName); err == nil {
		l.FeatureCount = n
	}
	return &l, nil
}

// TransformLayer 将图层所有几何转换到新的坐标系（重写 geom 列类型并重建索引）。
func (s *Store) TransformLayer(ctx context.Context, l *models.Layer, newSRID int) (*models.Layer, error) {
	if newSRID == l.SRID {
		return l, nil
	}
	ok, err := s.SRIDExists(ctx, newSRID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("未知的 SRID %d", newSRID)
	}

	gt := l.GeometryType
	if gt == "" {
		gt = "GEOMETRY"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ddl := fmt.Sprintf(
		"ALTER TABLE %s ALTER COLUMN geom TYPE geometry(%s, %d) USING ST_Transform(geom, %d)",
		quoteIdent(l.TableName), gt, newSRID, newSRID)
	if _, err := tx.Exec(ctx, ddl); err != nil {
		return nil, fmt.Errorf("转换图层几何失败: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE layers SET srid=$1, updated_at=now() WHERE id=$2`, newSRID, l.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetLayerByID(ctx, l.ID)
}

// DeleteLayer 删除图层元数据并 DROP 物理表。
func (s *Store) DeleteLayer(ctx context.Context, id int64) error {
	l, err := s.GetLayerByID(ctx, id)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "DROP TABLE IF EXISTS "+quoteIdent(l.TableName)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM layers WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
