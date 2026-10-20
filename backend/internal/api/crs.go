package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// listCRS 搜索支持的坐标参考系（来自 PostGIS spatial_ref_sys）。
func (h *Handlers) listCRS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	list, err := h.store.ListCRS(r.Context(), q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"crs": list})
}

// getCRS 返回指定 SRID 的信息。
func (h *Handlers) getCRS(w http.ResponseWriter, r *http.Request) {
	srid, err := strconv.Atoi(chi.URLParam(r, "srid"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errString("srid 非法"))
		return
	}
	c, err := h.store.GetCRS(r.Context(), srid)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}
