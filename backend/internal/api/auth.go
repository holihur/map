package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"mapserver/internal/auth"
	"mapserver/internal/models"
)

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, errString("用户名与密码不能为空"))
		return
	}

	user, err := h.store.GetUserByUsername(r.Context(), req.Username)
	if err != nil || !auth.CheckPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, errString("用户名或密码错误"))
		return
	}

	token, err := signToken(h.cfg, user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":     token,
		"user":      user,
		"expiresIn": int(h.cfg.TokenTTL.Seconds()),
	})
}

func (h *Handlers) me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errString("未登录"))
		return
	}
	// 回源数据库以获取完整信息（并确认用户仍然存在）。
	if user, err := h.store.GetUserByUsername(r.Context(), claims.Username); err == nil {
		writeJSON(w, http.StatusOK, user)
		return
	}
	writeError(w, http.StatusUnauthorized, errString("用户不存在"))
}

// ── 用户管理（仅管理员）─────────────────────────────────────────

func (h *Handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (h *Handlers) createUser(w http.ResponseWriter, r *http.Request) {
	var req models.CreateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, errString("用户名不能为空，密码至少 8 位"))
		return
	}
	if !validRole(req.Role) {
		writeError(w, http.StatusBadRequest, errString("角色必须是 admin / editor / viewer"))
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	user, err := h.store.CreateUser(r.Context(), req.Username, hash, req.Role)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (h *Handlers) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req models.UpdateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var hash *string
	if req.Password != nil {
		if len(*req.Password) < minPasswordLen {
			writeError(w, http.StatusBadRequest, errString("密码至少 8 位"))
			return
		}
		hs, err := auth.HashPassword(*req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		hash = &hs
	}
	if req.Role != nil && !validRole(*req.Role) {
		writeError(w, http.StatusBadRequest, errString("角色非法"))
		return
	}
	user, err := h.store.UpdateUser(r.Context(), id, hash, req.Role)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	// 修改密码会递增 token_version，使旧 Token 失效
	h.versions.invalidate(id)
	writeJSON(w, http.StatusOK, user)
}

func (h *Handlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if me, ok := auth.UserFrom(r.Context()); ok && me.ID == id {
		writeError(w, http.StatusBadRequest, errString("不能删除当前登录用户"))
		return
	}
	if err := h.store.DeleteUser(r.Context(), id); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// minPasswordLen 密码最小长度。
const minPasswordLen = 8

func validRole(role string) bool {
	switch role {
	case auth.RoleAdmin, auth.RoleEditor, auth.RoleViewer:
		return true
	default:
		return false
	}
}

func parseIDParam(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil {
		return 0, errString("id 非法")
	}
	return id, nil
}
