// Package store 封装对 PostgreSQL/PostGIS 的数据访问。
package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound 表示目标资源不存在。
var ErrNotFound = errors.New("not found")

// Store 持有数据库连接池。
type Store struct {
	pool *pgxpool.Pool
}

// New 构造 Store。
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Pool 暴露底层连接池（供需要事务的场景使用）。
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// execer 抽象 pgxpool.Pool 与 pgx.Tx，使同一套方法可在事务内外复用。
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Tx 是 pgx 事务的别名。
type Tx = pgx.Tx

// Transaction 在单个数据库事务中执行 fn；fn 返回错误则回滚。
func (s *Store) Transaction(ctx context.Context, fn func(tx Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var identRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// validIdent 校验标识符（表名、列名）仅包含安全字符。
func validIdent(v string) bool { return identRe.MatchString(v) }

// quoteIdent 返回带双引号的安全标识符。
func quoteIdent(v string) string { return pgx.Identifier{v}.Sanitize() }

// columnType 将字段类型映射为 PostgreSQL 列类型。
func columnType(t string) (string, error) {
	switch strings.ToLower(t) {
	case "text", "string":
		return "text", nil
	case "integer", "int":
		return "integer", nil
	case "bigint", "long":
		return "bigint", nil
	case "double", "number", "float":
		return "double precision", nil
	case "boolean", "bool":
		return "boolean", nil
	case "timestamptz", "datetime":
		return "timestamptz", nil
	case "date":
		return "date", nil
	default:
		return "", fmt.Errorf("unsupported field type %q", t)
	}
}
