package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	mw "github.com/kiranshivaraju/loghunter/internal/api/middleware"
	"github.com/kiranshivaraju/loghunter/internal/api/response"
)

// Dependencies holds all handler and middleware dependencies for the router.
type Dependencies struct {
	Auth      *mw.Auth
	RateLimit *mw.RateLimit

	HealthHandler   http.HandlerFunc
	AnalyzeHandler  http.HandlerFunc
	PollJobHandler  http.HandlerFunc
	ListClusters    http.HandlerFunc
	GetCluster      http.HandlerFunc
	SummarizeHandler http.HandlerFunc
	SearchHandler   http.HandlerFunc
	CreateKeyHandler http.HandlerFunc
	ListKeysHandler  http.HandlerFunc
	RevokeKeyHandler http.HandlerFunc
}

// NewRouter builds the Chi router with middleware stack and all routes.
func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(mw.Logger)
	r.Use(mw.Recovery)

	// Public health check
	r.Get("/api/v1/health", orNotImplemented(deps.HealthHandler))

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(deps.Auth.Authenticate)
		r.Use(deps.RateLimit.Limit)

		r.Post("/api/v1/analyze", orNotImplemented(deps.AnalyzeHandler))
		r.Get("/api/v1/analyze/{jobID}", orNotImplemented(deps.PollJobHandler))

		r.Get("/api/v1/clusters", orNotImplemented(deps.ListClusters))
		r.Get("/api/v1/clusters/{clusterID}", orNotImplemented(deps.GetCluster))

		r.Post("/api/v1/summarize", orNotImplemented(deps.SummarizeHandler))
		r.Post("/api/v1/search", orNotImplemented(deps.SearchHandler))

		// Admin routes
		r.Group(func(r chi.Router) {
			r.Use(deps.Auth.RequireScope("admin"))

			r.Post("/api/v1/admin/keys", orNotImplemented(deps.CreateKeyHandler))
			r.Get("/api/v1/admin/keys", orNotImplemented(deps.ListKeysHandler))
			r.Delete("/api/v1/admin/keys/{keyID}", orNotImplemented(deps.RevokeKeyHandler))
		})
	})

	return r
}

// orNotImplemented returns the handler if non-nil, or a 501 placeholder.
func orNotImplemented(h http.HandlerFunc) http.HandlerFunc {
	if h != nil {
		return h
	}
	return func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Endpoint not yet implemented", nil)
	}
}
