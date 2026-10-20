#!/usr/bin/env bash
# 将 MapServer 安装为 systemd 服务（单二进制 + 内嵌前端）。
#
#   sudo bash scripts/install-systemd.sh
#
# 可选环境变量：
#   INSTALL_DIR=/opt/mapserver   BINARY=bin/mapserver
#   DATABASE_URL=...             HTTP_ADDR=:8420
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/mapserver}"
BINARY="${BINARY:-bin/mapserver}"
SERVICE_NAME="mapserver"
SERVICE_USER="mapserver"

if [[ $EUID -ne 0 ]]; then
  echo "请使用 root 运行：sudo bash scripts/install-systemd.sh" >&2
  exit 1
fi

if [[ ! -x "$BINARY" ]]; then
  echo "未找到可执行文件 $BINARY，请先执行 make build" >&2
  exit 1
fi

echo "==> 创建用户 $SERVICE_USER"
if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
fi

echo "==> 安装到 $INSTALL_DIR"
mkdir -p "$INSTALL_DIR/data"
install -m 0755 "$BINARY" "$INSTALL_DIR/mapserver"
install -m 0644 README.md "$INSTALL_DIR/README.md" 2>/dev/null || true
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"

if [[ ! -f "$INSTALL_DIR/.env" ]]; then
  echo "==> 生成 $INSTALL_DIR/.env"
  JWT_SECRET="$(head -c 32 /dev/urandom | base64 | tr -d '\n=')"
  cat > "$INSTALL_DIR/.env" <<EOF
HTTP_ADDR=${HTTP_ADDR:-:8420}
DATABASE_URL=${DATABASE_URL:-postgres://map:map@localhost:5432/mapserver?sslmode=disable}
CORS_ORIGINS=${CORS_ORIGINS:-*}
JWT_SECRET=${JWT_SECRET}
ADMIN_USERNAME=${ADMIN_USERNAME:-admin}
ADMIN_PASSWORD=${ADMIN_PASSWORD:-admin123}
TILE_CACHE_DIR=${INSTALL_DIR}/data/tile-cache
EOF
  chmod 0600 "$INSTALL_DIR/.env"
  chown "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR/.env"
fi

echo "==> 安装 systemd 单元"
install -m 0644 scripts/mapserver.service "/etc/systemd/system/${SERVICE_NAME}.service"
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

sleep 2
systemctl --no-pager --full status "$SERVICE_NAME" || true
echo
echo "完成。常用命令："
echo "  systemctl status $SERVICE_NAME"
echo "  journalctl -u $SERVICE_NAME -f"
echo "  编辑配置：$INSTALL_DIR/.env 后 systemctl restart $SERVICE_NAME"
