//go:build integration

package migrations_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigratedSchemaContract(t *testing.T) {
	databaseURL := os.Getenv("DIFY_AGENT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DIFY_AGENT_DATABASE_URL is required for migration integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var version int
	var dirty bool
	if err := pool.QueryRow(ctx, `SELECT version, dirty FROM public.schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != 2 || dirty {
		t.Fatalf("migration state = version %d dirty %t; want version 2 clean", version, dirty)
	}

	assertConstraint(t, ctx, pool, "users_email_normalized", "c")
	assertConstraint(t, ctx, pool, "auth_sessions_user_id_fkey", "f")
	assertIndex(t, ctx, pool, "users_email_unique", true)
	assertIndex(t, ctx, pool, "auth_sessions_active_refresh_index", false)

	if _, err := pool.Exec(ctx, `INSERT INTO app.users (email, display_name, password_hash) VALUES ('UPPER@example.test', 'User', 'hash')`); err == nil {
		t.Fatal("expected normalized-email constraint to reject uppercase email")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO app.auth_sessions (user_id, access_token_hash, refresh_token_hash, access_expires_at, refresh_expires_at) VALUES (999999, '\x01', '\x02', now(), now())`); err == nil {
		t.Fatal("expected session foreign key to reject missing user")
	}
}

func assertConstraint(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, kind string) {
	t.Helper()
	var found bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = $1 AND contype::text = $2)`, name, kind).Scan(&found); err != nil || !found {
		t.Fatalf("constraint %s kind %s missing: %v", name, kind, err)
	}
}

func assertIndex(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, unique bool) {
	t.Helper()
	var found bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid WHERE c.relname = $1 AND i.indisunique = $2)`, name, unique).Scan(&found); err != nil || !found {
		t.Fatalf("index %s unique=%t missing: %v", name, unique, err)
	}
}
