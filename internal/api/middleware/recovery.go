package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/kiranshivaraju/loghunter/internal/api/response"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered",
					"error", err,
					"stack", string(debug.Stack()),
					"method", r.Method,
					"path", r.URL.Path,
				)
				response.Error(w, http.StatusInternalServerError,
					"INTERNAL_ERROR", "An unexpected error occurred", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
