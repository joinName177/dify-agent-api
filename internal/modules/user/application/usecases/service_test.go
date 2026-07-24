package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/joinname177/dify-agent-api/internal/modules/user/application/ports"
	"github.com/joinname177/dify-agent-api/internal/modules/user/domain"
)

type fakeRepository struct {
	user                          domain.User
	createErr, findErr, rotateErr error
	createdUser                   ports.NewUser
	createdSession                ports.NewSession
}

func (f *fakeRepository) CreateUserWithSession(_ context.Context, user ports.NewUser, session ports.NewSession) (domain.User, error) {
	f.createdUser, f.createdSession = user, session
	return f.user, f.createErr
}
func (f *fakeRepository) FindUserByEmail(context.Context, string) (domain.User, error) {
	return f.user, f.findErr
}
func (f *fakeRepository) CreateSession(_ context.Context, _ int64, session ports.NewSession) error {
	f.createdSession = session
	return nil
}
func (f *fakeRepository) RotateSession(context.Context, []byte, ports.NewSession, time.Time) (domain.User, error) {
	return f.user, f.rotateErr
}
func (*fakeRepository) RevokeSession(context.Context, []byte, time.Time) error { return nil }
func (f *fakeRepository) FindUserByAccessToken(context.Context, []byte, time.Time) (domain.User, error) {
	return f.user, f.findErr
}
func (*fakeRepository) EnsureSuperAdmin(context.Context, ports.NewUser) (bool, error) {
	return true, nil
}

type fakePasswordHasher struct {
	verify      bool
	verifyCalls int
}

func (fakePasswordHasher) Hash(string) (string, error)   { return "password-hash", nil }
func (f *fakePasswordHasher) Verify(string, string) bool { f.verifyCalls++; return f.verify }

type fakeTokenIssuer struct{ next byte }

func (f *fakeTokenIssuer) Issue() (ports.Token, error) {
	f.next++
	return ports.Token{Raw: string([]byte{'t', f.next}), Hash: []byte{f.next}}, nil
}
func (*fakeTokenIssuer) Hash(raw string) []byte { return []byte(raw) }

func newTestService(t *testing.T, repository ports.Repository, hasher ports.PasswordHasher) *Service {
	t.Helper()
	service, err := NewService(repository, hasher, &fakeTokenIssuer{}, 15*time.Minute, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	service.now = func() time.Time { return time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC) }
	return service
}

func TestRegisterNormalizesUserAndCreatesSession(t *testing.T) {
	repository := &fakeRepository{user: domain.User{ID: 7, Role: domain.RoleUser, Status: domain.StatusActive}}
	hasher := &fakePasswordHasher{verify: true}
	service := newTestService(t, repository, hasher)
	session, err := service.Register(context.Background(), Registration{Email: " Person@Example.Test ", Password: "long-enough-password", DisplayName: " Person "})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if repository.createdUser.Email != "person@example.test" || repository.createdUser.DisplayName != "Person" {
		t.Fatalf("user was not normalized: %+v", repository.createdUser)
	}
	if repository.createdUser.Role != domain.RoleUser {
		t.Fatalf("registration assigned role %q", repository.createdUser.Role)
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatal("session tokens were not issued")
	}
}

func TestRegisterMapsDuplicateEmailToStableError(t *testing.T) {
	service := newTestService(t, &fakeRepository{createErr: ports.ErrEmailConflict}, &fakePasswordHasher{})
	_, err := service.Register(context.Background(), Registration{Email: "person@example.test", Password: "long-enough-password", DisplayName: "Person"})
	if !errors.Is(err, ErrEmailAlreadyRegistered) {
		t.Fatalf("got %v", err)
	}
}

func TestLoginUsesGenericErrorForUnknownUser(t *testing.T) {
	hasher := &fakePasswordHasher{}
	service := newTestService(t, &fakeRepository{findErr: ports.ErrUserNotFound}, hasher)
	_, err := service.Login(context.Background(), Credentials{Email: "missing@example.test", Password: "password"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("got %v", err)
	}
	if hasher.verifyCalls != 1 {
		t.Fatalf("dummy hash verification calls = %d", hasher.verifyCalls)
	}
}

func TestRegistrationRejectsShortPassword(t *testing.T) {
	service := newTestService(t, &fakeRepository{}, &fakePasswordHasher{})
	_, err := service.Register(context.Background(), Registration{Email: "person@example.test", Password: "short", DisplayName: "Person"})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("got %v", err)
	}
}
