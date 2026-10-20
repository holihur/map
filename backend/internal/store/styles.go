package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"mapserver/internal/models"
)

const styleCols = `id, name, is_default, style, COALESCE(sld_xml, ''), created_at, updated_at`

func scanStyle(row pgx.Row, st *models.Style) error {
	var raw []byte
	if err := row.Scan(&st.ID, &st.Name, &st.IsDefault, &raw, &st.SLD, &st.CreatedAt, &st.UpdatedAt); err != nil {
		return err
	}
	if len(raw) > 0 {
		st.Style = json.RawMessage(raw)
	}
	return nil
}

// ListStyles 返回全部样式。
func (s *Store) ListStyles(ctx context.Context) ([]models.Style, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+styleCols+` FROM styles ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Style, 0)
	for rows.Next() {
		var st models.Style
		if err := scanStyle(rows, &st); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// GetStyleByName 按名称查询样式。
func (s *Store) GetStyleByName(ctx context.Context, name string) (*models.Style, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+styleCols+` FROM styles WHERE name=$1`, name)
	var st models.Style
	if err := scanStyle(row, &st); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &st, nil
}

// CreateStyle 创建样式；若标记为默认则清除其它默认样式。
func (s *Store) CreateStyle(ctx context.Context, req models.CreateStyleRequest) (*models.Style, error) {
	style := req.Style
	if len(style) == 0 {
		style = json.RawMessage(`{}`)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if req.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE styles SET is_default=false WHERE is_default=true`); err != nil {
			return nil, err
		}
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO styles (name, is_default, style, sld_xml) VALUES ($1,$2,$3,NULLIF($4,'')) RETURNING `+styleCols,
		req.Name, req.IsDefault, []byte(style), req.SLD)
	var st models.Style
	if err := scanStyle(row, &st); err != nil {
		return nil, err
	}
	return &st, tx.Commit(ctx)
}

// UpdateStyle 更新样式。
func (s *Store) UpdateStyle(ctx context.Context, id int64, req models.CreateStyleRequest) (*models.Style, error) {
	style := req.Style
	if len(style) == 0 {
		style = json.RawMessage(`{}`)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if req.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE styles SET is_default=false WHERE is_default=true`); err != nil {
			return nil, err
		}
	}
	row := tx.QueryRow(ctx,
		`UPDATE styles SET name=$1, is_default=$2, style=$3, sld_xml=NULLIF($4,''), updated_at=now() WHERE id=$5 RETURNING `+styleCols,
		req.Name, req.IsDefault, []byte(style), req.SLD, id)
	var st models.Style
	if err := scanStyle(row, &st); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &st, tx.Commit(ctx)
}

// DeleteStyle 删除样式。
func (s *Store) DeleteStyle(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM styles WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
