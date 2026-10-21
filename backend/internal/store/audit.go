package store

import (
	"context"
	"encoding/json"
	"strconv"

	"mapserver/internal/models"
)

// InsertAuditLog 写入一条审计记录。
func (s *Store) InsertAuditLog(ctx context.Context, entry models.AuditLog) error {
	detail := entry.Detail
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (user_id, username, role, method, path, status, layer, ip, detail)
		VALUES (NULLIF($1,0),$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9)`,
		entry.UserID, entry.Username, entry.Role, entry.Method, entry.Path,
		entry.Status, entry.Layer, entry.IP, []byte(detail))
	return err
}

// ListAuditLogs 返回最近的审计记录。
func (s *Store) ListAuditLogs(ctx context.Context, limit int, layer string) ([]models.AuditLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	sql := `SELECT id, COALESCE(user_id,0), COALESCE(username,''), COALESCE(role,''),
		method, path, status, COALESCE(layer,''), COALESCE(ip,''), detail, created_at
		FROM audit_logs`
	args := []any{}
	if layer != "" {
		args = append(args, layer)
		sql += ` WHERE layer=$1`
	}
	args = append(args, limit)
	sql += ` ORDER BY created_at DESC LIMIT $` + strconv.Itoa(len(args))
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.AuditLog, 0)
	for rows.Next() {
		var a models.AuditLog
		var detail []byte
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.Role, &a.Method, &a.Path,
			&a.Status, &a.Layer, &a.IP, &detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Detail = json.RawMessage(detail)
		out = append(out, a)
	}
	return out, rows.Err()
}
