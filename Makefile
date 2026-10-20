.PHONY: db backend frontend frontend-build install dev build serve test tidy clean down docker-db install-service

# 初始化本地 PostgreSQL + PostGIS（建库、建角色、启用扩展）
db:
	bash scripts/setup-local-db.sh

# 后端（开发模式）运行，默认 :8420
backend:
	cd backend && go run ./cmd/server

# 前端依赖 + 开发服务器（默认 :5173，代理到后端）
frontend:
	cd frontend && pnpm install && pnpm dev

# 构建前端并把产物复制到 Go 内嵌目录
frontend-build:
	cd frontend && pnpm install && pnpm build
	mkdir -p backend/internal/web/dist
	find backend/internal/web/dist -mindepth 1 ! -name .gitkeep -delete
	cp -R frontend/dist/. backend/internal/web/dist/

# 构建单二进制（内嵌前端，前端由 Go 托管）
build: frontend-build
	cd backend && go build -o ../bin/mapserver ./cmd/server

# 构建并启动（浏览器访问 http://localhost:8420）
serve: build
	./bin/mapserver

# 安装为 systemd 服务（需 root）
install-service: build
	sudo bash scripts/install-systemd.sh

install:
	cd backend && go mod download
	cd frontend && pnpm install

tidy:
	cd backend && go mod tidy

test:
	cd backend && go test ./...
	cd frontend && pnpm test

clean:
	rm -rf bin frontend/dist
	find backend/internal/web/dist -mindepth 1 ! -name .gitkeep -delete

# 可选：使用 Docker 运行 PostGIS（需自行拉取镜像）
docker-db:
	docker compose up -d db

down:
	docker compose down
