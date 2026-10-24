# syntax=docker/dockerfile:1

# ── 前端构建 ───────────────────────────────────────────────
FROM node:22-alpine AS web
WORKDIR /web
RUN corepack enable && corepack prepare pnpm@11 --activate
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

# ── Go 构建（内嵌前端）─────────────────────────────────────
FROM golang:1.26-alpine AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=web /web/dist ./internal/web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mapserver ./cmd/server

# ── 运行镜像 ──────────────────────────────────────────────
FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app -u 10001 app \
    && apk add --no-cache ca-certificates tzdata wget
COPY --from=build /out/mapserver /usr/local/bin/mapserver
RUN mkdir -p /data && chown -R app:app /data
USER app
WORKDIR /data
ENV HTTP_ADDR=:8420 \
    TILE_CACHE_DIR=/data/tile-cache \
    APP_ENV=production
EXPOSE 8420
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8420/readyz || exit 1
ENTRYPOINT ["/usr/local/bin/mapserver"]
