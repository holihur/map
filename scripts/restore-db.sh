#!/usr/bin/env bash
# 从备份恢复：scripts/restore-db.sh ./backups/mapserver-xxxx.dump
set -euo pipefail

if [ $# -lt 1 ]; then
  echo "用法: $0 <backup.dump>" >&2
  exit 1
fi
DATABASE_URL="${DATABASE_URL:-postgres://map:map@localhost:5432/mapserver?sslmode=disable}"
pg_restore --clean --if-exists --no-owner --dbname="$DATABASE_URL" "$1"
echo "恢复完成: $1"
