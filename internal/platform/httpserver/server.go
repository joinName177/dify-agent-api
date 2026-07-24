package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	openapiv1 "github.com/joinname177/dify-agent-api/generated/openapi/v1"
	userhttp "github.com/joinname177/dify-agent-api/internal/modules/user/transport/http"
	platformauth "github.com/joinname177/dify-agent-api/internal/platform/auth"
	"github.com/joinname177/dify-agent-api/internal/platform/config"
	platformhttp "github.com/joinname177/dify-agent-api/internal/platform/httputil"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type readinessChecker interface {
	Ping(context.Context) error
	CheckMigrations(context.Context) error
}

type Handler struct {
	db                             readinessChecker
	user                           *userhttp.Handler
	releaseID, buildCommit, digest string
}

func New(cfg config.Config, db readinessChecker, logger *slog.Logger, digest string, userHandler *userhttp.Handler, authenticator platformauth.Authenticator) http.Handler {
	router := chi.NewRouter()
	router.Use(platformhttp.RequestID)
	router.Use(platformhttp.Recover(logger))
	router.Use(platformhttp.AccessLog(logger))
	router.Use(otelhttp.NewMiddleware("http.server"))
	router.Use(platformhttp.SecurityHeaders)
	router.Use(cors.Handler(cors.Options{AllowedOrigins: cfg.CORSAllowedOrigins, AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", platformhttp.RequestIDHeader}, ExposedHeaders: []string{platformhttp.RequestIDHeader}, AllowCredentials: true, MaxAge: 300}))
	router.Use(platformhttp.BodyLimit(cfg.RequestBodyLimit))
	router.Use(platformhttp.RequireJSON)
	router.Use(chimiddleware.Timeout(cfg.RequestTimeout))
	router.Use(platformhttp.RateLimit(cfg.RateLimitRPS, cfg.RateLimitBurst))
	router.Use(platformauth.OptionalMiddleware(authenticator))
	implementation := &Handler{db: db, user: userHandler, releaseID: cfg.ReleaseID, buildCommit: cfg.BuildCommit, digest: digest}
	router.Mount("/api/v1", openapiv1.Handler(implementation))
	return router
}

func (h *Handler) RegisterUser(w http.ResponseWriter, r *http.Request)   { h.user.RegisterUser(w, r) }
func (h *Handler) LoginUser(w http.ResponseWriter, r *http.Request)      { h.user.LoginUser(w, r) }
func (h *Handler) RefreshSession(w http.ResponseWriter, r *http.Request) { h.user.RefreshSession(w, r) }
func (h *Handler) LogoutUser(w http.ResponseWriter, r *http.Request)     { h.user.LogoutUser(w, r) }
func (h *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) { h.user.GetCurrentUser(w, r) }

func (h *Handler) GetLivez(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, openapiv1.HealthResponse{Status: openapiv1.HealthResponseStatusOk})
}

func (h *Handler) GetReadyz(w http.ResponseWriter, r *http.Request) {
	if err := h.db.Ping(r.Context()); err != nil {
		platformhttp.WriteError(w, r, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "A required dependency is unavailable.")
		return
	}
	if err := h.db.CheckMigrations(r.Context()); err != nil {
		platformhttp.WriteError(w, r, http.StatusServiceUnavailable, "MIGRATIONS_NOT_READY", "Database migrations are not ready.")
		return
	}
	writeJSON(w, http.StatusOK, openapiv1.ReadinessResponse{Status: openapiv1.ReadinessResponseStatusOk, Checks: struct {
		Database   openapiv1.ReadinessResponseChecksDatabase   `json:"database"`
		Migrations openapiv1.ReadinessResponseChecksMigrations `json:"migrations"`
	}{Database: openapiv1.ReadinessResponseChecksDatabaseOk, Migrations: openapiv1.ReadinessResponseChecksMigrationsOk}})
}

func (h *Handler) GetVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, openapiv1.VersionResponse{ReleaseId: h.releaseID, BuildCommit: h.buildCommit, ContractId: "dify-agent-api/v1", OpenapiDigest: h.digest})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
