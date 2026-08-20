// Package database provides the single connection-pool foundation every
// later module builds its repositories on. M01 only establishes the pool
// and health check — no schema, no queries, no business tables.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxConns          = 10
	healthCheckPeriod = 30 * time.Second
	connectTimeout    = 10 * time.Second
)

// NewPool creates a PostgreSQL connection pool and verifies connectivity
// with an initial ping before returning, so configuration mistakes are
// caught at startup rather than on the first request.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}
	poolCfg.MaxConns = maxConns
	poolCfg.HealthCheckPeriod = healthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
