package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"mapserver/internal/auth"
	"mapserver/internal/models"
)

// externalReadAllowed 判断当前请求是否可读取第三方图层。
func (h *Handlers) externalReadAllowed(ctx context.Context, l *models.ExternalLayer) bool {
	if l == nil {
		return false
	}
	if key, ok := auth.APIKeyFrom(ctx); ok {
		if key.LayerName != "" && key.LayerName != l.Name {
			return false
		}
		if key.LayerName == l.Name {
			return true
		}
	}
	if l.Visibility != "private" {
		return true
	}
	user, ok := auth.UserFrom(ctx)
	if !ok {
		return false
	}
	return user.Role == auth.RoleAdmin || user.Role == auth.RoleEditor
}

func (h *Handlers) listExternalLayers(w http.ResponseWriter, r *http.Request) {
	all, err := h.store.ListExternalLayers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]models.ExternalLayer, 0, len(all))
	for i := range all {
		if h.externalReadAllowed(r.Context(), &all[i]) {
			out = append(out, h.externalForResponse(r.Context(), all[i]))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"layers": out})
}

func (h *Handlers) getExternalLayer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	layer, err := h.store.GetExternalLayerByName(r.Context(), name)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	if !h.externalReadAllowed(r.Context(), layer) {
		writeError(w, http.StatusForbidden, errString("没有该图层的访问权限"))
		return
	}
	writeJSON(w, http.StatusOK, h.externalForResponse(r.Context(), *layer))
}

func (h *Handlers) createExternalLayer(w http.ResponseWriter, r *http.Request) {
	var req models.CreateExternalLayerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Title == "" {
		req.Title = req.Name
	}
	layer, err := h.store.CreateExternalLayer(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.invalidateMeta()
	writeJSON(w, http.StatusCreated, layer)
}

func (h *Handlers) updateExternalLayer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	layer, err := h.store.GetExternalLayerByName(r.Context(), name)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	var req models.UpdateExternalLayerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	updated, err := h.store.UpdateExternalLayer(r.Context(), layer.ID, req)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	h.invalidateMeta()
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handlers) deleteExternalLayer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	layer, err := h.store.GetExternalLayerByName(r.Context(), name)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	if err := h.store.DeleteExternalLayer(r.Context(), layer.ID); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	h.invalidateMeta()
	w.WriteHeader(http.StatusNoContent)
}

// readableExternalLayers 返回当前身份可读取的第三方图层。
func (h *Handlers) readableExternalLayers(ctx context.Context) ([]models.ExternalLayer, error) {
	all, err := h.store.ListExternalLayers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.ExternalLayer, 0, len(all))
	for i := range all {
		if h.externalReadAllowed(ctx, &all[i]) {
			out = append(out, all[i])
		}
	}
	return out, nil
}

// externalForResponse 对非编辑者隐藏 sourceUrl（其中可能包含 Mapbox/天地图等 Token）。
// 开启代理时客户端只需要图层 name 即可通过 /api/external-layers/{name}/tile 渲染。
func (h *Handlers) externalForResponse(ctx context.Context, l models.ExternalLayer) models.ExternalLayer {
	if user, ok := auth.UserFrom(ctx); ok && (user.Role == auth.RoleAdmin || user.Role == auth.RoleEditor) {
		return l
	}
	if key, ok := auth.APIKeyFrom(ctx); ok && (key.Role == auth.RoleAdmin || key.Role == auth.RoleEditor) {
		return l
	}
	var opts map[string]any
	_ = json.Unmarshal(l.SourceOptions, &opts)
	if v, ok := opts["proxy"].(bool); ok && !v {
		return l // 直连模式必须把 URL 交给浏览器
	}
	l.SourceURL = ""
	return l
}
