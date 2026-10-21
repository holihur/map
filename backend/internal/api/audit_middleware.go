package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"mapserver/internal/auth"
	"mapserver/internal/models"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// auditMiddleware 记录所有写操作的审计日志（异步，不阻塞响应）。
func (h *Handlers) auditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/auth/login" {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		user, _ := auth.UserFrom(r.Context())
		entry := models.AuditLog{
			Method: r.Method,
			Path:   r.URL.Path,
			Status: rec.status,
			Layer:  layerNameFromPath(r.URL.Path),
			IP:     clientIP(r),
		}
		if user != nil {
			entry.UserID = user.ID
			entry.Username = user.Username
			entry.Role = user.Role
		}
		if entry.Layer == "" {
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if len(parts) >= 3 && parts[1] == "maps" {
				entry.Layer = ""
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = h.store.InsertAuditLog(ctx, entry)
	})
}
