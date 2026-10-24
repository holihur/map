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
	style, style_name, metadata, sort_order, COALESCE(visibility, 'public'), created_at, updated_at`

func scanLayer(row pgx.Row, l *models.Layer) error {
	var fieldsRaw, styleRaw, metaRaw []byte
	var minx, miny, maxx, maxy *float64
	var styleName *string
	err := row.Scan(&l.ID, &l.Name, &l.Title, &l.Description, &l.GeometryType, &l.SRID, &l.TableName,
		&fieldsRaw, &l.MinZoom, &l.MaxZoom, &minx, &miny, &maxx, &maxy, &styleRaw, &styleName, &metaRaw,
		&l.SortOrder, &l.Visibility, &l.CreatedAt, &l.UpdatedAt)
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
	rows, err := s.pool.Query(ctx, `SELECT `+layerCols+` FROM layers ORDER BY sort_order, name`)
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

	// 避免与第三方图层同名（前端以 name 作为唯一键）。
	var extExists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM external_layers WHERE name=$1)`, name).Scan(&extExists); err != nil {
		return nil, err
	}
	if extExists {
		return nil, fmt.Errorf("图层名 %q 已被第三方图层占用", name)
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
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	visibility := "public"
	if req.Visibility != nil && *req.Visibility != "" {
		visibility = normalizeVisibility(*req.Visibility)
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
		INSERT INTO layers (name, title, description, geometry_type, srid, table_name, fields, min_zoom, max_zoom, style, metadata, sort_order, visibility)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING `+layerCols,
		name, req.Title, req.Description, geometryType, srid, table, fieldsJSON, minZoom, maxZoom, style, meta, sortOrder, visibility)

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
	if req.SortOrder != nil {
		add("sort_order = $%d", *req.SortOrder)
	}
	if req.Visibility != nil && *req.Visibility != "" {
		add("visibility = $%d", normalizeVisibility(*req.Visibility))
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

// normalizeVisibility 只允许 public / private。
func normalizeVisibility(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "private") {
		return "private"
	}
	return "public"
}

// ReorderLayers 按 names 的顺序重写 sort_order；未列出的图层排在后面。
func (s *Store) ReorderLayers(ctx context.Context, names []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for i, name := range names {
		if _, err := tx.Exec(ctx,
			`UPDATE layers SET sort_order=$1, updated_at=now() WHERE name=$2`, i, name); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// GetLayerAccess 返回图层的发布设置。
func (s *Store) GetLayerAccess(ctx context.Context, layerID int64) (*models.LayerAccessSettings, error) {
	var visibility string
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(visibility,'public') FROM layers WHERE id=$1`, layerID).Scan(&visibility); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT a.user_id, u.username, a.level
		FROM layer_access a JOIN users u ON u.id=a.user_id
		WHERE a.layer_id=$1 ORDER BY u.username`, layerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &models.LayerAccessSettings{Visibility: visibility, Access: []models.LayerAccess{}}
	for rows.Next() {
		var a models.LayerAccess
		if err := rows.Scan(&a.UserID, &a.Username, &a.Level); err != nil {
			return nil, err
		}
		out.Access = append(out.Access, a)
	}
	return out, rows.Err()
}

// SetLayerAccess 覆盖式更新图层可见性与授权。
func (s *Store) SetLayerAccess(ctx context.Context, layerID int64, visibility string, access []models.LayerAccess) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`UPDATE layers SET visibility=$1, updated_at=now() WHERE id=$2`,
		normalizeVisibility(visibility), layerID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM layer_access WHERE layer_id=$1`, layerID); err != nil {
		return err
	}
	for _, a := range access {
		level := "read"
		if strings.EqualFold(a.Level, "write") {
			level = "write"
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO layer_access (layer_id,user_id,level) VALUES ($1,$2,$3)
			 ON CONFLICT (layer_id,user_id) DO UPDATE SET level=EXCLUDED.level`,
			layerID, a.UserID, level); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// LayerAccessLevel 返回用户对图层的最高权限（"" 表示无权限）。
func (s *Store) LayerAccessLevel(ctx context.Context, layerID, userID int64) (string, error) {
	var level string
	err := s.pool.QueryRow(ctx,
		`SELECT level FROM layer_access WHERE layer_id=$1 AND user_id=$2`, layerID, userID).Scan(&level)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return level, err
}
