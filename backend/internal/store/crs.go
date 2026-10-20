package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"mapserver/internal/models"
)

// crsSelect 从 spatial_ref_sys 提取 SRID 与可读名称。
// 名称依次尝试 PROJCS / PROJCRS / GEOGCS / 复合名称中引号内的内容。
const crsSelect = `srid,
	COALESCE(auth_name, 'EPSG') AS auth_name,
	COALESCE(auth_srid, srid) AS auth_srid,
	COALESCE(
		substring(srtext from 'PROJCS\["([^"]+)"'),
		substring(srtext from 'PROJCRS\["([^"]+)"'),
		substring(srtext from 'GEOGCS\["([^"]+)"'),
		substring(srtext from 'GEOGCRS\["([^"]+)"'),
		COALESCE(auth_name, 'EPSG') || ':' || COALESCE(auth_srid, srid)::text
	) AS name`

// ListCRS 搜索支持的坐标参考系。
func (s *Store) ListCRS(ctx context.Context, query string, limit int) ([]models.CRS, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	q := strings.TrimSpace(query)

	rows, err := s.pool.Query(ctx, `
		SELECT `+crsSelect+`
		FROM spatial_ref_sys
		WHERE $1 = ''
		   OR srid::text = $1
		   OR (COALESCE(auth_name,'EPSG') || ':' || COALESCE(auth_srid, srid)::text) ILIKE '%' || $1 || '%'
		   OR srtext ILIKE '%' || $1 || '%'
		ORDER BY CASE WHEN srid IN (4326, 3857, 4490) THEN 0 ELSE 1 END, srid
		LIMIT $2`, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.CRS, 0)
	for rows.Next() {
		var c models.CRS
		if err := rows.Scan(&c.SRID, &c.AuthName, &c.AuthSRID, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCRS 返回单个坐标参考系。
func (s *Store) GetCRS(ctx context.Context, srid int) (*models.CRS, error) {
	var c models.CRS
	err := s.pool.QueryRow(ctx, `SELECT `+crsSelect+` FROM spatial_ref_sys WHERE srid=$1`, srid).
		Scan(&c.SRID, &c.AuthName, &c.AuthSRID, &c.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// SRIDExists 判断 SRID 是否存在。
func (s *Store) SRIDExists(ctx context.Context, srid int) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM spatial_ref_sys WHERE srid=$1)`, srid).Scan(&ok)
	return ok, err
}

// CRSWKT 返回指定 SRID 的 WKT 定义（用于 Shapefile .prj）。
func (s *Store) CRSWKT(ctx context.Context, srid int) (string, error) {
	var wkt string
	err := s.pool.QueryRow(ctx, `SELECT srtext FROM spatial_ref_sys WHERE srid=$1`, srid).Scan(&wkt)
	if err != nil {
		return "", err
	}
	return wkt, nil
}
