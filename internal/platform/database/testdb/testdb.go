// Package testdb opens the dedicated test database and applies migrations for
// repository integration tests.
package testdb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Harshwagh21/NimbusCloud/db"
)

const connectTimeout = 10 * time.Second

// Open migrates nimbus_test to the latest schema and returns a pool. Every test
// package that needs Postgres should call this from a helper.
func Open(t testing.TB) *pgxpool.Pool {
	t.Helper()
	databaseURL := requireTestDatabaseURL(t)
	if err := migrateUp(databaseURL); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return openPool(t, databaseURL)
}

// TruncateUsers clears user rows between tests so cases do not share state.
func TruncateUsers(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "TRUNCATE users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate users: %v", err)
	}
}

func requireTestDatabaseURL(t testing.TB) string {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for repository integration tests")
	}
	return databaseURL
}

func openPool(t testing.TB, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	return pool
}

func migrateUp(databaseURL string) error {
	source, err := iofs.New(db.MigrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}

	migrator, err := migrate.NewWithSourceInstance("iofs", source, pgx5URL(databaseURL))
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() { _, _ = migrator.Close() }()

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// migrate's pgx/v5 driver expects the pgx5 scheme, not postgres://.
func pgx5URL(databaseURL string) string {
	return strings.Replace(databaseURL, "postgres://", "pgx5://", 1)
}
