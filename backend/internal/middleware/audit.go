package middleware

import (
	"log/slog"
	"net/http"
)

// AuditAction emits a structured audit line for security-sensitive API use.
func AuditAction(log *slog.Logger, r *http.Request, action string, attrs ...slog.Attr) {
	if log == nil {
		return
	}
	claims := GetAuthClaims(r)
	username := ""
	role := ""
	if claims != nil {
		username = claims.Username
		role = claims.Role
	}
	args := []any{
		slog.String("audit_action", action),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("request_id", RequestIDFromContext(r.Context())),
		slog.String("user", username),
		slog.String("role", role),
	}
	for _, a := range attrs {
		args = append(args, a)
	}
	log.Info("audit", args...)
}
