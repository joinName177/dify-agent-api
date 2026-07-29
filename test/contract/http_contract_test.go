package contract_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/joinname177/dify-agent-api/internal/modules/user/application/usecases"
	"github.com/joinname177/dify-agent-api/internal/modules/user/domain"
	userhttp "github.com/joinname177/dify-agent-api/internal/modules/user/transport/http"
	"github.com/joinname177/dify-agent-api/internal/platform/config"
	"github.com/joinname177/dify-agent-api/internal/platform/httpserver"
)

type readiness struct{ err error }

func (r readiness) Ping(context.Context) error            { return r.err }
func (r readiness) CheckMigrations(context.Context) error { return nil }

type userService struct {
	registerErr error
	loginErr    error
}

func (s userService) Register(_ context.Context, input usecases.Registration) (usecases.AuthSession, error) {
	if input.Email == "" || input.Password == "" || input.DisplayName == "" {
		return usecases.AuthSession{}, &usecases.ValidationError{Fields: []usecases.FieldError{{Field: "email", Code: "INVALID_EMAIL", Message: "Enter a valid email address."}}}
	}
	return contractSession(), s.registerErr
}
func (s userService) Login(context.Context, usecases.Credentials) (usecases.AuthSession, error) {
	return contractSession(), s.loginErr
}
func (userService) Refresh(context.Context, string) (usecases.AuthSession, error) {
	return contractSession(), nil
}
func (userService) Logout(context.Context, string) error { return nil }

func TestImplementedHTTPResponsesMatchOpenAPI(t *testing.T) {
	contractRouter := loadContract(t)
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		origin      string
		service     userService
		databaseErr error
		wantStatus  int
		wantCode    string
		secret      string
	}{
		{name: "register success", method: http.MethodPost, path: "/api/v1/auth/register", body: `{"email":"person@example.test","password":"long-enough-password","displayName":"Person"}`, contentType: "application/json", wantStatus: 201},
		{name: "malformed JSON", method: http.MethodPost, path: "/api/v1/auth/register", body: `{`, contentType: "application/json", wantStatus: 400, wantCode: "MALFORMED_JSON"},
		{name: "missing field", method: http.MethodPost, path: "/api/v1/auth/register", body: `{}`, contentType: "application/json", wantStatus: 422, wantCode: "VALIDATION_ERROR"},
		{name: "unsupported content type", method: http.MethodPost, path: "/api/v1/auth/register", body: `{}`, contentType: "text/plain", wantStatus: 415, wantCode: "UNSUPPORTED_MEDIA_TYPE"},
		{name: "forbidden origin", method: http.MethodPost, path: "/api/v1/auth/register", body: `{}`, contentType: "application/json", origin: "https://evil.example.test", wantStatus: 403, wantCode: "ORIGIN_NOT_ALLOWED"},
		{name: "email conflict", method: http.MethodPost, path: "/api/v1/auth/register", body: `{"email":"person@example.test","password":"long-enough-password","displayName":"Person"}`, contentType: "application/json", service: userService{registerErr: usecases.ErrEmailAlreadyRegistered}, wantStatus: 409, wantCode: "EMAIL_ALREADY_REGISTERED"},
		{name: "invalid credentials", method: http.MethodPost, path: "/api/v1/auth/login", body: `{"email":"person@example.test","password":"wrong"}`, contentType: "application/json", service: userService{loginErr: usecases.ErrInvalidCredentials}, wantStatus: 401, wantCode: "INVALID_CREDENTIALS"},
		{name: "missing identity", method: http.MethodGet, path: "/api/v1/auth/me", wantStatus: 401, wantCode: "UNAUTHENTICATED"},
		{name: "database unavailable", method: http.MethodGet, path: "/api/v1/readyz", databaseErr: errors.New("database password secret-value"), wantStatus: 503, wantCode: "DATABASE_UNAVAILABLE", secret: "secret-value"},
		{name: "safe internal error", method: http.MethodPost, path: "/api/v1/auth/login", body: `{"email":"person@example.test","password":"wrong"}`, contentType: "application/json", service: userService{loginErr: errors.New("internal secret-value")}, wantStatus: 500, wantCode: "INTERNAL_ERROR", secret: "secret-value"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newHandler(test.service, test.databaseErr, 1024, 1000, 1000)
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assertContractResponse(t, contractRouter, request, response, test.wantStatus, test.wantCode)
			if test.secret != "" && strings.Contains(response.Body.String(), test.secret) {
				t.Fatalf("response leaked internal data: %s", response.Body.String())
			}
		})
	}
}

func TestBodyLimitAndRateLimitResponsesMatchOpenAPI(t *testing.T) {
	contractRouter := loadContract(t)
	bodyHandler := newHandler(userService{}, nil, 32, 1000, 1000)
	bodyRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"person@example.test","password":"long-enough-password","displayName":"Person"}`))
	bodyRequest.Header.Set("Content-Type", "application/json")
	bodyResponse := httptest.NewRecorder()
	bodyHandler.ServeHTTP(bodyResponse, bodyRequest)
	assertContractResponse(t, contractRouter, bodyRequest, bodyResponse, http.StatusBadRequest, "MALFORMED_JSON")

	rateHandler := newHandler(userService{}, nil, 1024, 0.000001, 1)
	for attempt := 1; attempt <= 2; attempt++ {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/livez", nil)
		response := httptest.NewRecorder()
		rateHandler.ServeHTTP(response, request)
		if attempt == 2 {
			assertContractResponse(t, contractRouter, request, response, http.StatusTooManyRequests, "RATE_LIMITED")
		}
	}
}

func newHandler(service userService, databaseErr error, bodyLimit int64, rps float64, burst int) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	userHandler := userhttp.NewHandler(service, logger, []string{"https://app.example.test"}, userhttp.SecureCookies)
	cfg := config.Config{
		CORSAllowedOrigins: []string{"https://app.example.test"},
		ReleaseID:          "test", BuildCommit: "abc", RequestTimeout: time.Second,
		RequestBodyLimit: bodyLimit, RateLimitRPS: rps, RateLimitBurst: burst,
	}
	return httpserver.New(cfg, readiness{err: databaseErr}, logger, strings.Repeat("a", 64), userHandler, nil)
}

func contractSession() usecases.AuthSession {
	now := time.Now().UTC()
	return usecases.AuthSession{
		AccessToken: strings.Repeat("a", 40), RefreshToken: strings.Repeat("r", 40),
		AccessExpiresAt: now.Add(15 * time.Minute), RefreshExpiresAt: now.Add(24 * time.Hour),
		User: domain.User{PublicID: "d9428888-122b-11e1-b85c-61cd3cbb3210", Email: "person@example.test", DisplayName: "Person", Role: domain.RoleUser, Status: domain.StatusActive, CreatedAt: now},
	}
}

func loadContract(t *testing.T) routers.Router {
	t.Helper()
	document, err := openapi3.NewLoader().LoadFromFile("../../api/openapi/v1/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(document)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func assertContractResponse(t *testing.T, router routers.Router, request *http.Request, recorder *httptest.ResponseRecorder, wantStatus int, wantErrorCode string) {
	t.Helper()
	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var envelope struct {
		Code      int    `json:"code"`
		ErrorCode string `json:"errorCode"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	if envelope.Code != recorder.Code || envelope.RequestID == "" || envelope.ErrorCode != wantErrorCode {
		t.Fatalf("invalid envelope: %+v; HTTP status=%d", envelope, recorder.Code)
	}

	route, pathParams, err := router.FindRoute(request)
	if err != nil {
		t.Fatalf("find OpenAPI route: %v", err)
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: request, PathParams: pathParams, Route: route},
		Status:                 recorder.Code,
		Header:                 recorder.Header(),
	}
	input.SetBodyBytes(bytes.Clone(recorder.Body.Bytes()))
	if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
		t.Fatalf("response violates OpenAPI: %v; body=%s", err, recorder.Body.String())
	}
}
