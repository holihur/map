package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"

	"mapserver/internal/models"
)

const mapStateCols = `id, token, name, state, COALESCE(created_by,0), is_public, created_at, updated_at`

func scanMapState(row pgx.Row, m *models.MapState) error {
	var raw []byte
	if err := row.Scan(&m.ID, &m.Token, &m.Name, &raw, &m.CreatedBy, &m.IsPublic, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return err
	}
	m.State = raw
	return nil
}

// NewShareToken 生成一个不可猜测的分享令牌。
func NewShareToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateMapState 保存地图状态。
func (s *Store) CreateMapState(ctx context.Context, name string, state []byte, createdBy int64, isPublic bool) (*models.MapState, error) {
	token, err := NewShareToken()
	if err != nil {
		return nil, err
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO map_states (token, name, state, created_by, is_public)
		VALUES ($1,$2,$3,NULLIF($4,0),$5) RETURNING `+mapStateCols,
		token, name, state, createdBy, isPublic)
	var m models.MapState
	if err := scanMapState(row, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// GetMapStateByToken 按分享令牌读取地图状态。
func (s *Store) GetMapStateByToken(ctx context.Context, token string) (*models.MapState, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+mapStateCols+` FROM map_states WHERE token=$1`, token)
	var m models.MapState
	if err := scanMapState(row, &m); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

// ListMapStates 返回地图状态；isAdmin=false 时仅返回自己的。
func (s *Store) ListMapStates(ctx context.Context, userID int64, isAdmin bool) ([]models.MapState, error) {
	sql := `SELECT ` + mapStateCols + ` FROM map_states`
	args := []any{}
	if !isAdmin {
		args = append(args, userID)
		sql += ` WHERE created_by=$1`
	}
	sql += ` ORDER BY updated_at DESC LIMIT 200`
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.MapState, 0)
	for rows.Next() {
		var m models.MapState
		if err := scanMapState(rows, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpdateMapState 更新地图状态。
func (s *Store) UpdateMapState(ctx context.Context, token, name string, state []byte, isPublic *bool) (*models.MapState, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE map_states SET
			name = COALESCE(NULLIF($2,''), name),
			state = COALESCE($3, state),
			is_public = COALESCE($4, is_public),
			updated_at = now()
		WHERE token=$1 RETURNING `+mapStateCols,
		token, name, state, isPublic)
	var m models.MapState
	if err := scanMapState(row, &m); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

// DeleteMapState 删除地图状态。
func (s *Store) DeleteMapState(ctx context.Context, token string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM map_states WHERE token=$1`, token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
