package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"mapserver/internal/auth"
	"mapserver/internal/models"
)

func (h *Handlers) listMaps(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errString("需要登录"))
		return
	}
	maps, err := h.store.ListMapStates(r.Context(), user.ID, user.Role == auth.RoleAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"maps": maps})
}

func (h *Handlers) createMap(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFrom(r.Context())
	if !ok || user.ID <= 0 {
		writeError(w, http.StatusUnauthorized, errString("需要登录"))
		return
	}
	var req models.SaveMapRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(req.State) == 0 || !json.Valid(req.State) {
		writeError(w, http.StatusBadRequest, errString("state 必须是合法 JSON"))
		return
	}
	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}
	m, err := h.store.CreateMapState(r.Context(), req.Name, req.State, user.ID, isPublic)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handlers) getMap(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	m, err := h.store.GetMapStateByToken(r.Context(), token)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	if !m.IsPublic {
		user, ok := auth.UserFrom(r.Context())
		if !ok || (user.Role != auth.RoleAdmin && user.ID != m.CreatedBy) {
			writeError(w, http.StatusForbidden, errString("该地图未公开"))
			return
		}
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handlers) updateMap(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	m, err := h.store.GetMapStateByToken(r.Context(), token)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	user, ok := auth.UserFrom(r.Context())
	if !ok || (user.Role != auth.RoleAdmin && user.ID != m.CreatedBy) {
		writeError(w, http.StatusForbidden, errString("无权修改该地图"))
		return
	}
	var req models.SaveMapRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(req.State) > 0 && !json.Valid(req.State) {
		writeError(w, http.StatusBadRequest, errString("state 必须是合法 JSON"))
		return
	}
	updated, err := h.store.UpdateMapState(r.Context(), token, req.Name, req.State, req.IsPublic)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handlers) deleteMap(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	m, err := h.store.GetMapStateByToken(r.Context(), token)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	user, ok := auth.UserFrom(r.Context())
	if !ok || (user.Role != auth.RoleAdmin && user.ID != m.CreatedBy) {
		writeError(w, http.StatusForbidden, errString("无权删除该地图"))
		return
	}
	if err := h.store.DeleteMapState(r.Context(), token); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
