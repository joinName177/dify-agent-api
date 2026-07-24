package config

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const prefix = "DIFY_AGENT_"

type Config struct {
	Environment            string
	HTTPAddress            string
	DatabaseURL            string
	DatabaseMinConns       int32
	DatabaseMaxConns       int32
	BootstrapAdminEmail    string
	BootstrapAdminPassword string
	OTLPTraceEndpoint      string
	CORSAllowedOrigins     []string
	ReleaseID              string
	BuildCommit            string
	HTTPReadTimeout        time.Duration
	HTTPWriteTimeout       time.Duration
	HTTPIdleTimeout        time.Duration
	HTTPShutdownTimeout    time.Duration
	RequestTimeout         time.Duration
	RequestBodyLimit       int64
	RateLimitRPS           float64
	RateLimitBurst         int
	AccessTokenTTL         time.Duration
	RefreshTokenTTL        time.Duration
}

var allowed = map[string]struct{}{
	"DIFY_AGENT_ENVIRONMENT": {}, "DIFY_AGENT_HTTP_ADDRESS": {},
	"DIFY_AGENT_DATABASE_URL": {}, "DIFY_AGENT_DATABASE_MIN_CONNS": {},
	"DIFY_AGENT_DATABASE_MAX_CONNS": {}, "DIFY_AGENT_CORS_ALLOWED_ORIGINS": {},
	"DIFY_AGENT_BOOTSTRAP_ADMIN_EMAIL": {}, "DIFY_AGENT_BOOTSTRAP_ADMIN_PASSWORD": {},
	"DIFY_AGENT_OTEL_EXPORTER_OTLP_ENDPOINT": {},
	"DIFY_AGENT_RELEASE_ID":                  {}, "DIFY_AGENT_BUILD_COMMIT": {},
	"DIFY_AGENT_HTTP_READ_TIMEOUT": {}, "DIFY_AGENT_HTTP_WRITE_TIMEOUT": {},
	"DIFY_AGENT_HTTP_IDLE_TIMEOUT": {}, "DIFY_AGENT_HTTP_SHUTDOWN_TIMEOUT": {},
	"DIFY_AGENT_REQUEST_TIMEOUT": {}, "DIFY_AGENT_REQUEST_BODY_LIMIT": {},
	"DIFY_AGENT_RATE_LIMIT_RPS": {}, "DIFY_AGENT_RATE_LIMIT_BURST": {},
	"DIFY_AGENT_ACCESS_TOKEN_TTL": {}, "DIFY_AGENT_REFRESH_TOKEN_TTL": {},
}

func Load() (Config, error) { return load(os.Environ()) }

func load(environ []string) (Config, error) {
	values := make(map[string]string, len(environ))
	for _, item := range environ {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(key, prefix) {
			if _, ok := allowed[key]; !ok {
				return Config{}, fmt.Errorf("unknown configuration variable %s", key)
			}
		}
		values[key] = value
	}

	required := func(key string) (string, error) {
		value := strings.TrimSpace(values[key])
		if value == "" {
			return "", fmt.Errorf("%s is required", key)
		}
		return value, nil
	}
	withDefault := func(key, fallback string) string {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
		return fallback
	}

	var cfg Config
	var err error
	if cfg.Environment, err = required("DIFY_AGENT_ENVIRONMENT"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseURL, err = required("DIFY_AGENT_DATABASE_URL"); err != nil {
		return Config{}, err
	}
	if cfg.BootstrapAdminEmail, err = required("DIFY_AGENT_BOOTSTRAP_ADMIN_EMAIL"); err != nil {
		return Config{}, err
	}
	if cfg.BootstrapAdminPassword, err = required("DIFY_AGENT_BOOTSTRAP_ADMIN_PASSWORD"); err != nil {
		return Config{}, err
	}
	cfg.BootstrapAdminEmail = strings.ToLower(cfg.BootstrapAdminEmail)
	address, emailErr := mail.ParseAddress(cfg.BootstrapAdminEmail)
	if emailErr != nil || address.Address != cfg.BootstrapAdminEmail || len(cfg.BootstrapAdminEmail) > 254 {
		return Config{}, errors.New("DIFY_AGENT_BOOTSTRAP_ADMIN_EMAIL must be a valid email address")
	}
	passwordLength := utf8.RuneCountInString(cfg.BootstrapAdminPassword)
	if passwordLength < 12 || passwordLength > 128 || len(cfg.BootstrapAdminPassword) > 512 {
		return Config{}, errors.New("DIFY_AGENT_BOOTSTRAP_ADMIN_PASSWORD must contain between 12 and 128 characters")
	}
	if cfg.ReleaseID, err = required("DIFY_AGENT_RELEASE_ID"); err != nil {
		return Config{}, err
	}
	if cfg.BuildCommit, err = required("DIFY_AGENT_BUILD_COMMIT"); err != nil {
		return Config{}, err
	}
	cfg.OTLPTraceEndpoint = strings.TrimSpace(values["DIFY_AGENT_OTEL_EXPORTER_OTLP_ENDPOINT"])
	if cfg.Environment == "production" && cfg.OTLPTraceEndpoint == "" {
		return Config{}, errors.New("DIFY_AGENT_OTEL_EXPORTER_OTLP_ENDPOINT is required in production")
	}
	if cfg.OTLPTraceEndpoint != "" {
		if err := validateHTTPSURL(cfg.OTLPTraceEndpoint, cfg.Environment == "development"); err != nil {
			return Config{}, fmt.Errorf("DIFY_AGENT_OTEL_EXPORTER_OTLP_ENDPOINT: %w", err)
		}
	}

	corsValue, err := required("DIFY_AGENT_CORS_ALLOWED_ORIGINS")
	if err != nil {
		return Config{}, err
	}
	for _, origin := range strings.Split(corsValue, ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			return Config{}, errors.New("DIFY_AGENT_CORS_ALLOWED_ORIGINS contains an empty origin")
		}
		if err := validateHTTPSURL(origin, cfg.Environment == "development"); err != nil {
			return Config{}, fmt.Errorf("CORS origin %q: %w", origin, err)
		}
		cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, origin)
	}

	cfg.HTTPAddress = withDefault("DIFY_AGENT_HTTP_ADDRESS", ":8080")
	if cfg.DatabaseMinConns, err = parseInt32(withDefault("DIFY_AGENT_DATABASE_MIN_CONNS", "2")); err != nil {
		return Config{}, fmt.Errorf("DIFY_AGENT_DATABASE_MIN_CONNS: %w", err)
	}
	if cfg.DatabaseMaxConns, err = parseInt32(withDefault("DIFY_AGENT_DATABASE_MAX_CONNS", "10")); err != nil {
		return Config{}, fmt.Errorf("DIFY_AGENT_DATABASE_MAX_CONNS: %w", err)
	}
	if cfg.DatabaseMinConns < 0 || cfg.DatabaseMaxConns < 1 || cfg.DatabaseMinConns > cfg.DatabaseMaxConns {
		return Config{}, errors.New("database pool sizes are invalid")
	}
	if cfg.HTTPReadTimeout, err = time.ParseDuration(withDefault("DIFY_AGENT_HTTP_READ_TIMEOUT", "5s")); err != nil {
		return Config{}, err
	}
	if cfg.HTTPWriteTimeout, err = time.ParseDuration(withDefault("DIFY_AGENT_HTTP_WRITE_TIMEOUT", "15s")); err != nil {
		return Config{}, err
	}
	if cfg.HTTPIdleTimeout, err = time.ParseDuration(withDefault("DIFY_AGENT_HTTP_IDLE_TIMEOUT", "60s")); err != nil {
		return Config{}, err
	}
	if cfg.HTTPShutdownTimeout, err = time.ParseDuration(withDefault("DIFY_AGENT_HTTP_SHUTDOWN_TIMEOUT", "10s")); err != nil {
		return Config{}, err
	}
	if cfg.RequestTimeout, err = time.ParseDuration(withDefault("DIFY_AGENT_REQUEST_TIMEOUT", "10s")); err != nil {
		return Config{}, err
	}
	if cfg.RequestBodyLimit, err = strconv.ParseInt(withDefault("DIFY_AGENT_REQUEST_BODY_LIMIT", "1048576"), 10, 64); err != nil || cfg.RequestBodyLimit < 1 {
		return Config{}, errors.New("DIFY_AGENT_REQUEST_BODY_LIMIT must be a positive integer")
	}
	if cfg.RateLimitRPS, err = strconv.ParseFloat(withDefault("DIFY_AGENT_RATE_LIMIT_RPS", "100"), 64); err != nil || cfg.RateLimitRPS <= 0 {
		return Config{}, errors.New("DIFY_AGENT_RATE_LIMIT_RPS must be positive")
	}
	if cfg.RateLimitBurst, err = strconv.Atoi(withDefault("DIFY_AGENT_RATE_LIMIT_BURST", "200")); err != nil || cfg.RateLimitBurst < 1 {
		return Config{}, errors.New("DIFY_AGENT_RATE_LIMIT_BURST must be positive")
	}
	if cfg.AccessTokenTTL, err = time.ParseDuration(withDefault("DIFY_AGENT_ACCESS_TOKEN_TTL", "15m")); err != nil || cfg.AccessTokenTTL <= 0 {
		return Config{}, errors.New("DIFY_AGENT_ACCESS_TOKEN_TTL must be a positive duration")
	}
	if cfg.RefreshTokenTTL, err = time.ParseDuration(withDefault("DIFY_AGENT_REFRESH_TOKEN_TTL", "168h")); err != nil || cfg.RefreshTokenTTL <= cfg.AccessTokenTTL {
		return Config{}, errors.New("DIFY_AGENT_REFRESH_TOKEN_TTL must be greater than the access token TTL")
	}
	return cfg, nil
}

func validateHTTPSURL(raw string, allowLocalHTTP bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("must be an absolute URL without credentials, query, or fragment")
	}
	if u.Scheme == "https" {
		return nil
	}
	if allowLocalHTTP && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1") {
		return nil
	}
	return errors.New("must use HTTPS (HTTP is allowed only for localhost in development)")
}

func parseInt32(value string) (int32, error) {
	n, err := strconv.ParseInt(value, 10, 32)
	return int32(n), err
}
