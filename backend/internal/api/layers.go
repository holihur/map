package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"mapserver/internal/models"
)

func (h *Handlers) listLayers(w http.ResponseWriter, r *http.Request) {
	writeCachedJSON(w, r, h.meta, "layers", func() ([]byte, error) {
		layers, err := h.store.ListLayers(r.Context())
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"layers": layers})
	})
}

func (h *Handlers) createLayer(w http.ResponseWriter, r *http.Request) {
	var req models.CreateLayerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, errString("name 不能为空"))
		return
	}
	if req.Title == "" {
		req.Title = req.Name
	}
	layer, err := h.store.CreateLayer(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.invalidateMeta()
	writeJSON(w, http.StatusCreated, layer)
}

func (h *Handlers) getLayer(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, layer)
}

func (h *Handlers) updateLayer(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	var req models.UpdateLayerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	updated, err := h.store.UpdateLayer(r.Context(), layer.ID, req)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	h.invalidateMeta()
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handlers) deleteLayer(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	if err := h.store.DeleteLayer(r.Context(), layer.ID); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	h.invalidateLayer(layer.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) layerBounds(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	b, err := h.store.LayerBounds(r.Context(), layer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bounds": b})
}

// lookupLayer 从路径参数解析图层，未找到时写入 404。
func (h *Handlers) lookupLayer(w http.ResponseWriter, r *http.Request) (*models.Layer, bool) {
	name := chi.URLParam(r, "layer")
	layer, err := h.store.GetLayerByName(r.Context(), name)
	if err != nil {
		writeError(w, statusForError(err), err)
		return nil, false
	}
	return layer, true
}

type errString string

func (e errString) Error() string { return string(e) }

// transformLayer 将图层所有几何转换到新的坐标系。
func (h *Handlers) transformLayer(w http.ResponseWriter, r *http.Request) {
	layer, ok := h.lookupLayer(w, r)
	if !ok {
		return
	}
	var req models.TransformLayerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.SRID <= 0 {
		writeError(w, http.StatusBadRequest, errString("srid 必须为正整数"))
		return
	}
	if req.SRID == layer.SRID {
		writeJSON(w, http.StatusOK, layer)
		return
	}
	updated, err := h.store.TransformLayer(r.Context(), layer, req.SRID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.invalidateLayer(layer.Name)
	writeJSON(w, http.StatusOK, updated)
}
