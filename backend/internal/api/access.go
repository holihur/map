package api

import (
	"context"
	"net/http"
	"strings"

	"mapserver/internal/auth"
	"mapserver/internal/models"
)

// layerReadAllowed 判断当前请求是否可读取图层。
func (h *Handlers) layerReadAllowed(ctx context.Context, l *models.Layer) bool {
	if l == nil {
		return false
	}
	if key, ok := auth.APIKeyFrom(ctx); ok {
		if key.LayerName != "" && key.LayerName != l.Name {
			return false
		}
		// 明确绑定到该图层的 API Key 即视为已授权。
		if key.LayerName == l.Name {
			return true
		}
	}
	if user, ok := auth.UserFrom(ctx); ok && user.Role == auth.RoleAdmin {
		return true
	}
	if l.Visibility != "private" {
		return true
	}
	user, ok := auth.UserFrom(ctx)
	if !ok {
		return false
	}
	level, err := h.store.LayerAccessLevel(ctx, l.ID, user.ID)
	if err != nil {
		return false
	}
	return level == "read" || level == "write"
}

// layerWriteAllowed 判断当前请求是否可写入图层。
func (h *Handlers) layerWriteAllowed(ctx context.Context, l *models.Layer) bool {
	if l == nil {
		return false
	}
	if key, ok := auth.APIKeyFrom(ctx); ok {
		if key.LayerName != "" && key.LayerName != l.Name {
			return false
		}
		if key.LayerName == l.Name && (key.Role == auth.RoleEditor || key.Role == auth.RoleAdmin) {
			return true
		}
	}
	user, ok := auth.UserFrom(ctx)
	if !ok {
		return false
	}
	if user.Role == auth.RoleAdmin {
		return true
	}
	if user.Role != auth.RoleEditor {
		return false
	}
	if l.Visibility != "private" {
		return true
	}
	level, err := h.store.LayerAccessLevel(ctx, l.ID, user.ID)
	if err != nil {
		return false
	}
	return level == "write"
}

// requireLayerWrite 在写入前校验图层级权限。
func (h *Handlers) requireLayerWrite(w http.ResponseWriter, r *http.Request, l *models.Layer) bool {
	if h.layerWriteAllowed(r.Context(), l) {
		return true
	}
	writeError(w, http.StatusForbidden, errString("没有该图层的写入权限"))
	return false
}

// lookupLayerByName 按名称读取图层并校验读权限。
func (h *Handlers) lookupLayerByName(w http.ResponseWriter, r *http.Request, name string) (*models.Layer, bool) {
	layer, err := h.store.GetLayerByName(r.Context(), name)
	if err != nil {
		writeError(w, statusForError(err), err)
		return nil, false
	}
	if !h.layerReadAllowed(r.Context(), layer) {
		writeError(w, http.StatusForbidden, errString("没有该图层的访问权限"))
		return nil, false
	}
	return layer, true
}

// 兼容路径中的逗号分隔图层（WMS LAYERS）。
func layerNameFromPath(path string) string {
	const prefix = "/api/layers/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if idx := strings.IndexAny(rest, "/?"); idx >= 0 {
		rest = rest[:idx]
	}
	return rest
}

// readableLayers 返回当前身份可读取的图层。
func (h *Handlers) readableLayers(ctx context.Context) ([]models.Layer, error) {
	all, err := h.store.ListLayers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.Layer, 0, len(all))
	for i := range all {
		if h.layerReadAllowed(ctx, &all[i]) {
			out = append(out, all[i])
		}
	}
	return out, nil
}
