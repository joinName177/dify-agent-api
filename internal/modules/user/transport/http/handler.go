package userhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	openapiv1 "github.com/joinname177/dify-agent-api/generated/openapi/v1"
	"github.com/joinname177/dify-agent-api/internal/modules/user/application/usecases"
	"github.com/joinname177/dify-agent-api/internal/modules/user/domain"
	platformauth "github.com/joinname177/dify-agent-api/internal/platform/auth"
	platformhttp "github.com/joinname177/dify-agent-api/internal/platform/httputil"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

const refreshCookieName = "dify_agent_refresh"

type Handler struct {
	service        service
	logger         *slog.Logger
	allowedOrigins map[string]struct{}
	secureCookies  bool
}

type service interface {
	Register(context.Context, usecases.Registration) (usecases.AuthSession, error)
	Login(context.Context, usecases.Credentials) (usecases.AuthSession, error)
	Refresh(context.Context, string) (usecases.AuthSession, error)
	Logout(context.Context, string) error
}

func NewHandler(service service, logger *slog.Logger, allowedOrigins []string, secureCookies bool) *Handler {
	origins := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origins[origin] = struct{}{}
	}
	return &Handler{service: service, logger: logger, allowedOrigins: origins, secureCookies: secureCookies}
}

func (h *Handler) RegisterUser(w http.ResponseWriter, r *http.Request) {
	if !h.validateOrigin(w, r) {
		return
	}
	var request openapiv1.RegisterRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	password := ""
	if request.Password != nil {
		password = *request.Password
	}
	session, err := h.service.Register(r.Context(), usecases.Registration{Email: string(request.Email), Password: password, DisplayName: request.DisplayName})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeSession(w, r, http.StatusCreated, "User registered.", session)
}

func (h *Handler) LoginUser(w http.ResponseWriter, r *http.Request) {
	if !h.validateOrigin(w, r) {
		return
	}
	var request openapiv1.LoginRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	password := ""
	if request.Password != nil {
		password = *request.Password
	}
	session, err := h.service.Login(r.Context(), usecases.Credentials{Email: string(request.Email), Password: password})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeSession(w, r, http.StatusOK, "Authentication succeeded.", session)
}

func (h *Handler) RefreshSession(w http.ResponseWriter, r *http.Request) {
	if !h.validateOrigin(w, r) {
		return
	}
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		h.clearRefreshCookie(w)
		h.writeError(w, r, usecases.ErrUnauthenticated)
		return
	}
	session, err := h.service.Refresh(r.Context(), cookie.Value)
	if err != nil {
		h.clearRefreshCookie(w)
		h.writeError(w, r, err)
		return
	}
	h.writeSession(w, r, http.StatusOK, "Session refreshed.", session)
}

func (h *Handler) LogoutUser(w http.ResponseWriter, r *http.Request) {
	if !h.validateOrigin(w, r) {
		return
	}
	refreshToken := ""
	if cookie, err := r.Cookie(refreshCookieName); err == nil {
		refreshToken = cookie.Value
	}
	if err := h.service.Logout(r.Context(), refreshToken); err != nil {
		h.writeError(w, r, err)
		return
	}
	h.clearRefreshCookie(w)
	response := openapiv1.LogoutResponse{
		Code:      openapiv1.LogoutResponseCodeN200,
		Message:   "Session revoked.",
		RequestId: platformhttp.RequestIDFromContext(r.Context()),
	}
	response.Data.Revoked = true
	platformhttp.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	identity, err := platformauth.IdentityFromContext(r.Context())
	if err != nil {
		h.writeError(w, r, usecases.ErrUnauthenticated)
		return
	}
	response, err := identityResponse(identity)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, http.StatusOK, openapiv1.UserResponse{
		Code:      openapiv1.UserResponseCodeN200,
		Message:   "Current user retrieved.",
		RequestId: platformhttp.RequestIDFromContext(r.Context()),
		Data:      response,
	})
}

func (h *Handler) writeSession(w http.ResponseWriter, r *http.Request, status int, message string, session usecases.AuthSession) {
	user, err := userResponse(session.User)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.setRefreshCookie(w, session.RefreshToken, session.RefreshExpiresAt)
	w.Header().Set("Cache-Control", "no-store")
	platformhttp.WriteJSON(w, status, openapiv1.AuthResponse{
		Code:      openapiv1.AuthResponseCode(status),
		Message:   message,
		RequestId: platformhttp.RequestIDFromContext(r.Context()),
		Data: openapiv1.AuthSessionData{
			AccessToken: session.AccessToken,
			ExpiresAt:   session.AccessExpiresAt,
			User:        user,
		},
	})
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *usecases.ValidationError
	switch {
	case errors.As(err, &validationErr):
		fields := make([]openapiv1.FieldError, 0, len(validationErr.Fields))
		for _, field := range validationErr.Fields {
			fields = append(fields, openapiv1.FieldError{Field: field.Field, Code: field.Code, Message: field.Message})
		}
		writeAPIError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Review the highlighted fields.", &fields)
	case errors.Is(err, usecases.ErrEmailAlreadyRegistered):
		writeAPIError(w, r, http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "An account with this email already exists.", nil)
	case errors.Is(err, usecases.ErrInvalidCredentials):
		writeAPIError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email or password is invalid.", nil)
	case errors.Is(err, usecases.ErrUnauthenticated), errors.Is(err, usecases.ErrAccountDisabled):
		writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "Email, password, or session is invalid.", nil)
	default:
		h.logger.ErrorContext(r.Context(), "user request failed", "request_id", platformhttp.RequestIDFromContext(r.Context()), "error", err)
		writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.", nil)
	}
}

func (h *Handler) validateOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if _, ok := h.allowedOrigins[origin]; ok {
		return true
	}
	writeAPIError(w, r, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "Request origin is not allowed.", nil)
	return false
}

func (h *Handler) setRefreshCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{Name: refreshCookieName, Value: token, Path: "/api/v1/auth", Expires: expiresAt, MaxAge: int(time.Until(expiresAt).Seconds()), HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteLaxMode})
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: refreshCookieName, Value: "", Path: "/api/v1/auth", MaxAge: -1, Expires: time.Unix(0, 0), HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteLaxMode})
}

func decodeRequest(w http.ResponseWriter, r *http.Request, destination any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		platformhttp.WriteError(w, r, http.StatusBadRequest, "MALFORMED_JSON", "Request body must contain valid JSON.")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		platformhttp.WriteError(w, r, http.StatusBadRequest, "MALFORMED_JSON", "Request body must contain one JSON object.")
		return false
	}
	return true
}

func userResponse(user domain.User) (openapiv1.UserData, error) {
	id, err := uuid.Parse(user.PublicID)
	if err != nil {
		return openapiv1.UserData{}, errors.New("stored user id is invalid")
	}
	return openapiv1.UserData{Id: id, Email: openapi_types.Email(user.Email), DisplayName: user.DisplayName, Role: openapiv1.UserDataRole(user.Role), CreatedAt: user.CreatedAt}, nil
}

func identityResponse(identity platformauth.Identity) (openapiv1.UserData, error) {
	id, err := uuid.Parse(identity.PublicID)
	if err != nil {
		return openapiv1.UserData{}, errors.New("authenticated user id is invalid")
	}
	return openapiv1.UserData{Id: id, Email: openapi_types.Email(identity.Email), DisplayName: identity.DisplayName, Role: openapiv1.UserDataRole(identity.Role), CreatedAt: identity.CreatedAt}, nil
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, errorCode, message string, fields *[]openapiv1.FieldError) {
	platformhttp.WriteJSON(w, status, openapiv1.ErrorResponse{Code: status, ErrorCode: errorCode, Message: message, RequestId: platformhttp.RequestIDFromContext(r.Context()), Errors: fields})
}
