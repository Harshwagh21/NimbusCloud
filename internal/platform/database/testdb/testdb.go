// Package testdb opens the dedicated test database and applies migrations for
// repository integration tests.
package testdb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/Harshwagh21/NimbusCloud/db"
)

const connectTimeout = 10 * time.Second

// testDatabaseLock is a session advisory lock. go test runs packages in parallel,
// and they share one database: concurrent TRUNCATE deadlocks (40P01).
// ponytail: one shared lock serializes integration tests; use a database per
// package if the suite gets slow.
const testDatabaseLock int64 = 7481901

// Open migrates nimbus_test to the latest schema and returns a pool. Every test
// package that needs Postgres should call this from a helper. The returned pool
// holds the shared test lock until the test ends.
func Open(t testing.TB) *pgxpool.Pool {
	t.Helper()
	databaseURL := requireTestDatabaseURL(t)
	pool := openPool(t, databaseURL)
	holdTestLock(t, pool)
	if err := migrateUp(databaseURL); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return pool
}

// TruncateUsers clears user rows between tests so cases do not share state.
func TruncateUsers(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	TruncateAll(t, pool)
}

// TruncateAll clears every domain table via CASCADE from users.
func TruncateAll(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "TRUNCATE users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate all tables: %v", err)
	}
}

// loadProjectEnv finds the module .env when tests run inside a nested package.
func loadProjectEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for {
		envFile := filepath.Join(dir, ".env")
		if _, statErr := os.Stat(envFile); statErr == nil {
			_ = godotenv.Load(envFile)
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func requireTestDatabaseURL(t testing.TB) string {
	t.Helper()
	loadProjectEnv()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for repository integration tests")
	}
	return databaseURL
}

func holdTestLock(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire test database lock: %v", err)
	}
	if _, err := conn.Exec(context.Background(), "SELECT pg_advisory_lock($1)", testDatabaseLock); err != nil {
		conn.Release()
		t.Fatalf("lock test database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", testDatabaseLock)
		conn.Release()
	})
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
