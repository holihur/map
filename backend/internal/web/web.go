// Package web 提供前端静态资源的托管（内嵌 dist + 磁盘回退）与 SPA 路由回退。
package web

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// 构建时通过 Makefile 将 frontend/dist 复制到本目录后内嵌进二进制。
// 保留 dist/.gitkeep 以保证该目录始终存在（未构建前端时回退到磁盘）。
//
//go:embed all:dist
var embedded embed.FS

// Handler 提供单页应用的静态资源服务。
type Handler struct {
	fsys   fs.FS
	source string
}

// New 依次尝试：内嵌资源 → STATIC_DIR → 常见磁盘路径。
// 找不到任何可用前端时返回 nil（调用方据此禁用静态托管）。
func New(staticDir string) *Handler {
	if sub, err := fs.Sub(embedded, "dist"); err == nil {
		if _, err := fs.Stat(sub, "index.html"); err == nil {
			return &Handler{fsys: sub, source: "embedded"}
		}
	}

	candidates := []string{}
	if staticDir != "" {
		candidates = append(candidates, staticDir)
	}
	candidates = append(candidates,
		"frontend/dist",
		"../frontend/dist",
		"../../frontend/dist",
		"dist",
	)
	for _, dir := range candidates {
		if fi, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !fi.IsDir() {
			return &Handler{fsys: os.DirFS(dir), source: dir}
		}
	}
	return nil
}

// Enabled 表示是否找到可用的前端资源。
func (h *Handler) Enabled() bool { return h != nil }

// Source 返回资源来源（embedded 或磁盘路径），用于日志。
func (h *Handler) Source() string {
	if h == nil {
		return ""
	}
	return h.source
}

// ServeHTTP 服务静态文件；未命中的无扩展名路径回退到 index.html（SPA）。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil {
		writeJSONError(w, http.StatusNotFound, "前端资源未构建")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	// API / 健康检查不应落到前端
	if name == "health" || strings.HasPrefix(name, "api/") {
		writeJSONError(w, http.StatusNotFound, "route not found")
		return
	}

	if h.serveFile(w, r, name) {
		return
	}
	// 带扩展名的资源缺失 → 404，避免把 JS/CSS 请求回退成 HTML
	if path.Ext(name) != "" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	// SPA 路由回退
	if h.serveFile(w, r, "index.html") {
		return
	}
	writeJSONError(w, http.StatusNotFound, "not found")
}

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, name string) bool {
	f, err := h.fsys.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return false
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		return false
	}
	w.Header().Set("Cache-Control", cacheControl(name))
	http.ServeContent(w, r, name, st.ModTime(), rs)
	return true
}

// cacheControl 对带指纹的构建产物启用长期缓存，其余禁用缓存。
func cacheControl(name string) string {
	if strings.HasPrefix(name, "assets/") {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
