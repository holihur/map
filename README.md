# MapServer

一个参考 **Mapbox**（矢量瓦片地图渲染）与 **GeoServer**（地理数据服务）的开源地图服务。

- **前端**：React + Vite + TypeScript + TailwindCSS + shadcn/ui + MapLibre GL（渲染 MVT 矢量瓦片）
- **后端**：Go（chi 路由）+ pgx
- **数据库**：PostgreSQL + PostGIS（`ST_AsMVT` 生成矢量瓦片，`ST_AsGeoJSON` 输出 GeoJSON）

## 功能

| 能力 | 说明 |
| --- | --- |
| 图层管理 | 创建 / 查询 / 更新 / 删除图层，自动建物理表并建立 GiST 空间索引 |
| 要素 CRUD | GeoJSON FeatureCollection 查询、新增、更新、删除；bbox / 属性过滤 / 分页 / 排序 |
| 多坐标系 (CRS) | 图层可存任意 EPSG 坐标系；输入/输出/bbox 可指定 SRID，服务端自动重投影；支持图层坐标系转换 |
| 矢量瓦片 | `GET /api/tiles/{layer}/{z}/{x}/{y}.mvt`，基于 PostGIS `ST_AsMVT`，属性自动展平 |
| 瓦片矩阵集 | 内置 TileMatrixSet（`WebMercatorQuad` / `WorldCRS84Quad`），可按矩阵集请求 MVT：`/api/tiles/{layer}/{tms}/{z}/{x}/{y}.mvt` |
| WFS | `GET /api/wfs` 提供 GetCapabilities / DescribeFeatureType（XSD 或 JSON Schema）/ GetFeature（GeoJSON 或 GML，支持 BBOX、FILTER） |
| WFS-T | `POST /api/wfs` 支持事务写入：Insert / Update / Delete（GML 或 WKT 几何 + FES Filter） |
| SLD 样式库 | 上传/保存 SLD，图层可指定默认样式；WMS `STYLES=名称` 或 `SLD_BODY` 应用，**按比例尺选规则** |
| 导入导出 | GeoJSON / CSV / Shapefile(.zip) / GeoPackage 双向往返，自动识别格式 |
| 磁盘缓存 | 栅格瓦片内存 + 磁盘两级缓存，带容量上限与 LRU 淘汰；写入要素时自动失效 |
| 可观测 | `GET /metrics`（Prometheus 文本格式）与 `GET /api/stats` |
| 部署 | 单二进制内嵌前端；提供 systemd 单元与一键安装脚本 |
| WMS 栅格出图 | `GET /api/wms` 提供 GetCapabilities / GetMap / **GetFeatureInfo**（PNG），**支持任意 EPSG 输出坐标系**，多图层叠加，支持 **SLD_BODY** 简单样式 |
| 栅格瓦片 | `GET /api/tiles/{layer}/{z}/{x}/{y}.png` 输出透明 PNG 瓦片，可直接用作 XYZ 图层 |
| WMTS | `GET /api/wmts` 提供 GetCapabilities 与 GetTile（KVP + RESTful，PNG / MVT） |
| 投影视图 | 前端 OpenLayers + proj4 加载 `WorldCRS84Quad` 矢量瓦片，并客户端重投影到 4326 / 3857 / 3413 等 |
| 移动端适配 | 响应式布局：移动端顶栏 + 底部导航 + 抽屉面板；触摸友好的绘制工具栏 |
| 底图与工作区 | OSM / CARTO 浅色 / CARTO 深色 / Esri 卫星影像 / 纯色底图；视角、底图、图层顺序、可见性、透明度自动保存 |
| 图层控制 | 拖拽/上下移动排序、透明度滑块、缩放范围、标注字段、点击弹窗字段 |
| 可视化样式 | 图层样式编辑器：颜色、透明度、线宽、点半径、按字段分级着色、热力图、图例预览 |
| 地图分享 | 一键保存当前工作区并生成 `/share/{token}` 公开/私有链接，无需登录查看 |
| 地图绘制 | 在浏览器地图上绘制点 / 线 / 面并直接写入图层（MapLibre 事件驱动的轻量绘制工具） |
| 要素编辑 | 点击地图要素查看属性、拖动移动、节点编辑、吸附、撤销/重做、编辑 GeoJSON 或删除 |
| 数据工作台 | 多条件筛选、全文搜索、排序、分页、框选联动、批量删除/批量改字段、按筛选导出 |
| 空间分析 | 缓冲区 / 质心 / 凸包 / 外包矩形 / 几何简化，结果导出 GeoJSON |
| 搜索定位 | 图层名、标题、描述与文本属性全文搜索，结果一键定位 |
| 上传建层 | 拖拽 GeoJSON / CSV / Shapefile / GeoPackage，自动推断几何类型与字段类型，预览后一次建层导入 |
| 发布治理 | 私有图层 + 用户级读/写授权、第三方 API Key、写操作审计日志 |
| 第三方图层 | XYZ / WMS / WMTS / MVT / GeoJSON / ArcGIS；内置高德、腾讯、天地图、Google、Mapbox、Esri、CARTO、MapTiler、HERE、TomTom、OpenStreetMap 等服务商预设 |
| CORS 代理 | 第三方瓦片/GeoJSON 可经后端代理，自动替换 z/x/y、TMS 负 Y、WMS BBOX，避免浏览器跨域限制并缓存 |
| 服务商预览 | `/providers` 页用 OpenLayers 预览全部服务商；百度 BD-09M 自定义投影在此视图中正确显示，并可一键添加到主地图（MapLibre 兼容项） |
| 自然语言助手 | BYOK（用户自带模型 Key）+ Go 工具调用（ADK 风格）；支持查询、统计、空间分析、定位、打开数据表、批量改/删；默认 DeepSeek |
| OGC API Features | 现代 REST/JSON 接口：`/ogcapi`、collections、items、分页/next/prev、bbox/bbox-crs、sortby、Content-Crs，与 WFS 并存 |
| 生产就绪 | 健康/就绪探针、按用户/API Key 限流、TLS、HSTS、CSP、AI Base URL SSRF 防护、Docker/K8s、备份恢复、k6 压测脚本 |
| 瓦片性能 | 按缩放级别简化几何、单瓦片要素上限、低缩放级别自动收紧，避免大数据拖垮瓦片 |
| 鉴权与权限 | JWT 登录 + RBAC（admin / editor / viewer），写操作受保护 |
| 瓦片缓存 | 内存 LRU + TTL 缓存，图层要素变更时按图层失效 |
| 元数据缓存 | 图层 / 样式 / 能力文档缓存 + `ETag` / `304` 协商缓存 |
| 样式管理 | 保存 MapLibre Style 规范样式，支持默认样式 |
| 服务能力 | `GET /api/capabilities` 输出类似 WMS GetCapabilities 的能力文档；`GET /api/stats` 输出缓存统计 |

## 权限模型

| 角色 | 读取 | 编辑要素/样式/图层 | 删除图层 | 用户管理 |
| --- | :---: | :---: | :---: | :---: |
| viewer 查看者 | ✅ | ❌ | ❌ | ❌ |
| editor 编辑者 | ✅ | ✅ | ❌ | ❌ |
| admin 管理员 | ✅ | ✅ | ✅ | ✅ |

读取类接口（图层 / 要素 / 瓦片 / 样式 / 能力）默认公开，便于像 GeoServer 一样被第三方地图客户端直接消费；
写操作需要 `Authorization: Bearer <token>`。

## 快速开始（使用本地 PostgreSQL）

环境要求：Go 1.22+、Node 20+ / pnpm、本地 PostgreSQL（脚本会自动安装 PostGIS）。

### 方式一：开发模式（前后端分离 + Vite 热更新）

```bash
# 1. 初始化本地数据库（建角色 map、库 mapserver，并启用 PostGIS）
make db                      # 等价于 bash scripts/setup-local-db.sh

# 2. 启动后端（自动执行迁移与种子数据），默认 http://localhost:8420
make backend

# 3. 启动前端开发服务器，默认 http://localhost:5173
make frontend
```

打开 http://localhost:5173，使用默认管理员 **admin / admin123** 登录（首次启动自动创建，请尽快修改）。

### 方式二：单二进制部署（前端由 Go 托管）

```bash
make db
make serve        # 构建前端 → 内嵌进 Go → 启动，浏览器访问 http://localhost:8420
```

`make serve` 会：构建前端 → 把 `frontend/dist` 复制到 `backend/internal/web/dist` → `go build`（`go:embed` 内嵌）→ 启动。
产物为 **单个可执行文件** `bin/mapserver`，无需单独部署前端，也无需 Node 运行时。

前端资源解析顺序（自高到低）：

1. **内嵌资源**（`go:embed`，构建时写入，优先）；
2. `STATIC_DIR` 指定的目录；
3. 自动探测 `frontend/dist` 等常见路径。

未命中且无扩展名的路径（如 `/map`）会回退到 `index.html`（SPA 路由）；缺失的 `*.js`/`*.css` 返回 404；
`/assets/*` 启用一年期 `immutable` 缓存，`index.html` 使用 `no-cache`。

### 脚本做了什么

`scripts/setup-local-db.sh` 会：

1. 检查并按需安装 `postgresql-<ver>-postgis-3`；
2. 创建登录角色 `map`（密码 `map`）与数据库 `mapserver`；
3. 在库中执行 `CREATE EXTENSION postgis`。

可通过环境变量覆盖：`DB_USER`、`DB_PASS`、`DB_NAME`、`PG_VERSION`。

### 配置项

把 [`backend/.env.example`](backend/.env.example) 复制为 `backend/.env` 即可自动加载。主要变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `HTTP_ADDR` | `:8420` | 后端监听地址 |
| `DATABASE_URL` | `postgres://map:map@localhost:5432/mapserver` | 数据库连接串 |
| `CORS_ORIGINS` | `http://localhost:5173,...` | 允许跨域来源（逗号分隔） |
| `JWT_SECRET` | `dev-secret-change-me` | **JWT 签名密钥（生产必须修改，否则拒绝启动）** |
| `APP_ENV` | `development` | 设为 `production` 时强制要求非默认密钥 |
| `TOKEN_TTL_HOURS` | `72` | 登录 Token 有效期 |
| `RATE_LIMIT_RPS` / `RATE_LIMIT_BURST` | `30` / `60` | 按 IP 限流（0 禁用） |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `admin` / `admin123` | 首次启动自动创建的管理员 |
| `AI_BASE_URL` | `https://api.deepseek.com/v1` | 默认模型服务（OpenAI 兼容） |
| `AI_MODEL` | `deepseek-chat` | 默认模型 |
| `AI_TIMEOUT_SECONDS` | `60` | 模型请求超时 |
| `AI_ALLOW_PRIVATE` | `false` | 允许 AI Base URL 访问私有网段（本地 Ollama/内网网关） |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | 空 | 直接启用 HTTPS（留空由反向代理终止 TLS） |
| `TILE_MAX_FEATURES` | `5000` | 单个矢量瓦片最多编码要素数（低缩放自动收紧） |
| `TILE_SIMPLIFY` | `true` | 是否按瓦片分辨率简化几何 |
| `TILE_CACHE_ENTRIES` | `2048` | 瓦片缓存条目数（0 禁用） |
| `TILE_CACHE_TTL_SECONDS` | `300` | 瓦片缓存有效期 |
| `META_CACHE_TTL_SECONDS` | `30` | 元数据缓存有效期 |
| `TILE_CACHE_DIR` | `./data/tile-cache` | 栅格瓦片磁盘缓存目录（留空禁用） |
| `TILE_DISK_TTL_SECONDS` | `3600` | 磁盘缓存有效期 |
| `TILE_CACHE_MAX_MB` | `256` | 磁盘缓存容量上限（MB，0 不限，超过按 LRU 淘汰） |
| `STATIC_DIR` | 空 | 前端静态资源目录（空则优先后端内嵌资源） |

### 缓存策略

- **瓦片缓存**：内存 LRU + TTL，键为 `图层/矩阵集/z/x/y/extent`；对某图层写入要素后自动失效该图层全部瓦片。
  响应头 `X-Tile-Cache: HIT|MISS` 可直接观察命中情况。
- **磁盘缓存**：栅格 PNG 瓦片额外落地到 `TILE_CACHE_DIR`（原子写入、TTL、按图层失效），重启后仍可复用。
- **元数据缓存**：图层列表 / 样式 / 能力文档带 `ETag`，客户端用 `If-None-Match` 获得 `304`。
- 运行统计见 `GET /api/stats`，管理员可 `POST /api/cache/purge` 清空。

### 可选：Docker 运行数据库

```bash
make docker-db   # docker compose up -d db（需自行拉取 postgis 镜像）
```

## 发布与鉴权

- **私有图层**：`PUT /api/layers/{name}/access` 设置 `visibility=private` 并授权用户（`read` / `write`）；匿名请求将返回 403。
- **API Key**：管理员在「发布」页创建，第三方请求使用 `X-API-Key: ms_...` 或 `?api_key=ms_...`；可限定到单个图层。
- **分享地图**：地图页「分享」生成 `/share/{token}`，保存视角、底图、图层顺序/可见性/透明度；公开链接无需登录。
- **审计**：所有写操作（POST/PUT/PATCH/DELETE）自动记录用户、路径、状态、IP，管理员在「发布 → 审计日志」查看。

## 第三方图层与地图服务商

支持把第三方地图作为**底图或叠加层**，不需要导入 PostGIS：

- **类型**：XYZ 栅格瓦片、WMS、WMTS、MVT 矢量瓦片、GeoJSON URL、ArcGIS 瓦片。
- **内置服务商**：OpenStreetMap、CARTO、Esri、OpenTopoMap、Google、Mapbox、MapTiler、HERE、TomTom、高德、腾讯、天地图、百度（OpenLayers 投影视图）。
- **Token 预设**：Mapbox / MapTiler / HERE / TomTom / 天地图在添加时填写自己的 Key，保存后写入图层 URL。
- **代理**：默认经 `/api/external-layers/{name}/tile/...` 代理，自动处理 `{z}/{x}/{y}`、TMS `{-y}`、`{bbox-epsg-3857}`、WMS `BBOX`，避免 CORS 并缓存。
- **坐标系说明**：高德 / 腾讯 / Google 国内数据为 GCJ-02，与 WGS84 数据叠加存在偏移；百度使用 BD-09M，MapLibre 无法直接渲染，请在 `/providers` 页用 OpenLayers 查看。

入口：**图层 → 第三方图层 → 添加**，或 **服务商** 页预览后一键添加到主地图。

## 自然语言地图助手（BYOK）

地图页点击「AI 助手」，使用**你自己的模型 API Key**（Bring Your Own Key），后端只做代理与工具调用，不落库、不写日志。

- **默认模型**：DeepSeek `https://api.deepseek.com/v1` + `deepseek-chat`。
- **兼容服务**：OpenAI、Moonshot、智谱 GLM、Ollama 等任何 OpenAI 兼容 `/chat/completions` 接口。
- **工具（ADK 风格）**：`list_layers`、`query_features`、`layer_stats`、`search`、`analyze_features`、`map_action`、`update_features`、`delete_features`。
- **地图动作**：定位图层、飞到 bbox、高亮要素、打开数据表、切换图层可见性。
- **写操作保护**：默认关闭；只有用户在前端打开「允许写操作」且账号具备 editor 权限、图层写权限时，AI 才能批量修改/删除，且必须提供 ids 或 filters/bbox。
- **二次确认**：默认开启。AI 只生成 5 分钟有效的 HMAC 签名待确认令牌，用户点击「确认执行」后由 `/api/ai/confirm` 真正执行，避免模型直接删改数据。
- **密钥存储**：仅保存在浏览器 `localStorage`，随请求头 `X-AI-API-Key` 发送；服务端不持久化。

示例：

```bash
# 前端会自动带 JWT 与 BYOK 头；这里演示协议
curl -X POST localhost:8420/api/ai/chat \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-AI-API-Key: $DEEPSEEK_API_KEY" \
  -H "X-AI-Base-URL: https://api.deepseek.com/v1" \
  -H "X-AI-Model: deepseek-chat" \
  -H 'Content-Type: application/json' \
  -d '{"messages":[{"role":"user","content":"查找人口超过 1000 万的城市并定位到第一个"}],"allowWrite":false}'
```

## 生产部署

### Docker Compose

```bash
# 生产环境务必改掉 JWT_SECRET / ADMIN_PASSWORD
JWT_SECRET="$(openssl rand -hex 32)" ADMIN_PASSWORD='your-strong-password' \
  docker compose up -d --build
```

- 浏览器访问 `http://localhost:8420`
- 数据库与瓦片缓存分别持久化到 `mapserver_pgdata` / `mapserver_data`
- `APP_ENV=production` 时会拒绝默认 JWT 密钥

### Kubernetes

```bash
kubectl apply -f deploy/k8s.yaml
```

包含 Deployment（2 副本、非 root、只读根文件系统）、Service、Ingress、PVC、HPA、健康探针。使用前修改镜像地址、域名与 Secret。

### HTTPS

- 反向代理（Nginx / Ingress）终止 TLS 时，设置 `X-Forwarded-Proto: https`，服务会自动返回 HSTS；
- 或直接配置 `TLS_CERT_FILE` / `TLS_KEY_FILE` 由 Go 启动 HTTPS。

### 备份 / 恢复

```bash
DATABASE_URL=... ./scripts/backup-db.sh     # 默认保留 14 天
DATABASE_URL=... ./scripts/restore-db.sh ./backups/mapserver-xxxx.dump
```

### 冒烟与压测

```bash
./scripts/e2e-smoke.sh                       # API 端到端冒烟
k6 run scripts/loadtest.js                   # 瓦片 + 要素 + OGC API 压测
```

### 安全加固清单

- 修改默认 `JWT_SECRET`、`ADMIN_PASSWORD`，启用 HTTPS
- 按用户/API Key 限流已内置；生产可在网关再加一层
- AI BYOK 默认禁止访问私有网段；使用本地 Ollama 时设置 `AI_ALLOW_PRIVATE=true`
- 第三方图层默认服务端代理，匿名响应不返回 `sourceUrl`，避免泄露 Token
- 定期执行 `govulncheck ./...`（CI 已包含）并更新依赖

## API

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/health` | 公开 | 健康检查 |
| POST | `/api/auth/login` | 公开 | 登录，返回 JWT |
| GET | `/api/auth/me` | 登录 | 当前用户 |
| GET | `/api/capabilities` | 公开 | 服务能力文档（ETag） |
| GET | `/api/crs` | 公开 | 搜索支持的坐标系（`q`、`limit`） |
| GET | `/api/crs/{srid}` | 公开 | 坐标系详情 |
| GET | `/api/stats` | 公开 | 缓存运行统计 |
| POST | `/api/cache/purge` | admin | 清空全部缓存 |
| GET | `/api/layers` | 公开 | 图层列表（ETag） |
| POST | `/api/layers` | editor | 创建图层 |
| GET | `/api/layers/{name}` | 公开 | 图层详情 |
| PATCH | `/api/layers/{name}` | editor | 更新图层 |
| DELETE | `/api/layers/{name}` | admin | 删除图层（DROP 物理表） |
| POST | `/api/layers/{name}/transform` | editor | 转换图层坐标系（重写 geom 列） |
| GET | `/api/layers/{name}/bounds` | 公开 | 数据范围 |
| GET | `/api/layers/{name}/features` | 公开 | 要素查询（GeoJSON） |
| POST | `/api/layers/{name}/features` | editor | 新增要素（`inputSrid` 指定输入坐标系，默认 4326） |
| GET/PUT/DELETE | `/api/layers/{name}/features/{fid}` | 读公开 / 写 editor | 要素详情 / 更新 / 删除（`outSrid` / `inputSrid`） |
| GET | `/api/tiles/{name}/{z}/{x}/{y}.mvt` | 公开 | 矢量瓦片（默认 WebMercatorQuad，内存缓存，`X-Tile-Cache`） |
| GET | `/api/tiles/{name}/{tms}/{z}/{x}/{y}.mvt` | 公开 | 按指定瓦片矩阵集输出矢量瓦片 |
| GET | `/api/tilematrixsets` | 公开 | 支持的瓦片矩阵集列表 |
| GET | `/api/tiles/{name}/{z}/{x}/{y}.png` | 公开 | 栅格 PNG 瓦片（默认 WebMercatorQuad） |
| GET | `/api/tiles/{name}/{tms}/{z}/{x}/{y}.png` | 公开 | 按指定瓦片矩阵集输出栅格 PNG 瓦片 |
| GET | `/api/wmts` | 公开 | WMTS 1.0.0：GetCapabilities / GetTile（KVP） |
| GET | `/api/wmts/rest/{layer}/{style}/{tms}/{z}/{y}/{x}.{ext}` | 公开 | WMTS RESTful（ext = png / mvt） |
| GET | `/api/wfs` | 公开 | WFS 2.0.0：GetCapabilities / DescribeFeatureType / GetFeature |
| POST | `/api/wfs` | editor | WFS-T 事务（Insert / Update / Delete） |
| GET | `/api/layers/{name}/export` | 公开 | 导出（`format=geojson|csv|shapefile|gpkg`，支持当前筛选/bbox/q） |
| POST | `/api/layers/{name}/import` | editor | 导入（multipart `file`，可带 `srid`、gpkg 的 `table`、字段 `mapping`） |
| POST | `/api/layers/import` | editor | 上传文件自动推断 schema 建层并导入；`preview=true` 仅预览 |
| POST | `/api/layers/reorder` | editor | 按 `{names:[...]}` 重排图层顺序 |
| GET/PUT | `/api/layers/{name}/access` | 读公开 / 写 editor | 图层发布设置：public/private 与用户授权 |
| GET | `/api/layers/{name}/stats` | 公开 | 字段统计（min/max/distinct/top values） |
| POST | `/api/layers/{name}/features/batch` | editor | 按 ids/筛选批量删除或更新属性 |
| POST | `/api/layers/{name}/analyze` | 公开 | 空间分析：buffer / centroid / convexhull / envelope / simplify |
| GET | `/api/search?q=` | 公开 | 图层与文本属性搜索（可选 `layer`、`limit`） |
| POST/GET | `/api/maps` | 登录 | 保存地图工作区 / 列出自己的分享地图 |
| GET | `/api/maps/{token}` | 公开 | 读取公开分享地图（私有需登录且为创建者/管理员） |
| PUT/DELETE | `/api/maps/{token}` | 登录 | 更新 / 删除分享地图 |
| GET/POST | `/api/apikeys` | admin | 列出 / 创建第三方 API Key（明文仅返回一次） |
| POST/DELETE | `/api/apikeys/{id}` | admin | 吊销 / 删除 API Key |
| GET | `/api/audit` | admin | 写操作审计日志（`limit`、`layer`） |
| GET/POST | `/api/external-layers` | 读公开 / 写 editor | 第三方图层列表 / 创建（XYZ、WMS、WMTS、MVT、GeoJSON、ArcGIS） |
| GET/PATCH/DELETE | `/api/external-layers/{name}` | 读公开 / 写 editor | 第三方图层详情 / 更新 / 删除 |
| GET | `/api/external-layers/{name}/tile/{z}/{x}/{y}` | 公开 | 代理第三方瓦片（解决 CORS，支持 `{-y}`、WMS BBOX） |
| GET | `/api/external-layers/{name}/data` | 公开 | 代理第三方 GeoJSON 数据 |
| POST | `/api/ai/chat` | 登录 | BYOK 自然语言地图助手（OpenAI 兼容 / DeepSeek，工具调用） |
| GET | `/ogcapi` | 公开 | OGC API Features 落地页 |
| GET | `/ogcapi/conformance` | 公开 | 一致性声明 |
| GET | `/ogcapi/collections` | 公开 | 要素集合列表 |
| GET | `/ogcapi/collections/{name}` | 公开 | 集合元数据 |
| GET | `/ogcapi/collections/{name}/items` | 公开 | 要素查询（limit/offset/bbox/bbox-crs/sortby/q/f.*） |
| GET | `/ogcapi/collections/{name}/items/{fid}` | 公开 | 单个要素 |
| GET | `/healthz` / `/readyz` | 公开 | 存活 / 就绪探针（readyz 检查数据库） |
| GET | `/api/export?format=gpkg` | 公开 | 全部图层导出为单个 GeoPackage |
| GET | `/metrics` | 公开 | Prometheus 指标 |
| POST | `/api/styles/sld?name=` | editor | 上传 SLD 文档到样式库 |
| GET | `/api/styles/{name}` | 公开 | 按名称获取样式（含 SLD） |
| GET | `/api/wms` | 公开 | WMS 1.3.0：GetCapabilities / GetMap / GetFeatureInfo |
| GET/POST | `/api/styles` | 读公开 / 写 editor | 样式列表 / 创建（ETag） |
| PUT/DELETE | `/api/styles/{id}` | editor | 样式更新 / 删除 |
| GET/POST | `/api/users` | admin | 用户列表 / 创建 |
| PUT/DELETE | `/api/users/{id}` | admin | 用户更新 / 删除 |

要素查询参数：`bbox=minx,miny,maxx,maxy`、`bboxSrid`（bbox 所在坐标系，默认 4326）、`outSrid`（输出坐标系，默认 4326）、
`limit`、`offset`、`orderBy`、`order`、`simplify`；
属性过滤使用 `f.<字段>` 或 `f.<字段>.<运算符>`，运算符支持 `eq neq gt gte lt lte like isnull notnull`。
例如：`/api/layers/cities/features?f.population.gte=10000000&f.country.like=中`

## 坐标系 (CRS)

后端基于 PostGIS `spatial_ref_sys` 支持任意 EPSG 坐标系，前端提供可搜索的坐标系选择器。

- **存储**：每个图层有自己的 `srid`，几何按该坐标系存储（建表时 `geometry(<类型>, <srid>)`）。
- **输入**：`POST/PUT` 要素时用 `?inputSrid=` 声明 GeoJSON 坐标的坐标系（默认 `4326`，符合 GeoJSON 规范），
  服务端自动 `ST_Transform` 到图层坐标系。前端地图绘制得到的经纬度也以 `inputSrid=4326` 提交。
- **输出**：`GET .../features?outSrid=` 指定返回坐标系（默认 4326），响应体带 GeoJSON 顶层 `crs` 成员。
- **空间过滤**：`bbox` 可用 `bboxSrid` 声明其坐标系，服务端转换到图层坐标系后走 GiST 索引（`&&`）。
- **瓦片**：MVT 规范要求 Web Mercator，故瓦片**始终输出 EPSG:3857**，服务端自动重投影；
  MapLibre 以 3857 渲染，因此无论图层存什么坐标系，地图都能正确显示。
- **坐标系转换**：`POST /api/layers/{name}/transform {"srid": 4490}` 会将 `geom` 列整体
  `ALTER ... USING ST_Transform(...)` 重投影并更新元数据（自动重建空间索引）。

示例：

```bash
# 查询支持的坐标系
curl 'localhost:8420/api/crs?q=CGCS2000'

# 查看图层（含 srid）
curl localhost:8420/api/layers

# 地图/瓦片统一 3857；数据接口可在 4326 与其它坐标系间自由转换
curl 'localhost:8420/api/layers/forest/features?outSrid=3857'
curl 'localhost:8420/api/layers/forest/features?bbox=116,39,117,40&bboxSrid=4326'

# 把图层从 3857 转为 4326
curl -X POST localhost:8420/api/layers/forest/transform \
  -H "Authorization: Bearer $TOKEN" -d '{"srid":4326}'
```

## WMS 栅格出图

`GET /api/wms` 提供极简 WMS 1.3.0，服务端用 PostGIS 栅格化后输出 PNG，**支持任意 EPSG**。

```bash
# 能力文档
curl 'localhost:8420/api/wms?SERVICE=WMS&REQUEST=GetCapabilities'

# 全球（EPSG:4326，1.3.0 轴顺序为 lat,lon）
curl 'localhost:8420/api/wms?SERVICE=WMS&VERSION=1.3.0&REQUEST=GetMap&LAYERS=countries&CRS=EPSG:4326&BBOX=-90,-180,90,180&WIDTH=800&HEIGHT=400&FORMAT=image/png&TRANSPARENT=true'

# Web Mercator
curl 'localhost:8420/api/wms?SERVICE=WMS&VERSION=1.3.0&REQUEST=GetMap&LAYERS=countries,cities&CRS=EPSG:3857&BBOX=-20037508,-20037508,20037508,20037508&WIDTH=800&HEIGHT=400&FORMAT=image/png'

# 任意坐标系（如 CGCS2000）
curl 'localhost:8420/api/wms?SERVICE=WMS&VERSION=1.3.0&REQUEST=GetMap&LAYERS=countries&CRS=EPSG:4490&BBOX=70,10,140,55&WIDTH=800&HEIGHT=400&FORMAT=image/png'
```

参数：`LAYERS`（逗号分隔可叠加）、`CRS/SRS`、`BBOX`、`WIDTH`/`HEIGHT`（≤4096）、`FORMAT=image/png`、
`TRANSPARENT`、`BGCOLOR`、`SLD_BODY`。颜色取自图层样式的 `fill-color` / `circle-color` / `line-color`，
若提供 `SLD_BODY` 则用其中的 `fill`/`stroke`/`fill-opacity` 覆盖。
实现：`ST_AsRaster` 栅格化 + `ST_Union` 铺满画布 + `ST_DumpValues` 取掩膜，在 Go 中用 `image.NRGBA` 着色合成 32 位 PNG，
**不依赖 GDAL**（故需启用 `postgis_raster` 扩展，已由迁移 `0004_raster.sql` 自动启用）。

### GetFeatureInfo

```bash
# 点击 (I,J) 像素，返回附近要素（GeoJSON）
curl 'localhost:8420/api/wms?SERVICE=WMS&VERSION=1.3.0&REQUEST=GetFeatureInfo&QUERY_LAYERS=countries&CRS=EPSG:4326&BBOX=-90,-180,90,180&WIDTH=360&HEIGHT=180&I=296&J=50&INFO_FORMAT=application/json'
```

像素坐标先换算为地图坐标，再按 5 像素容差在图层坐标系下用 `ST_DWithin` 查询（自动走 GiST 索引）。

### SLD_BODY 样式覆盖

```bash
curl -G 'localhost:8420/api/wms' \
  --data-urlencode 'SERVICE=WMS' --data-urlencode 'VERSION=1.3.0' --data-urlencode 'REQUEST=GetMap' \
  --data-urlencode 'LAYERS=countries' --data-urlencode 'CRS=EPSG:4326' \
  --data-urlencode 'BBOX=-90,-180,90,180' --data-urlencode 'WIDTH=800' --data-urlencode 'HEIGHT=400' \
  --data-urlencode 'FORMAT=image/png' \
  --data-urlencode 'SLD_BODY=<StyledLayerDescriptor><NamedLayer><UserStyle><FeatureTypeStyle><Rule><PolygonSymbolizer><Fill><CssParameter name="fill">#ff0000</CssParameter><CssParameter name="fill-opacity">0.8</CssParameter></Fill></PolygonSymbolizer></Rule></FeatureTypeStyle></UserStyle></NamedLayer></StyledLayerDescriptor>'
```

## 栅格瓦片与 WMTS

```bash
# 栅格 PNG 瓦片（XYZ）
curl localhost:8420/api/tiles/countries/2/3/1.png                 # WebMercatorQuad
curl localhost:8420/api/tiles/countries/WorldCRS84Quad/0/0/0.png

# WMTS GetCapabilities
curl 'localhost:8420/api/wmts?SERVICE=WMTS&REQUEST=GetCapabilities'

# WMTS GetTile（KVP）
curl -o t.png 'localhost:8420/api/wmts?SERVICE=WMTS&REQUEST=GetTile&LAYER=countries&TILEMATRIXSET=WebMercatorQuad&TILEMATRIX=2&TILEROW=1&TILECOL=3&FORMAT=image/png'

# WMTS RESTful（png / mvt）
curl -o t.png localhost:8420/api/wmts/rest/countries/default/WebMercatorQuad/2/1/3.png
curl -o t.mvt localhost:8420/api/wmts/rest/countries/default/WorldCRS84Quad/0/0/0.mvt
```

## 瓦片矩阵集（TileMatrixSet）

| ID | CRS | 方案 | 说明 |
| --- | --- | --- | --- |
| `WebMercatorQuad` | EPSG:3857 | 2^z × 2^z | 默认，MapLibre 可直接使用 |
| `WorldCRS84Quad` | CRS:84 | 2^(z+1) × 2^z | 经纬度全球网格 |

```bash
curl localhost:8420/api/tilematrixsets

# 默认（等价于 WebMercatorQuad）
curl localhost:8420/api/tiles/countries/2/3/1.mvt

# 指定矩阵集
curl localhost:8420/api/tiles/countries/WebMercatorQuad/2/3/1.mvt
curl localhost:8420/api/tiles/countries/WorldCRS84Quad/0/0/0.mvt
```

瓦片按矩阵集坐标系栅格化/编码，越界坐标返回 400；缓存键包含矩阵集 ID。
矢量瓦片（`.mvt`）与栅格瓦片（`.png`）共用同一套矩阵集。

## 投影视图（OpenLayers）

前端「投影」页用 OpenLayers + proj4 加载 `WorldCRS84Quad` 矢量瓦片，并在客户端重投影到：

- `EPSG:4326` 等距圆柱（与矩阵集一致，无需重投影）
- `EPSG:3857` Web 墨卡托（内置）
- `EPSG:3413` 北极极射赤面（通过 proj4 自定义注册）

OpenLayers 体积较大（~410KB gzip 124KB），已按路由懒加载并单独分包，不影响地图页首屏。

## WFS

```bash
curl 'localhost:8420/api/wfs?SERVICE=WFS&REQUEST=GetCapabilities'
curl 'localhost:8420/api/wfs?SERVICE=WFS&REQUEST=DescribeFeatureType&TYPENAMES=cities'

# GeoJSON
curl 'localhost:8420/api/wfs?SERVICE=WFS&VERSION=2.0.0&REQUEST=GetFeature&TYPENAMES=cities&COUNT=10'

# GML
curl 'localhost:8420/api/wfs?SERVICE=WFS&REQUEST=GetFeature&TYPENAMES=cities&OUTPUTFORMAT=application/gml+xml'

# BBOX 过滤
curl 'localhost:8420/api/wfs?SERVICE=WFS&REQUEST=GetFeature&TYPENAMES=cities&BBOX=115,39,117,41,EPSG:4326'

# FILTER（支持 EqualTo/NotEqualTo/LessThan/GreaterThan/Like/Between，可带 fes:/ogc: 命名空间）
curl -G localhost:8420/api/wfs \
  --data-urlencode 'SERVICE=WFS' --data-urlencode 'REQUEST=GetFeature' --data-urlencode 'TYPENAMES=cities' \
  --data-urlencode 'FILTER=<fes:Filter xmlns:fes="http://www.opengis.net/fes/2.0"><fes:PropertyIsBetween><fes:PropertyName>population</fes:PropertyName><fes:LowerBoundary><fes:Literal>10000000</fes:Literal></fes:LowerBoundary><fes:UpperBoundary><fes:Literal>14000000</fes:Literal></fes:UpperBoundary></fes:PropertyIsBetween></fes:Filter>'

# 只取计数
curl 'localhost:8420/api/wfs?SERVICE=WFS&REQUEST=GetFeature&TYPENAMES=cities&RESULTTYPE=hits'
```

## WFS-T（事务写入）

```bash
# Insert（几何为 GML，也可用 <ms:wkt>POINT(...)</ms:wkt>）
curl -X POST localhost:8420/api/wfs -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/xml' --data-binary '
<wfs:Transaction xmlns:wfs="http://www.opengis.net/wfs/2.0" xmlns:ms="http://mapserver.local/ns" xmlns:gml="http://www.opengis.net/gml/3.2" version="2.0.0">
  <wfs:Insert><ms:cities>
    <ms:geom><gml:Point><gml:pos>116.4 39.9</gml:pos></gml:Point></ms:geom>
    <ms:name>示例城市</ms:name><ms:population>1000</ms:population>
  </ms:cities></wfs:Insert>
</wfs:Transaction>'

# Update / Delete 使用 FES Filter
curl -X POST localhost:8420/api/wfs -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/xml' --data-binary '
<wfs:Transaction xmlns:wfs="http://www.opengis.net/wfs/2.0" xmlns:fes="http://www.opengis.net/fes/2.0" version="2.0.0">
  <wfs:Update typeName="cities">
    <wfs:Property><wfs:ValueReference>name</wfs:ValueReference><wfs:Value>新名称</wfs:Value></wfs:Property>
    <fes:Filter><fes:PropertyIsEqualTo><fes:PropertyName>name</fes:PropertyName><fes:Literal>示例城市</fes:Literal></fes:PropertyIsEqualTo></fes:Filter>
  </wfs:Update>
</wfs:Transaction>'
```

## SLD 样式库

```bash
# 上传 SLD
curl -X POST 'localhost:8420/api/styles/sld?name=green-countries' \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/xml' --data-binary @style.sld

# WMS 引用样式名
curl 'localhost:8420/api/wms?...&STYLES=green-countries'

# 设置图层默认样式（之后 GetMap 不传 STYLES 也会使用）
curl -X PATCH localhost:8420/api/layers/countries \
  -H "Authorization: Bearer $TOKEN" -d '{"styleName":"green-countries"}'
```

## 可观测性

```bash
curl localhost:8420/metrics          # Prometheus 文本格式
curl localhost:8420/api/stats        # JSON 统计（含磁盘缓存上限）
```

指标包括 `mapserver_http_requests_total{method,route,status}`、请求耗时、内存/磁盘缓存命中与容量。

## GeoPackage 多图层

```bash
# 全部图层写入同一个 .gpkg（每层一张表）
curl -OJ 'localhost:8420/api/export?format=gpkg'

# 导入时指定表名
curl -X POST localhost:8420/api/layers/mylayer/import \
  -H "Authorization: Bearer $TOKEN" -F 'file=@all.gpkg' -F 'table=countries'
```

## SLD 样式（WMS）

`SLD_BODY` 会按比例尺分母选择匹配的 Rule（`MinScaleDenominator ≤ scale < MaxScaleDenominator`），
并应用其 `Fill`/`Stroke`/`Size`。比例尺分母根据请求 BBOX、WIDTH 与 CRS 单位自动计算。

```bash
curl -G localhost:8420/api/wms \
  --data-urlencode 'SERVICE=WMS' --data-urlencode 'VERSION=1.3.0' --data-urlencode 'REQUEST=GetMap' \
  --data-urlencode 'LAYERS=countries' --data-urlencode 'CRS=EPSG:4326' \
  --data-urlencode 'BBOX=-90,-180,90,180' --data-urlencode 'WIDTH=800' --data-urlencode 'HEIGHT=400' \
  --data-urlencode 'FORMAT=image/png' \
  --data-urlencode 'SLD_BODY=<StyledLayerDescriptor><NamedLayer><Name>countries</Name><UserStyle><FeatureTypeStyle><Rule><Name>overview</Name><MinScaleDenominator>1000000</MinScaleDenominator><PolygonSymbolizer><Fill><CssParameter name="fill">#0000ff</CssParameter></Fill></PolygonSymbolizer></Rule><Rule><Name>detail</Name><MaxScaleDenominator>1000000</MaxScaleDenominator><PolygonSymbolizer><Fill><CssParameter name="fill">#ff0000</CssParameter></Fill></PolygonSymbolizer></Rule></FeatureTypeStyle></UserStyle></NamedLayer></StyledLayerDescriptor>'
```

## 导入 / 导出

支持 **GeoJSON / CSV / Shapefile(.zip) / GeoPackage**，导入按文件扩展名自动识别。

```bash
# 导出
curl -OJ 'localhost:8420/api/layers/cities/export?format=geojson'
curl -OJ 'localhost:8420/api/layers/cities/export?format=csv'         # 几何为 WKT 列
curl -OJ 'localhost:8420/api/layers/cities/export?format=shapefile'   # zip: shp/shx/dbf/prj
curl -OJ 'localhost:8420/api/layers/cities/export?format=gpkg'

# 导入（需 editor/admin）
curl -X POST localhost:8420/api/layers/mylayer/import \
  -H "Authorization: Bearer $TOKEN" -F 'file=@cities.zip' -F 'srid=4326'
```

实现说明：GeoPackage 使用纯 Go 的 `modernc.org/sqlite`（无 cgo）；Shapefile 的 `.shp/.shx/.dbf`
为自实现读写（支持 Point/PolyLine/Polygon，含 Z 变体读取）；CSV 几何列支持 WKT 或 `lon`/`lat` 列。

## 测试

```bash
make test              # 后端单元 + 集成测试，以及前端 Vitest

# 后端集成测试需要测试库（首次创建）
createdb -O map mapserver_test
cd backend && TEST_DATABASE_URL='postgres://map:map@localhost:5432/mapserver_test?sslmode=disable' \
  go test ./internal/api/... -v

# 前端
cd frontend && pnpm test
```

后端集成测试使用真实 PostgreSQL + PostGIS，启动时自动**重置测试库并注入固定夹具**，覆盖：

- 鉴权与 RBAC（登录 / 401 / 403 / 密码策略）、**Token 吊销与刷新**
- 图层与要素 CRUD、属性过滤、bbox、404
- 多坐标系（inputSrid / outSrid / bboxSrid / transform / 未知 SRID）
- 矢量瓦片（含内存+磁盘缓存命中）、CRS84 矩阵集、栅格 PNG、越界
- WMS（GetCapabilities / GetMap PNG / GetFeatureInfo / 错误处理）
- WFS（GetCapabilities / DescribeFeatureType / GetFeature GeoJSON + GML / hits）
- **WFS-T 原子性**（多操作中途失败应整体回滚）
- 样式与 SLD（上传校验 / 按名获取 / WMS STYLES）
- 导入导出四种格式**往返一致性 + 属性完整性**
- 指标与统计、限流 429

前端 Vitest 覆盖 API 客户端（鉴权头 / 错误映射 / 401 事件 / multipart / URL 构造 / 坐标轴序）、
登录态上下文（角色权限、登录登出、会话恢复）、`CRSInput` 与工具函数。

## 安全

- **JWT + RBAC**：`admin > editor > viewer`，写操作按角色限制
- **Token 吊销**：`users.token_version` 参与 Token 签名；`POST /api/auth/logout-all` 递增版本使全部旧 Token 失效，中间件带 30s 缓存校验；修改密码自动递增；`POST /api/auth/refresh` 续期
- **限流**：按客户端 IP 的令牌桶（`RATE_LIMIT_RPS`/`RATE_LIMIT_BURST`），超限返回 `429` + `Retry-After`
- **安全响应头**：`CSP`（适配 MapLibre/OpenLayers worker）、`X-Content-Type-Options`、`X-Frame-Options: DENY`、`Referrer-Policy`、`Permissions-Policy`、`COOP`
- **生产密钥约束**：`APP_ENV=production` 且密钥为默认值时**拒绝启动**
- **密码策略**：最少 8 位；默认管理员密码持续告警
- **SQL 注入防护**：标识符（表名/列名）统一正则校验 + `pgx.Identifier` 引用，值一律参数化

## 部署（systemd）

```bash
make build                     # 构建单二进制 bin/mapserver（内嵌前端）
sudo bash scripts/install-systemd.sh
```

安装脚本会创建 `mapserver` 系统用户、安装到 `/opt/mapserver`（含自动生成 `JWT_SECRET` 的 `.env`）、
写入并启用 `mapserver.service`。

```bash
systemctl status mapserver
journalctl -u mapserver -f
```

单元文件启用了 `NoNewPrivileges`、`ProtectSystem`、`PrivateTmp` 等加固选项，仅允许写 `/opt/mapserver/data`。

## 目录结构

```
mapserver/
├── Makefile
├── scripts/setup-local-db.sh   # 本地 PostGIS 初始化脚本
├── docker-compose.yml          # 可选：容器化 PostGIS
├── backend/                    # Go API 服务
│   ├── cmd/server/main.go
│   └── internal/
│       ├── config/             # 环境变量 + .env 加载
│       ├── auth/               # JWT / bcrypt / 角色中间件
│       ├── cache/              # 带 TTL 的 LRU 字节缓存
│       ├── db/                 # 连接池 + 迁移 + 管理员种子
│       ├── models/             # 领域模型
│       ├── store/              # 数据访问（图层 / 要素 / 瓦片 / 样式 / 用户）
│       ├── web/                # 前端静态托管（go:embed + SPA 回退）
│       │   └── dist/           # 构建时写入的前端产物（内嵌进二进制）
│       └── api/                # HTTP 路由与处理器
└── frontend/                   # React 控制台
    └── src/
        ├── components/ui/      # shadcn/ui 组件
        ├── components/map/     # MapLibre 地图与绘制工具
        ├── lib/api.ts          # 后端 API 客户端
        ├── lib/auth.tsx        # 登录态上下文
        └── pages/              # 登录 / 地图 / 图层 / 数据 / 样式 / 能力 / 用户
```

## 数据模型

每个图层的空间数据存放在独立物理表 `layer_<name>` 中（类似 GeoServer 的 featureType），
字段由图层 `schema`（`fields`）定义。矢量瓦片查询按字段展开，MVT 属性即扁平字段，
前端样式表达式可直接使用 `["get", "name"]` 等。

内置演示图层：`countries`（面）与 `cities`（点），几何为高度简化形状，仅用于演示。
