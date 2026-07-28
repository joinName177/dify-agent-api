package httpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joinname177/dify-agent-api/internal/platform/config"
)

type fakeReadiness struct{ pingErr, migrationErr error }

func (f fakeReadiness) Ping(context.Context) error            { return f.pingErr }
func (f fakeReadiness) CheckMigrations(context.Context) error { return f.migrationErr }

func testConfig() config.Config {
	return config.Config{
		CORSAllowedOrigins: []string{"https://app.example.com"},
		ReleaseID:          "test", BuildCommit: "abc123", RequestTimeout: time.Second,
		RequestBodyLimit: 1024, RateLimitRPS: 1000, RateLimitBurst: 1000,
	}
}

func TestOperationalEndpoints(t *testing.T) {
	handler := New(testConfig(), fakeReadiness{}, slog.New(slog.NewTextHandler(io.Discard, nil)), strings.Repeat("a", 64), nil, nil)
	for _, test := range []struct{ path, code, body string }{
		{"/api/v1/livez", `"code":"HEALTHY"`, `"status":"ok"`},
		{"/api/v1/readyz", `"code":"READY"`, `"database":"ok"`},
		{"/api/v1/version", `"code":"VERSION_RETRIEVED"`, `"releaseId":"test"`},
	} {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Errorf("%s: status %d, body %s", test.path, res.Code, res.Body.String())
		}
		if !strings.Contains(res.Body.String(), test.code) || !strings.Contains(res.Body.String(), test.body) {
			t.Errorf("%s: standard response missing %s or %s in %s", test.path, test.code, test.body, res.Body.String())
		}
		if res.Header().Get("X-Request-Id") == "" {
			t.Errorf("%s: request id missing", test.path)
		}
		if res.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: security headers missing", test.path)
		}
	}
}

func TestUnknownRoutesAndMethodsUseStandardErrors(t *testing.T) {
	handler := New(testConfig(), fakeReadiness{}, slog.New(slog.NewTextHandler(io.Discard, nil)), strings.Repeat("a", 64), nil, nil)
	for _, test := range []struct {
		method, path, code string
		status             int
	}{
		{http.MethodGet, "/api/v1/missing", `"code":"ROUTE_NOT_FOUND"`, http.StatusNotFound},
		{http.MethodDelete, "/api/v1/livez", `"code":"METHOD_NOT_ALLOWED"`, http.StatusMethodNotAllowed},
	} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(test.method, test.path, nil))
		if res.Code != test.status || !strings.Contains(res.Body.String(), test.code) {
			t.Errorf("%s %s: unexpected response %d %s", test.method, test.path, res.Code, res.Body.String())
		}
	}
}

func TestReadinessDoesNotLeakDependencyError(t *testing.T) {
	handler := New(testConfig(), fakeReadiness{pingErr: errors.New("password for database secret-value")}, slog.New(slog.NewTextHandler(io.Discard, nil)), strings.Repeat("a", 64), nil, nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/readyz", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", res.Code)
	}
	if strings.Contains(res.Body.String(), "secret-value") {
		t.Fatalf("dependency error leaked: %s", res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"code":"DATABASE_UNAVAILABLE"`) {
		t.Fatalf("stable error code missing: %s", res.Body.String())
	}
}

func TestCORSAllowlist(t *testing.T) {
	handler := New(testConfig(), fakeReadiness{}, slog.New(slog.NewTextHandler(io.Discard, nil)), strings.Repeat("a", 64), nil, nil)
	for _, test := range []struct{ origin, expected string }{{"https://app.example.com", "https://app.example.com"}, {"https://evil.example.com", ""}} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/livez", nil)
		req.Header.Set("Origin", test.origin)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if got := res.Header().Get("Access-Control-Allow-Origin"); got != test.expected {
			t.Errorf("origin %s: got %q, want %q", test.origin, got, test.expected)
		}
	}
}
