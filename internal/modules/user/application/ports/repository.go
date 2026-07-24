package ports

import (
	"context"
	"errors"
	"time"

	"github.com/joinname177/dify-agent-api/internal/modules/user/domain"
)

var (
	ErrEmailConflict      = errors.New("email is already registered")
	ErrUserNotFound       = errors.New("user was not found")
	ErrSessionNotFound    = errors.New("session was not found")
	ErrSuperAdminConflict = errors.New("bootstrap email belongs to a non-super-admin user")
)

type NewUser struct {
	Email        string
	DisplayName  string
	PasswordHash string
	Role         domain.Role
}

type NewSession struct {
	AccessTokenHash  []byte
	RefreshTokenHash []byte
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

type Repository interface {
	CreateUserWithSession(context.Context, NewUser, NewSession) (domain.User, error)
	FindUserByEmail(context.Context, string) (domain.User, error)
	CreateSession(context.Context, int64, NewSession) error
	RotateSession(context.Context, []byte, NewSession, time.Time) (domain.User, error)
	RevokeSession(context.Context, []byte, time.Time) error
	FindUserByAccessToken(context.Context, []byte, time.Time) (domain.User, error)
	EnsureSuperAdmin(context.Context, NewUser) (bool, error)
}

type PasswordHasher interface {
	Hash(string) (string, error)
	Verify(string, string) bool
}

type Token struct {
	Raw  string
	Hash []byte
}

type TokenIssuer interface {
	Issue() (Token, error)
	Hash(string) []byte
}
