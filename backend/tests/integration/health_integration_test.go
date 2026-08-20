//go:build integration

// Package integration holds tests that require a real running PostgreSQL
// instance (e.g. via `docker compose up -d postgres`). They are excluded
// from the default `go test ./...` run via the integration build tag, so
// unit tests never accidentally depend on external infrastructure.
//
// Run with:
//
//	TEST_DATABASE_URL=postgres://lastmile:lastmile@localhost:5432/lastmile?sslmode=disable \
//	    go test -tags=integration ./tests/integration/...
package integration

import (
	"context"
	"os"
	"testing"

	"lastmiletracker/internal/database"
)

func TestDatabasePool_ConnectsAndPings(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test (see package doc for how to run it)")
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPool() failed against a real database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Ping() failed: %v", err)
	}
}
