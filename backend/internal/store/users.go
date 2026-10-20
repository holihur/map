package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"mapserver/internal/models"
)

const userCols = `id, username, role, COALESCE(token_version, 0), password_hash, created_at, updated_at`

func scanUser(row pgx.Row, u *models.User) error {
	return row.Scan(&u.ID, &u.Username, &u.Role, &u.TokenVersion, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
}

// CountUsers 返回用户数量。
func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

// ListUsers 返回全部用户（不含密码哈希）。
func (s *Store) ListUsers(ctx context.Context) ([]models.User, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.User, 0)
	for rows.Next() {
		var u models.User
		if err := scanUser(rows, &u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetUserByUsername 按用户名查询用户（含密码哈希）。
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE username=$1`, username)
	var u models.User
	if err := scanUser(row, &u); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// CreateUser 创建用户。
func (s *Store) CreateUser(ctx context.Context, username, passwordHash, role string) (*models.User, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, role) VALUES ($1,$2,$3) RETURNING `+userCols,
		username, passwordHash, role)
	var u models.User
	if err := scanUser(row, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateUser 更新密码与角色；修改密码会递增 token_version 以失效已有 Token。
func (s *Store) UpdateUser(ctx context.Context, id int64, passwordHash, role *string) (*models.User, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE users SET
			password_hash = COALESCE($1, password_hash),
			role          = COALESCE($2, role),
			token_version = CASE WHEN $1 IS NULL THEN token_version ELSE token_version + 1 END,
			updated_at    = now()
		WHERE id=$3 RETURNING `+userCols,
		passwordHash, role, id)
	var u models.User
	if err := scanUser(row, &u); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// TokenVersion 返回用户的当前 Token 版本。
func (s *Store) TokenVersion(ctx context.Context, id int64) (int, error) {
	var v int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(token_version,0) FROM users WHERE id=$1`, id).Scan(&v)
	return v, err
}

// BumpTokenVersion 递增 Token 版本（使该用户所有 Token 失效）。
func (s *Store) BumpTokenVersion(ctx context.Context, id int64) (int, error) {
	var v int
	err := s.pool.QueryRow(ctx,
		`UPDATE users SET token_version = COALESCE(token_version,0) + 1, updated_at = now() WHERE id=$1 RETURNING token_version`,
		id).Scan(&v)
	return v, err
}

// DeleteUser 删除用户。
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
