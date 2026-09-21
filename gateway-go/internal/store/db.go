// Package store owns the acquirer database: pool setup, migrations, and repositories.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver, needed by goose
	"github.com/pressly/goose/v3"

	"github.com/mcn/gateway-go/migrations"
)

// Pool wraps a pgx connection pool.
type Pool struct{ *pgxpool.Pool }

// Open creates a pgx connection pool against dsn.
func Open(ctx context.Context, dsn string) (*Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	return &Pool{pool}, nil
}

// Migrate applies every pending goose migration embedded in migrationsFS. Goose needs a
// database/sql connection, not a pgxpool, so it opens one separately via the "pgx" driver
// registered by pgx/v5/stdlib and closes it once done.
func Migrate(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer db.Close()

	if err := pingWithRetry(db); err != nil {
		return fmt.Errorf("wait for database: %w", err)
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// pingWithRetry tolerates the brief window right after a Postgres container reports itself
// ready but isn't yet accepting connections (observed with Testcontainers; also possible against
// the real docker-compose stack on gateway startup).
func pingWithRetry(db *sql.DB) error {
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if err = db.Ping(); err == nil {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return err
}
