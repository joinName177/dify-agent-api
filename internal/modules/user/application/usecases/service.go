package usecases

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/joinname177/dify-agent-api/internal/modules/user/application/ports"
	"github.com/joinname177/dify-agent-api/internal/modules/user/domain"
	platformauth "github.com/joinname177/dify-agent-api/internal/platform/auth"
)

type Service struct {
	repository        ports.Repository
	passwordHasher    ports.PasswordHasher
	tokenIssuer       ports.TokenIssuer
	dummyPasswordHash string
	accessTokenTTL    time.Duration
	refreshTokenTTL   time.Duration
	now               func() time.Time
}

type Registration struct{ Email, Password, DisplayName string }
type Credentials struct{ Email, Password string }

type AuthSession struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
	User             domain.User
}

func NewService(repository ports.Repository, passwordHasher ports.PasswordHasher, tokenIssuer ports.TokenIssuer, accessTTL, refreshTTL time.Duration) (*Service, error) {
	dummyHash, err := passwordHasher.Hash("constant-time-dummy-password")
	if err != nil {
		return nil, fmt.Errorf("create dummy password hash: %w", err)
	}
	return &Service{repository: repository, passwordHasher: passwordHasher, tokenIssuer: tokenIssuer, dummyPasswordHash: dummyHash, accessTokenTTL: accessTTL, refreshTokenTTL: refreshTTL, now: time.Now}, nil
}

func (s *Service) Register(ctx context.Context, input Registration) (AuthSession, error) {
	email, displayName, validationErr := validateRegistration(input)
	if validationErr != nil {
		return AuthSession{}, validationErr
	}
	passwordHash, err := s.passwordHasher.Hash(input.Password)
	if err != nil {
		return AuthSession{}, fmt.Errorf("hash password: %w", err)
	}
	session, storedSession, err := s.issueSession()
	if err != nil {
		return AuthSession{}, err
	}
	user, err := s.repository.CreateUserWithSession(ctx, ports.NewUser{Email: email, DisplayName: displayName, PasswordHash: passwordHash, Role: domain.RoleUser}, storedSession)
	if errors.Is(err, ports.ErrEmailConflict) {
		return AuthSession{}, ErrEmailAlreadyRegistered
	}
	if err != nil {
		return AuthSession{}, fmt.Errorf("create user: %w", err)
	}
	session.User = user
	return session, nil
}

func (s *Service) Login(ctx context.Context, input Credentials) (AuthSession, error) {
	email, validationErr := validateCredentials(input)
	if validationErr != nil {
		return AuthSession{}, validationErr
	}
	user, err := s.repository.FindUserByEmail(ctx, email)
	if errors.Is(err, ports.ErrUserNotFound) {
		s.passwordHasher.Verify(input.Password, s.dummyPasswordHash)
		return AuthSession{}, ErrInvalidCredentials
	}
	if err != nil {
		return AuthSession{}, fmt.Errorf("find user: %w", err)
	}
	if !s.passwordHasher.Verify(input.Password, user.PasswordHash) {
		return AuthSession{}, ErrInvalidCredentials
	}
	if user.Status != domain.StatusActive {
		return AuthSession{}, ErrAccountDisabled
	}
	session, storedSession, err := s.issueSession()
	if err != nil {
		return AuthSession{}, err
	}
	if err := s.repository.CreateSession(ctx, user.ID, storedSession); err != nil {
		return AuthSession{}, fmt.Errorf("create session: %w", err)
	}
	session.User = user
	return session, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (AuthSession, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return AuthSession{}, ErrUnauthenticated
	}
	session, storedSession, err := s.issueSession()
	if err != nil {
		return AuthSession{}, err
	}
	user, err := s.repository.RotateSession(ctx, s.tokenIssuer.Hash(refreshToken), storedSession, s.now().UTC())
	if errors.Is(err, ports.ErrSessionNotFound) {
		return AuthSession{}, ErrUnauthenticated
	}
	if err != nil {
		return AuthSession{}, fmt.Errorf("rotate session: %w", err)
	}
	if user.Status != domain.StatusActive {
		return AuthSession{}, ErrAccountDisabled
	}
	session.User = user
	return session, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return nil
	}
	if err := s.repository.RevokeSession(ctx, s.tokenIssuer.Hash(refreshToken), s.now().UTC()); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *Service) Authenticate(ctx context.Context, accessToken string) (platformauth.Identity, error) {
	if strings.TrimSpace(accessToken) == "" {
		return platformauth.Identity{}, ErrUnauthenticated
	}
	user, err := s.repository.FindUserByAccessToken(ctx, s.tokenIssuer.Hash(accessToken), s.now().UTC())
	if errors.Is(err, ports.ErrSessionNotFound) {
		return platformauth.Identity{}, ErrUnauthenticated
	}
	if err != nil {
		return platformauth.Identity{}, fmt.Errorf("authenticate session: %w", err)
	}
	if user.Status != domain.StatusActive {
		return platformauth.Identity{}, ErrAccountDisabled
	}
	return platformauth.Identity{UserID: user.ID, PublicID: user.PublicID, Email: user.Email, DisplayName: user.DisplayName, Role: string(user.Role), CreatedAt: user.CreatedAt}, nil
}

func (s *Service) BootstrapSuperAdmin(ctx context.Context, email, password string) (bool, error) {
	normalizedEmail, displayName, validationErr := validateRegistration(Registration{Email: email, Password: password, DisplayName: "Administrator"})
	if validationErr != nil {
		return false, fmt.Errorf("bootstrap super admin: %w", validationErr)
	}
	passwordHash, err := s.passwordHasher.Hash(password)
	if err != nil {
		return false, fmt.Errorf("hash bootstrap password: %w", err)
	}
	created, err := s.repository.EnsureSuperAdmin(ctx, ports.NewUser{Email: normalizedEmail, DisplayName: displayName, PasswordHash: passwordHash, Role: domain.RoleSuperAdmin})
	if err != nil {
		return false, fmt.Errorf("ensure bootstrap super admin: %w", err)
	}
	return created, nil
}

func (s *Service) issueSession() (AuthSession, ports.NewSession, error) {
	accessToken, err := s.tokenIssuer.Issue()
	if err != nil {
		return AuthSession{}, ports.NewSession{}, fmt.Errorf("issue access token: %w", err)
	}
	refreshToken, err := s.tokenIssuer.Issue()
	if err != nil {
		return AuthSession{}, ports.NewSession{}, fmt.Errorf("issue refresh token: %w", err)
	}
	now := s.now().UTC()
	accessExpiry, refreshExpiry := now.Add(s.accessTokenTTL), now.Add(s.refreshTokenTTL)
	return AuthSession{AccessToken: accessToken.Raw, RefreshToken: refreshToken.Raw, AccessExpiresAt: accessExpiry, RefreshExpiresAt: refreshExpiry}, ports.NewSession{AccessTokenHash: accessToken.Hash, RefreshTokenHash: refreshToken.Hash, AccessExpiresAt: accessExpiry, RefreshExpiresAt: refreshExpiry}, nil
}

func validateRegistration(input Registration) (string, string, error) {
	email, emailErr := normalizeEmail(input.Email)
	displayName := strings.TrimSpace(input.DisplayName)
	fields := make([]FieldError, 0, 3)
	if emailErr != nil {
		fields = append(fields, FieldError{Field: "email", Code: "INVALID_EMAIL", Message: "Enter a valid email address."})
	}
	passwordLength := utf8.RuneCountInString(input.Password)
	if passwordLength < 12 || passwordLength > 128 || len(input.Password) > 512 {
		fields = append(fields, FieldError{Field: "password", Code: "INVALID_PASSWORD_LENGTH", Message: "Password must contain between 12 and 128 characters."})
	}
	if length := utf8.RuneCountInString(displayName); length < 1 || length > 80 {
		fields = append(fields, FieldError{Field: "displayName", Code: "INVALID_DISPLAY_NAME", Message: "Display name must contain between 1 and 80 characters."})
	}
	if len(fields) > 0 {
		return "", "", &ValidationError{Fields: fields}
	}
	return email, displayName, nil
}

func validateCredentials(input Credentials) (string, error) {
	email, emailErr := normalizeEmail(input.Email)
	fields := make([]FieldError, 0, 2)
	if emailErr != nil {
		fields = append(fields, FieldError{Field: "email", Code: "INVALID_EMAIL", Message: "Enter a valid email address."})
	}
	if input.Password == "" || utf8.RuneCountInString(input.Password) > 128 || len(input.Password) > 512 {
		fields = append(fields, FieldError{Field: "password", Code: "INVALID_PASSWORD", Message: "Enter a valid password."})
	}
	if len(fields) > 0 {
		return "", &ValidationError{Fields: fields}
	}
	return email, nil
}

func normalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if len(email) < 3 || len(email) > 254 {
		return "", errors.New("email length is invalid")
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", errors.New("email is invalid")
	}
	return email, nil
}
