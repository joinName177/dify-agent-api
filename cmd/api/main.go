package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	openapiv1 "github.com/joinname177/dify-agent-api/generated/openapi/v1"
	"github.com/joinname177/dify-agent-api/internal/modules/user/application/usecases"
	"github.com/joinname177/dify-agent-api/internal/modules/user/infrastructure/credentials"
	userpostgres "github.com/joinname177/dify-agent-api/internal/modules/user/infrastructure/postgres"
	userhttp "github.com/joinname177/dify-agent-api/internal/modules/user/transport/http"
	"github.com/joinname177/dify-agent-api/internal/platform/config"
	"github.com/joinname177/dify-agent-api/internal/platform/database"
	"github.com/joinname177/dify-agent-api/internal/platform/httpserver"
	"github.com/joinname177/dify-agent-api/internal/platform/observability"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := database.Open(rootCtx, cfg.DatabaseURL, cfg.DatabaseMinConns, cfg.DatabaseMaxConns)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.CheckMigrations(rootCtx); err != nil {
		logger.Error("database migrations not ready", "error", err)
		os.Exit(1)
	}
	userRepository := userpostgres.NewRepository(db.Pool())
	userService, err := usecases.NewService(userRepository, credentials.PasswordHasher{}, credentials.TokenIssuer{}, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	if err != nil {
		logger.Error("user service initialization failed", "error", err)
		os.Exit(1)
	}
	adminCreated, err := userService.BootstrapSuperAdmin(rootCtx, cfg.BootstrapAdminEmail, cfg.BootstrapAdminPassword)
	if err != nil {
		logger.Error("super admin bootstrap failed", "error", err)
		os.Exit(1)
	}
	if adminCreated {
		logger.Info("bootstrap super admin created")
	}
	cookiePolicy := userhttp.SecureCookies
	if cfg.Environment == "development" || cfg.Environment == "test" {
		cookiePolicy = userhttp.InsecureCookiesForLocalDevelopment
	}
	userHandler := userhttp.NewHandler(userService, logger, cfg.CORSAllowedOrigins, cookiePolicy)
	shutdownTracing, err := observability.SetupTracing(rootCtx, cfg.OTLPTraceEndpoint, cfg.ReleaseID)
	if err != nil {
		logger.Error("telemetry initialization failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			logger.Error("telemetry shutdown failed", "error", err)
		}
	}()

	server := &http.Server{Addr: cfg.HTTPAddress, Handler: httpserver.New(cfg, db, logger, openapiv1.ContractSHA256, userHandler, userService), ReadHeaderTimeout: cfg.HTTPReadTimeout, ReadTimeout: cfg.HTTPReadTimeout, WriteTimeout: cfg.HTTPWriteTimeout, IdleTimeout: cfg.HTTPIdleTimeout}
	go func() {
		logger.Info("api listening", "address", cfg.HTTPAddress, "release_id", cfg.ReleaseID)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server failed", "error", err)
			stop()
		}
	}()
	<-rootCtx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		return
	}
	logger.Info("api stopped")
}
