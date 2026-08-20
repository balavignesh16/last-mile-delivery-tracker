package agents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// EnsureDemoAgentRecord creates a delivery_agents row for the demo
// DELIVERY_AGENT account if one doesn't already exist. It exists because
// internal/auth.SeedDemoUsers (M02) only knows how to create users rows —
// it predates this module's delivery_agents table, and correctly doesn't
// import internal/agents to avoid a real cycle. main.go calls this right
// after SeedDemoUsers, the same way it sequences migrations before
// seeding: without it, the seeded demo agent account has a DELIVERY_AGENT
// role but no operational record, and every agent endpoint 404s for it.
func (r *PostgresRepository) EnsureDemoAgentRecord(ctx context.Context, userID string, logger *slog.Logger) error {
	if _, err := r.FindByUserID(ctx, userID); err == nil {
		return nil // already has a record
	} else if !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("check demo agent record: %w", err)
	}

	if _, err := r.pool.Exec(ctx, `INSERT INTO delivery_agents (user_id) VALUES ($1)`, userID); err != nil {
		return fmt.Errorf("create demo agent record: %w", err)
	}
	logger.Info("seeded demo agent operational record", "user_id", userID)
	return nil
}
