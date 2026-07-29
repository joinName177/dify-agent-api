//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joinname177/dify-agent-api/internal/modules/user/application/ports"
	"github.com/joinname177/dify-agent-api/internal/modules/user/domain"
)

func TestRepositoryTransactionConstraintsAndAdminConflict(t *testing.T) {
	pool := integrationPool(t)
	repository := NewRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC()

	invalidSession := newSession("rollback", now)
	invalidSession.RefreshExpiresAt = invalidSession.AccessExpiresAt.Add(-time.Minute)
	if _, err := repository.CreateUserWithSession(ctx, newUser("rollback@example.test"), invalidSession); err == nil {
		t.Fatal("expected invalid session to fail")
	}
	if _, err := repository.FindUserByEmail(ctx, "rollback@example.test"); !errors.Is(err, ports.ErrUserNotFound) {
		t.Fatalf("user insertion was not rolled back: %v", err)
	}

	user, err := repository.CreateUserWithSession(ctx, newUser("person@example.test"), newSession("initial", now))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateUserWithSession(ctx, newUser("person@example.test"), newSession("duplicate", now)); !errors.Is(err, ports.ErrEmailConflict) {
		t.Fatalf("duplicate email error = %v", err)
	}
	if err := repository.CreateSession(ctx, user.ID+999999, newSession("foreign", now)); err == nil {
		t.Fatal("expected foreign-key violation")
	}
	if _, err := repository.EnsureSuperAdmin(ctx, ports.NewUser{Email: user.Email, DisplayName: user.DisplayName, PasswordHash: "hash", Role: domain.RoleSuperAdmin}); !errors.Is(err, ports.ErrSuperAdminConflict) {
		t.Fatalf("administrator identity collision did not fail closed: %v", err)
	}
}

func TestRepositoryRefreshRotationIsAtomicAndRevocationIsEnforced(t *testing.T) {
	pool := integrationPool(t)
	repository := NewRepository(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	current := newSession("current", now)
	user, err := repository.CreateUserWithSession(ctx, newUser("rotation@example.test"), current)
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := repository.RotateSession(ctx, current.RefreshTokenHash, newSession(string(rune('a'+index)), now), now)
			results <- err
		}(i)
	}
	wait.Wait()
	close(results)
	var successes, rejected int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ports.ErrSessionNotFound):
			rejected++
		default:
			t.Fatalf("unexpected rotation error: %v", err)
		}
	}
	if successes != 1 || rejected != 1 {
		t.Fatalf("concurrent rotations: successes=%d rejected=%d", successes, rejected)
	}
	if _, err := repository.RotateSession(ctx, current.RefreshTokenHash, newSession("replay", now), now); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("old refresh token replay was accepted: %v", err)
	}

	revoked := newSession("revoked", now)
	if err := repository.CreateSession(ctx, user.ID, revoked); err != nil {
		t.Fatal(err)
	}
	if err := repository.RevokeSession(ctx, revoked.RefreshTokenHash, now); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.FindUserByAccessToken(ctx, revoked.AccessTokenHash, now); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("revoked session remained usable: %v", err)
	}

	expired := newSession("expired", now.Add(-2*time.Hour))
	expired.AccessExpiresAt = now.Add(-time.Hour)
	expired.RefreshExpiresAt = now.Add(time.Hour)
	if err := repository.CreateSession(ctx, user.ID, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.FindUserByAccessToken(ctx, expired.AccessTokenHash, now); !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("expired access token remained usable: %v", err)
	}
}

func TestRepositoryHonorsCancelledContext(t *testing.T) {
	pool := integrationPool(t)
	repository := NewRepository(pool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repository.FindUserByEmail(ctx, "nobody@example.test"); err == nil || errors.Is(err, ports.ErrUserNotFound) {
		t.Fatalf("cancelled context was not propagated: %v", err)
	}
}

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DIFY_AGENT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DIFY_AGENT_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE app.auth_sessions, app.users RESTART IDENTITY CASCADE`); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newUser(email string) ports.NewUser {
	return ports.NewUser{Email: email, DisplayName: "Integration User", PasswordHash: "hash", Role: domain.RoleUser}
}

func newSession(seed string, now time.Time) ports.NewSession {
	return ports.NewSession{
		AccessTokenHash: []byte("access-" + seed), RefreshTokenHash: []byte("refresh-" + seed),
		AccessExpiresAt: now.Add(15 * time.Minute), RefreshExpiresAt: now.Add(time.Hour),
	}
}
