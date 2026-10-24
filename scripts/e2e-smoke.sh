#!/usr/bin/env bash
# API 端到端冒烟测试：需要已启动的 MapServer 与默认管理员
set -euo pipefail

BASE="${BASE_URL:-http://localhost:8420}"
USER="${ADMIN_USERNAME:-admin}"
PASS="${ADMIN_PASSWORD:-admin123}"

code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
expect() {
  local want="$1" got="$2" label="$3"
  if [ "$got" != "$want" ]; then
    echo "FAIL: $label（期望 $want，实际 $got）" >&2
    exit 1
  fi
  echo "ok: $label ($got)"
}

expect 200 "$(code "$BASE/healthz")" "healthz"
expect 200 "$(code "$BASE/readyz")" "readyz"
expect 200 "$(code "$BASE/ogcapi/collections")" "OGC collections"
expect 200 "$(code "$BASE/ogcapi/collections/cities/items?limit=2")" "OGC items"
expect 200 "$(code "$BASE/api/tiles/cities/2/2/1.mvt")" "MVT tile"

TOKEN=$(curl -s -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" \
  | python3 -c 'import sys,json;print(json.load(sys.stdin).get("token",""))')
[ -n "$TOKEN" ] || { echo "登录失败" >&2; exit 1; }
echo "ok: 登录"

expect 200 "$(code -H "Authorization: Bearer $TOKEN" "$BASE/api/layers")" "图层列表"
expect 200 "$(code -H "Authorization: Bearer $TOKEN" "$BASE/api/layers/cities/features?limit=5")" "要素查询"
expect 400 "$(code -X POST "$BASE/api/ai/chat" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"messages":[{"role":"user","content":"hi"}]}')" "AI BYOK 缺少 Key"

MAP_JSON=$(curl -s -X POST "$BASE/api/maps" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"e2e-smoke","state":{"version":1,"basemap":"osm","center":[116,39],"zoom":5},"isPublic":true}')
MAP_TOKEN=$(echo "$MAP_JSON" | python3 -c 'import sys,json;print(json.load(sys.stdin).get("token",""))')
[ -n "$MAP_TOKEN" ] || { echo "创建分享地图失败" >&2; exit 1; }
expect 200 "$(code "$BASE/api/maps/$MAP_TOKEN")" "分享地图公开读取"
expect 204 "$(code -X DELETE "$BASE/api/maps/$MAP_TOKEN" -H "Authorization: Bearer $TOKEN")" "清理分享地图"

echo "冒烟测试全部通过"
