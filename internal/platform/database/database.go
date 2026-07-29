package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	migrationinfo "github.com/joinname177/dify-agent-api/generated/migrations"
)

const ExpectedMigrationVersion = migrationinfo.ExpectedVersion

type Database struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, rawURL string, minConns, maxConns int32) (*Database, error) {
	cfg, err := pgxpool.ParseConfig(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	cfg.MinConns, cfg.MaxConns = minConns, maxConns
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	db := &Database{pool: pool}
	if err := db.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return db, nil
}

func (db *Database) Close() { db.pool.Close() }

func (db *Database) Pool() *pgxpool.Pool { return db.pool }

func (db *Database) Ping(ctx context.Context) error {
	if err := db.pool.Ping(ctx); err != nil {
		return fmt.Errorf("database ping: %w", err)
	}
	return nil
}

func (db *Database) CheckMigrations(ctx context.Context) error {
	var version int
	var dirty bool
	err := db.pool.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty)
	if err != nil {
		return fmt.Errorf("read migration state: %w", err)
	}
	if dirty {
		return errors.New("database migration state is dirty")
	}
	if uint(version) != ExpectedMigrationVersion {
		return fmt.Errorf("migration version is %d, expected %d", version, ExpectedMigrationVersion)
	}
	return nil
}
