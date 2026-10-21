package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"mapserver/internal/auth"
	"mapserver/internal/models"
	"mapserver/internal/store"
)

func (h *Handlers) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.store.ListAPIKeys(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (h *Handlers) createAPIKey(w http.ResponseWriter, r *http.Request) {
	var req models.CreateAPIKeyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, errString("name 不能为空"))
		return
	}
	role := strings.ToLower(req.Role)
	if role != auth.RoleEditor && role != auth.RoleAdmin {
		role = auth.RoleViewer
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	plain := "ms_" + hex.EncodeToString(buf)
	prefix := plain[:11]
	user, _ := auth.UserFrom(r.Context())
	var createdBy int64
	if user != nil {
		createdBy = user.ID
	}
	key, err := h.store.CreateAPIKey(r.Context(), req.Name, prefix, store.HashAPIKey(plain), role, req.LayerName, createdBy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"key": key, "plaintext": plain})
}

func (h *Handlers) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errString("id 非法"))
		return
	}
	if err := h.store.RevokeAPIKey(r.Context(), id); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handlers) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errString("id 非法"))
		return
	}
	if err := h.store.DeleteAPIKey(r.Context(), id); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	logs, err := h.store.ListAuditLogs(r.Context(), limit, r.URL.Query().Get("layer"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}
