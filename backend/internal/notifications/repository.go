package notifications

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ClaimInput is Claim's only input — everything Repository needs to
// attempt reserving one (tracking_event_id, channel) slot.
type ClaimInput struct {
	TrackingEventID string
	OrderID         string
	Event           EventType
	Channel         Channel
	Recipient       string
}

// Repository is the storage interface Service depends on. An interface
// — rather than the concrete *PostgresRepository — so unit tests can
// inject an in-memory fake, the same pattern every prior module in this
// project uses.
type Repository interface {
	// Claim atomically reserves a (tracking_event_id, channel) slot by
	// inserting a PENDING row. Returns claimed=true and the new row's id
	// only if this call is the one that gets to actually attempt
	// delivery; claimed=false (with an empty id) if another call already
	// claimed this exact slot — Service must not invoke the provider in
	// that case. This is the idempotency mechanism itself, not just a
	// side effect of it: the claim happens strictly before any provider
	// call, so a second, concurrent attempt for the same occurrence can
	// never cause two sends, not merely two rows.
	Claim(ctx context.Context, input ClaimInput) (id string, claimed bool, err error)
	// Resolve records the final outcome of a previously claimed
	// notification attempt.
	Resolve(ctx context.Context, id string, status Status) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Claim's single INSERT ... ON CONFLICT DO NOTHING RETURNING id is the
// entire idempotency mechanism: Postgres's own unique index
// (idx_notifications_tracking_event_channel) is what actually
// guarantees at most one claim ever succeeds for a given
// (tracking_event_id, channel) pair, even under real concurrent
// invocation — no application-level locking is needed because a single
// statement is already atomic. A conflict (no row returned) is not an
// error — it means someone else already claimed this occurrence, so
// pgx.ErrNoRows is translated into claimed=false, not propagated.
func (r *PostgresRepository) Claim(ctx context.Context, input ClaimInput) (string, bool, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO notifications (tracking_event_id, order_id, event, channel, recipient, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tracking_event_id, channel) DO NOTHING
		 RETURNING id`,
		input.TrackingEventID, input.OrderID, input.Event, input.Channel, input.Recipient, StatusPending,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("claim notification: %w", err)
	}
	return id, true, nil
}

func (r *PostgresRepository) Resolve(ctx context.Context, id string, status Status) error {
	if _, err := r.pool.Exec(ctx, `UPDATE notifications SET status = $1 WHERE id = $2`, status, id); err != nil {
		return fmt.Errorf("resolve notification: %w", err)
	}
	return nil
}
