//go:build integration

package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCheckMigrationsFailsClosedForDirtyAndUnexpectedVersion(t *testing.T) {
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
	defer pool.Close()
	database := &Database{pool: pool}

	for _, test := range []struct {
		name          string
		version       int
		dirty         bool
		wantErrorText string
	}{
		{name: "dirty", version: int(ExpectedMigrationVersion), dirty: true, wantErrorText: "dirty"},
		{name: "unexpected version", version: int(ExpectedMigrationVersion) - 1, dirty: false, wantErrorText: "expected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, `UPDATE public.schema_migrations SET version = $1, dirty = $2`, test.version, test.dirty); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = pool.Exec(context.Background(), `UPDATE public.schema_migrations SET version = $1, dirty = false`, ExpectedMigrationVersion)
			})
			if err := database.CheckMigrations(ctx); err == nil || !strings.Contains(err.Error(), test.wantErrorText) {
				t.Fatalf("CheckMigrations() error = %v", err)
			}
			if _, err := pool.Exec(ctx, `UPDATE public.schema_migrations SET version = $1, dirty = false`, ExpectedMigrationVersion); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := database.CheckMigrations(ctx); err != nil {
		t.Fatalf("restored migration state rejected: %v", err)
	}
}
