package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"mapserver/internal/models"
)

// HashAPIKey 返回 API Key 的 SHA-256 十六进制摘要。
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

const apiKeyCols = `id, name, key_prefix, role, COALESCE(layer_name,''), revoked, created_at, last_used_at`

func scanAPIKey(row pgx.Row, k *models.APIKey) error {
	return row.Scan(&k.ID, &k.Name, &k.Prefix, &k.Role, &k.LayerName, &k.Revoked, &k.CreatedAt, &k.LastUsedAt)
}

// CreateAPIKey 保存一个 API Key（仅保存哈希）并返回元数据。
func (s *Store) CreateAPIKey(ctx context.Context, name, prefix, keyHash, role, layerName string, createdBy int64) (*models.APIKey, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (name, key_prefix, key_hash, role, layer_name, created_by)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6)
		RETURNING `+apiKeyCols,
		name, prefix, keyHash, role, layerName, createdBy)
	var k models.APIKey
	if err := scanAPIKey(row, &k); err != nil {
		return nil, err
	}
	return &k, nil
}

// ListAPIKeys 返回全部 API Key（不含哈希）。
func (s *Store) ListAPIKeys(ctx context.Context) ([]models.APIKey, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+apiKeyCols+` FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.APIKey, 0)
	for rows.Next() {
		var k models.APIKey
		if err := scanAPIKey(rows, &k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeAPIKey 吊销 API Key。
func (s *Store) RevokeAPIKey(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE api_keys SET revoked=true WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAPIKey 删除 API Key。
func (s *Store) DeleteAPIKey(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM api_keys WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FindAPIKey 按明文 Key 查找有效记录。
func (s *Store) FindAPIKey(ctx context.Context, key string) (*models.APIKey, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+apiKeyCols+` FROM api_keys WHERE key_hash=$1 AND NOT revoked`, HashAPIKey(strings.TrimSpace(key)))
	var k models.APIKey
	if err := scanAPIKey(row, &k); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &k, nil
}

// TouchAPIKey 更新最后使用时间。
func (s *Store) TouchAPIKey(ctx context.Context, id int64) {
	_, _ = s.pool.Exec(ctx, `UPDATE api_keys SET last_used_at=now() WHERE id=$1`, id)
}
