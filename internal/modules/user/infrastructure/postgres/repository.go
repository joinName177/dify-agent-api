package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joinname177/dify-agent-api/internal/modules/user/application/ports"
	"github.com/joinname177/dify-agent-api/internal/modules/user/domain"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) CreateUserWithSession(ctx context.Context, user ports.NewUser, session ports.NewSession) (domain.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("begin create user transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := scanUser(tx.QueryRow(ctx, `
		INSERT INTO app.users (email, display_name, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, public_id::text, email, display_name, password_hash, role, status, created_at`, user.Email, user.DisplayName, user.PasswordHash, user.Role))
	if isConstraint(err, "users_email_unique") {
		return domain.User{}, ports.ErrEmailConflict
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("insert user: %w", err)
	}
	if err := insertSession(ctx, tx, created.ID, session); err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, fmt.Errorf("commit create user transaction: %w", err)
	}
	return created, nil
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (domain.User, error) {
	user, err := scanUser(r.pool.QueryRow(ctx, `
		SELECT id, public_id::text, email, display_name, password_hash, role, status, created_at
		FROM app.users WHERE email = $1`, email))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ports.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("select user by email: %w", err)
	}
	return user, nil
}

func (r *Repository) CreateSession(ctx context.Context, userID int64, session ports.NewSession) error {
	if err := insertSession(ctx, r.pool, userID, session); err != nil {
		return err
	}
	return nil
}

func (r *Repository) RotateSession(ctx context.Context, currentRefreshHash []byte, replacement ports.NewSession, now time.Time) (domain.User, error) {
	user, err := scanUser(r.pool.QueryRow(ctx, `
		UPDATE app.auth_sessions AS session
		SET access_token_hash = $2, refresh_token_hash = $3,
		    access_expires_at = $4, refresh_expires_at = $5, updated_at = $6
		FROM app.users AS users
		WHERE session.user_id = users.id
		  AND session.refresh_token_hash = $1
		  AND session.revoked_at IS NULL
		  AND session.refresh_expires_at > $6
		  AND users.status = 'active'
		RETURNING users.id, users.public_id::text, users.email, users.display_name,
		          users.password_hash, users.role, users.status, users.created_at`,
		currentRefreshHash, replacement.AccessTokenHash, replacement.RefreshTokenHash,
		replacement.AccessExpiresAt, replacement.RefreshExpiresAt, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ports.ErrSessionNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("rotate session: %w", err)
	}
	return user, nil
}

func (r *Repository) RevokeSession(ctx context.Context, refreshHash []byte, now time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE app.auth_sessions SET revoked_at = $2, updated_at = $2
		WHERE refresh_token_hash = $1 AND revoked_at IS NULL`, refreshHash, now)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (r *Repository) FindUserByAccessToken(ctx context.Context, accessHash []byte, now time.Time) (domain.User, error) {
	user, err := scanUser(r.pool.QueryRow(ctx, `
		SELECT users.id, users.public_id::text, users.email, users.display_name,
		       users.password_hash, users.role, users.status, users.created_at
		FROM app.auth_sessions AS session
		JOIN app.users AS users ON users.id = session.user_id
		WHERE session.access_token_hash = $1
		  AND session.revoked_at IS NULL
		  AND session.access_expires_at > $2
		  AND users.status = 'active'`, accessHash, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ports.ErrSessionNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("select user by access token: %w", err)
	}
	return user, nil
}

func (r *Repository) EnsureSuperAdmin(ctx context.Context, user ports.NewUser) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin super admin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, user.Email); err != nil {
		return false, fmt.Errorf("lock super admin identity: %w", err)
	}
	var role domain.Role
	err = tx.QueryRow(ctx, `SELECT role FROM app.users WHERE email = $1 FOR UPDATE`, user.Email).Scan(&role)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := tx.Exec(ctx, `INSERT INTO app.users (email, display_name, password_hash, role) VALUES ($1, $2, $3, $4)`, user.Email, user.DisplayName, user.PasswordHash, user.Role); err != nil {
			return false, fmt.Errorf("insert super admin: %w", err)
		}
	case err != nil:
		return false, fmt.Errorf("select super admin: %w", err)
	case role != domain.RoleSuperAdmin:
		return false, ports.ErrSuperAdminConflict
	default:
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit existing super admin: %w", err)
		}
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit new super admin: %w", err)
	}
	return true, nil
}

type rowScanner interface{ Scan(...any) error }
type queryExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func scanUser(row rowScanner) (domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.PublicID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Role, &user.Status, &user.CreatedAt)
	return user, err
}

func insertSession(ctx context.Context, executor queryExecutor, userID int64, session ports.NewSession) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO app.auth_sessions
		    (user_id, access_token_hash, refresh_token_hash, access_expires_at, refresh_expires_at)
		VALUES ($1, $2, $3, $4, $5)`, userID, session.AccessTokenHash, session.RefreshTokenHash, session.AccessExpiresAt, session.RefreshExpiresAt)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

func isConstraint(err error, constraint string) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.ConstraintName == constraint
}
