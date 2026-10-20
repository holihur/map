package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"mapserver/internal/auth"
	"mapserver/internal/store"
)

// EnsureAdmin 在用户表为空时创建一个管理员账户。
func EnsureAdmin(ctx context.Context, pool *pgxpool.Pool, username, password string) (bool, error) {
	st := store.New(pool)
	n, err := st.CountUsers(ctx)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	if username == "" || password == "" {
		return false, fmt.Errorf("管理员用户名/密码未配置")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return false, err
	}
	if _, err := st.CreateUser(ctx, username, hash, auth.RoleAdmin); err != nil {
		return false, err
	}
	return true, nil
}
