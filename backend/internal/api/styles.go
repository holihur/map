package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"mapserver/internal/models"
	"mapserver/internal/sld"
)

func (h *Handlers) listStyles(w http.ResponseWriter, r *http.Request) {
	writeCachedJSON(w, r, h.meta, "styles", func() ([]byte, error) {
		styles, err := h.store.ListStyles(r.Context())
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"styles": styles})
	})
}

func (h *Handlers) createStyle(w http.ResponseWriter, r *http.Request) {
	var req models.CreateStyleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, errString("name 不能为空"))
		return
	}
	if req.SLD != "" {
		if _, err := sld.Parse([]byte(req.SLD)); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	st, err := h.store.CreateStyle(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.invalidateMeta()
	writeJSON(w, http.StatusCreated, st)
}

// getStyle 按名称获取样式（含内嵌 SLD）。
func (h *Handlers) getStyle(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	st, err := h.store.GetStyleByName(r.Context(), name)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// uploadSLD 上传 SLD 文档（请求体为 SLD XML，`?name=` 指定样式名）。
func (h *Handlers) uploadSLD(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, errString("缺少 name 参数"))
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := sld.Parse(body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	isDefault := r.URL.Query().Get("default") == "true"
	req := models.CreateStyleRequest{
		Name:      name,
		IsDefault: isDefault,
		Style:     json.RawMessage(`{}`),
		SLD:       string(body),
	}

	if existing, err := h.store.GetStyleByName(r.Context(), name); err == nil {
		st, err := h.store.UpdateStyle(r.Context(), existing.ID, req)
		if err != nil {
			writeError(w, statusForError(err), err)
			return
		}
		h.invalidateMeta()
		writeJSON(w, http.StatusOK, st)
		return
	}
	st, err := h.store.CreateStyle(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.invalidateMeta()
	writeJSON(w, http.StatusCreated, st)
}

func (h *Handlers) updateStyle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errString("id 非法"))
		return
	}
	var req models.CreateStyleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.SLD != "" {
		if _, err := sld.Parse([]byte(req.SLD)); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	st, err := h.store.UpdateStyle(r.Context(), id, req)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	h.invalidateMeta()
	writeJSON(w, http.StatusOK, st)
}

func (h *Handlers) deleteStyle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errString("id 非法"))
		return
	}
	if err := h.store.DeleteStyle(r.Context(), id); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	h.invalidateMeta()
	w.WriteHeader(http.StatusNoContent)
}
