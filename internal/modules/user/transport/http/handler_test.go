package userhttp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joinname177/dify-agent-api/internal/modules/user/application/usecases"
	"github.com/joinname177/dify-agent-api/internal/modules/user/domain"
)

type fakeService struct {
	registerCalls int
	loginErr      error
}

func (f *fakeService) Register(context.Context, usecases.Registration) (usecases.AuthSession, error) {
	f.registerCalls++
	return testSession(), nil
}
func (f *fakeService) Login(context.Context, usecases.Credentials) (usecases.AuthSession, error) {
	return usecases.AuthSession{}, f.loginErr
}
func (*fakeService) Refresh(context.Context, string) (usecases.AuthSession, error) {
	return testSession(), nil
}
func (*fakeService) Logout(context.Context, string) error { return nil }

func TestRegisterSetsServerManagedRefreshCookie(t *testing.T) {
	service := &fakeService{}
	handler := NewHandler(service, slog.New(slog.NewTextHandler(io.Discard, nil)), []string{"https://app.example.test"}, true)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"person@example.test","password":"long-enough-password","displayName":"Person"}`))
	request.Header.Set("Origin", "https://app.example.test")
	response := httptest.NewRecorder()

	handler.RegisterUser(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("refresh cookie is unsafe: %+v", cookies)
	}
}

func TestRegisterRejectsUnknownOriginBeforeCallingService(t *testing.T) {
	service := &fakeService{}
	handler := NewHandler(service, slog.Default(), []string{"https://app.example.test"}, true)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{}`))
	request.Header.Set("Origin", "https://evil.example.test")
	response := httptest.NewRecorder()

	handler.RegisterUser(response, request)

	if response.Code != http.StatusForbidden || service.registerCalls != 0 {
		t.Fatalf("status = %d, register calls = %d", response.Code, service.registerCalls)
	}
}

func TestLoginUsesStableInvalidCredentialsCode(t *testing.T) {
	handler := NewHandler(&fakeService{loginErr: usecases.ErrInvalidCredentials}, slog.Default(), nil, false)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"person@example.test","password":"wrong"}`))
	response := httptest.NewRecorder()

	handler.LoginUser(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"code":"INVALID_CREDENTIALS"`) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}

func TestRefreshClearsInvalidCookie(t *testing.T) {
	service := &fakeService{}
	handler := NewHandler(service, slog.Default(), nil, false)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	response := httptest.NewRecorder()

	handler.RefreshSession(response, request)

	if response.Code != http.StatusUnauthorized || len(response.Result().Cookies()) != 1 || response.Result().Cookies()[0].MaxAge != -1 {
		t.Fatalf("invalid refresh response: %d %+v", response.Code, response.Result().Cookies())
	}
}

func testSession() usecases.AuthSession {
	now := time.Now().UTC()
	return usecases.AuthSession{
		AccessToken:      strings.Repeat("a", 40),
		RefreshToken:     strings.Repeat("r", 40),
		AccessExpiresAt:  now.Add(15 * time.Minute),
		RefreshExpiresAt: now.Add(7 * 24 * time.Hour),
		User:             domain.User{PublicID: "d9428888-122b-11e1-b85c-61cd3cbb3210", Email: "person@example.test", DisplayName: "Person", Role: domain.RoleUser, Status: domain.StatusActive, CreatedAt: now},
	}
}
