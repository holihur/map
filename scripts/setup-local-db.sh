#!/usr/bin/env bash
# 初始化本地 PostgreSQL + PostGIS（不使用 Docker）。
# 用法：bash scripts/setup-local-db.sh
set -euo pipefail

DB_USER="${DB_USER:-map}"
DB_PASS="${DB_PASS:-map}"
DB_NAME="${DB_NAME:-mapserver}"
PG_VERSION="${PG_VERSION:-14}"

cd / 2>/dev/null || true

run_psql() { (cd / && sudo -u postgres psql "$@"); }

echo "==> 检查 PostGIS 扩展"
if ! ls /usr/share/postgresql/*/extension/postgis.control >/dev/null 2>&1; then
  echo "PostGIS 未安装，正在后台安装 postgresql-${PG_VERSION}-postgis-3 ..."
  sudo apt-get update
  sudo apt-get install -y "postgresql-${PG_VERSION}-postgis-3"
else
  echo "PostGIS 已安装，跳过。"
fi

echo "==> 创建角色 ${DB_USER}"
run_psql -v ON_ERROR_STOP=1 <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${DB_USER}') THEN
    CREATE ROLE ${DB_USER} LOGIN PASSWORD '${DB_PASS}' SUPERUSER;
  END IF;
END
\$\$;
SQL

echo "==> 创建数据库 ${DB_NAME}"
if ! run_psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
  (cd / && sudo -u postgres createdb -O "${DB_USER}" "${DB_NAME}")
else
  echo "数据库已存在，跳过。"
fi

echo "==> 启用 PostGIS 扩展"
run_psql -d "${DB_NAME}" -c "CREATE EXTENSION IF NOT EXISTS postgis;"

echo "==> 完成。连接串：postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable"
