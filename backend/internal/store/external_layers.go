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

const externalLayerCols = `id, name, title, description, source_type, source_url,
	source_options, style, metadata, min_zoom, max_zoom, sort_order,
	COALESCE(visibility,'public'), created_at, updated_at`

func scanExternalLayer(row pgx.Row, l *models.ExternalLayer) error {
	var options, style, meta []byte
	err := row.Scan(&l.ID, &l.Name, &l.Title, &l.Description, &l.SourceType, &l.SourceURL,
		&options, &style, &meta, &l.MinZoom, &l.MaxZoom, &l.SortOrder, &l.Visibility,
		&l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return err
	}
	l.SourceOptions = options
	l.Style = style
	l.Metadata = meta
	return nil
}

// ExternalSourceTypes 返回支持的第三方图层类型。
func ExternalSourceTypes() []string {
	return []string{"xyz", "wms", "wmts", "mvt", "geojson", "arcgis"}
}

func normalizeSourceType(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "raster" {
		v = "xyz"
	}
	for _, t := range ExternalSourceTypes() {
		if t == v {
			return v, nil
		}
	}
	return "", fmt.Errorf("不支持的第三方图层类型 %q（可选：%s）", v, strings.Join(ExternalSourceTypes(), ", "))
}

func defaultExternalStyle(sourceType string) json.RawMessage {
	switch sourceType {
	case "geojson", "mvt":
		return json.RawMessage(`{"type":"fill","paint":{"fill-color":"#2563eb","fill-opacity":0.45,"fill-outline-color":"#1d4ed8"}}`)
	default:
		return json.RawMessage(`{"type":"raster","paint":{"raster-opacity":1}}`)
	}
}

// ListExternalLayers 返回全部第三方图层。
func (s *Store) ListExternalLayers(ctx context.Context) ([]models.ExternalLayer, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+externalLayerCols+` FROM external_layers ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.ExternalLayer, 0)
	for rows.Next() {
		var l models.ExternalLayer
		if err := scanExternalLayer(rows, &l); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetExternalLayerByName 按名称查询第三方图层。
func (s *Store) GetExternalLayerByName(ctx context.Context, name string) (*models.ExternalLayer, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+externalLayerCols+` FROM external_layers WHERE name=$1`, name)
	var l models.ExternalLayer
	if err := scanExternalLayer(row, &l); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &l, nil
}

// CreateExternalLayer 创建第三方图层。
func (s *Store) CreateExternalLayer(ctx context.Context, req models.CreateExternalLayerRequest) (*models.ExternalLayer, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !validIdent(name) {
		return nil, fmt.Errorf("invalid layer name %q (只允许小写字母、数字、下划线，且以字母开头)", req.Name)
	}
	sourceType, err := normalizeSourceType(req.SourceType)
	if err != nil {
		return nil, err
	}
	sourceURL := strings.TrimSpace(req.SourceURL)
	if sourceURL == "" {
		return nil, fmt.Errorf("sourceUrl 不能为空")
	}
	// 避免与 PostGIS 图层同名（前端以 name 作为唯一键）。
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM layers WHERE name=$1)`, name).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("图层名 %q 已被 PostGIS 图层占用", name)
	}

	minZoom, maxZoom := 0, 22
	if req.MinZoom != nil {
		minZoom = *req.MinZoom
	}
	if req.MaxZoom != nil {
		maxZoom = *req.MaxZoom
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	visibility := "public"
	if req.Visibility != nil && *req.Visibility != "" {
		visibility = normalizeVisibility(*req.Visibility)
	}
	options := req.SourceOptions
	if len(options) == 0 || !json.Valid(options) {
		options = json.RawMessage(`{}`)
	}
	style := req.Style
	if len(style) == 0 || !json.Valid(style) {
		style = defaultExternalStyle(sourceType)
	}
	meta := req.Metadata
	if len(meta) == 0 || !json.Valid(meta) {
		meta = json.RawMessage(`{}`)
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO external_layers
			(name, title, description, source_type, source_url, source_options, style, metadata,
			 min_zoom, max_zoom, sort_order, visibility)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING `+externalLayerCols,
		name, req.Title, req.Description, sourceType, sourceURL, options, style, meta,
		minZoom, maxZoom, sortOrder, visibility)
	var l models.ExternalLayer
	if err := scanExternalLayer(row, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// UpdateExternalLayer 更新第三方图层。
func (s *Store) UpdateExternalLayer(ctx context.Context, id int64, req models.UpdateExternalLayerRequest) (*models.ExternalLayer, error) {
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
	if req.SourceType != nil && *req.SourceType != "" {
		st, err := normalizeSourceType(*req.SourceType)
		if err != nil {
			return nil, err
		}
		add("source_type = $%d", st)
	}
	if req.SourceURL != nil {
		u := strings.TrimSpace(*req.SourceURL)
		if u == "" {
			return nil, fmt.Errorf("sourceUrl 不能为空")
		}
		add("source_url = $%d", u)
	}
	if len(req.SourceOptions) > 0 {
		if !json.Valid(req.SourceOptions) {
			return nil, fmt.Errorf("sourceOptions 必须是合法 JSON")
		}
		add("source_options = $%d", []byte(req.SourceOptions))
	}
	if len(req.Style) > 0 {
		if !json.Valid(req.Style) {
			return nil, fmt.Errorf("style 必须是合法 JSON")
		}
		add("style = $%d", []byte(req.Style))
	}
	if len(req.Metadata) > 0 {
		if !json.Valid(req.Metadata) {
			return nil, fmt.Errorf("metadata 必须是合法 JSON")
		}
		add("metadata = $%d", []byte(req.Metadata))
	}
	if req.MinZoom != nil {
		add("min_zoom = $%d", *req.MinZoom)
	}
	if req.MaxZoom != nil {
		add("max_zoom = $%d", *req.MaxZoom)
	}
	if req.SortOrder != nil {
		add("sort_order = $%d", *req.SortOrder)
	}
	if req.Visibility != nil && *req.Visibility != "" {
		add("visibility = $%d", normalizeVisibility(*req.Visibility))
	}
	args = append(args, id)
	row := s.pool.QueryRow(ctx,
		`UPDATE external_layers SET `+strings.Join(sets, ", ")+` WHERE id=$`+fmt.Sprint(len(args))+` RETURNING `+externalLayerCols,
		args...)
	var l models.ExternalLayer
	if err := scanExternalLayer(row, &l); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &l, nil
}

// DeleteExternalLayer 删除第三方图层。
func (s *Store) DeleteExternalLayer(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM external_layers WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
