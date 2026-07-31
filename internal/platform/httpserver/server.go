package httpserver

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	openapiv1 "github.com/joinname177/dify-agent-api/generated/openapi/v1"
	platformauth "github.com/joinname177/dify-agent-api/internal/platform/auth"
	"github.com/joinname177/dify-agent-api/internal/platform/config"
	platformhttp "github.com/joinname177/dify-agent-api/internal/platform/httputil"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type readinessChecker interface {
	Ping(context.Context) error
	CheckMigrations(context.Context) error
}

type userHandler interface {
	RegisterUser(http.ResponseWriter, *http.Request)
	LoginUser(http.ResponseWriter, *http.Request)
	RefreshSession(http.ResponseWriter, *http.Request)
	LogoutUser(http.ResponseWriter, *http.Request)
	GetCurrentUser(http.ResponseWriter, *http.Request)
}

type agentHandler interface {
	ChatWithAgent(http.ResponseWriter, *http.Request)
}

type Handler struct {
	db                             readinessChecker
	user                           userHandler
	agent                          agentHandler
	releaseID, buildCommit, digest string
}

func New(cfg config.Config, db readinessChecker, logger *slog.Logger, digest string, user userHandler, agent agentHandler, authenticator platformauth.Authenticator) http.Handler {
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
	implementation := &Handler{db: db, user: user, agent: agent, releaseID: cfg.ReleaseID, buildCommit: cfg.BuildCommit, digest: digest}
	apiRouter := chi.NewRouter()
	apiRouter.NotFound(func(w http.ResponseWriter, r *http.Request) {
		platformhttp.WriteError(w, r, http.StatusNotFound, "ROUTE_NOT_FOUND", "The requested route does not exist.")
	})
	apiRouter.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		platformhttp.WriteError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "The request method is not allowed.")
	})
	openapiv1.HandlerFromMux(implementation, apiRouter)
	router.Mount("/api/v1", apiRouter)
	return router
}

func (h *Handler) ChatWithAgent(w http.ResponseWriter, r *http.Request)  { h.agent.ChatWithAgent(w, r) }
func (h *Handler) RegisterUser(w http.ResponseWriter, r *http.Request)   { h.user.RegisterUser(w, r) }
func (h *Handler) LoginUser(w http.ResponseWriter, r *http.Request)      { h.user.LoginUser(w, r) }
func (h *Handler) RefreshSession(w http.ResponseWriter, r *http.Request) { h.user.RefreshSession(w, r) }
func (h *Handler) LogoutUser(w http.ResponseWriter, r *http.Request)     { h.user.LogoutUser(w, r) }
func (h *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) { h.user.GetCurrentUser(w, r) }

func (h *Handler) GetLivez(w http.ResponseWriter, r *http.Request) {
	response := openapiv1.HealthResponse{
		Code:      openapiv1.HealthResponseCodeN200,
		Message:   "Service is healthy.",
		RequestId: platformhttp.RequestIDFromContext(r.Context()),
	}
	response.Data.Status = openapiv1.HealthResponseDataStatusOk
	platformhttp.WriteJSON(w, http.StatusOK, response)
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
	response := openapiv1.ReadinessResponse{
		Code:      openapiv1.ReadinessResponseCodeN200,
		Message:   "Service is ready.",
		RequestId: platformhttp.RequestIDFromContext(r.Context()),
	}
	response.Data.Status = openapiv1.ReadinessResponseDataStatusOk
	response.Data.Checks.Database = openapiv1.ReadinessResponseDataChecksDatabaseOk
	response.Data.Checks.Migrations = openapiv1.ReadinessResponseDataChecksMigrationsOk
	platformhttp.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	response := openapiv1.VersionResponse{
		Code:      openapiv1.VersionResponseCodeN200,
		Message:   "Version retrieved.",
		RequestId: platformhttp.RequestIDFromContext(r.Context()),
	}
	response.Data.ReleaseId = h.releaseID
	response.Data.BuildCommit = h.buildCommit
	response.Data.ContractId = "dify-agent-api/v1"
	response.Data.OpenapiDigest = h.digest
	platformhttp.WriteJSON(w, http.StatusOK, response)
}
