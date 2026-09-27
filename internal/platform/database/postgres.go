// Package database owns the PostgreSQL connection pool and its readiness check.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Harshwagh21/NimbusCloud/config"
)

const (
	// Neon scales to zero after inactivity, so connections are recycled rather than held
	// indefinitely, and the pool verifies a connection before handing it to a caller.
	maxConnLifetime   = time.Hour
	maxConnIdleTime   = 5 * time.Minute
	healthCheckPeriod = 30 * time.Second
	connectTimeout    = 10 * time.Second
)

// NewPool builds the pool and proves the database is reachable before returning, so a bad
// DATABASE_URL fails at startup rather than on the first query.
func NewPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	poolConfig.MaxConns = cfg.DatabaseMaxConns
	poolConfig.MaxConnLifetime = maxConnLifetime
	poolConfig.MaxConnIdleTime = maxConnIdleTime
	poolConfig.HealthCheckPeriod = healthCheckPeriod
	poolConfig.ConnConfig.ConnectTimeout = connectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("reach database: %w", err)
	}
	return pool, nil
}

// PoolChecker reports database reachability to the readiness probe.
type PoolChecker struct {
	pool *pgxpool.Pool
}

func NewPoolChecker(pool *pgxpool.Pool) PoolChecker { return PoolChecker{pool: pool} }

func (p PoolChecker) Name() string { return "postgres" }

func (p PoolChecker) Check(ctx context.Context) error { return p.pool.Ping(ctx) }
