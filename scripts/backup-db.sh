#!/usr/bin/env bash
# 备份 MapServer 数据库（PostgreSQL 自定义格式，支持 pg_restore）
set -euo pipefail

DATABASE_URL="${DATABASE_URL:-postgres://map:map@localhost:5432/mapserver?sslmode=disable}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"

mkdir -p "$BACKUP_DIR"
file="$BACKUP_DIR/mapserver-$(date +%Y%m%d-%H%M%S).dump"

pg_dump "$DATABASE_URL" --format=custom --no-owner --file="$file"
echo "备份完成: $file ($(du -h "$file" | cut -f1))"

find "$BACKUP_DIR" -name 'mapserver-*.dump' -type f -mtime "+$RETENTION_DAYS" -print -delete
