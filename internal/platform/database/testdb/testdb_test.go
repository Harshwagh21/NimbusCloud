package testdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHoldTestLockBlocksASecondSession(t *testing.T) {
	pool := openPool(t, requireTestDatabaseURL(t))
	holdTestLock(t, pool)

	err := tryLock(t, pool)
	if err == nil {
		t.Fatal("second session acquired the test lock while it was held")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second session failed for another reason: %v", err)
	}
}

func tryLock(t *testing.T, pool *pgxpool.Pool) error {
	t.Helper()
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire second session: %v", err)
	}
	defer conn.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err = conn.Exec(ctx, "SELECT pg_advisory_lock($1)", testDatabaseLock)
	if err == nil {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", testDatabaseLock)
	}
	return err
}
