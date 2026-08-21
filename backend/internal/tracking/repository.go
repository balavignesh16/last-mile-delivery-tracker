package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lastmiletracker/internal/users"
)

// Repository is the storage interface the tracking handlers depend on.
// An interface — rather than the concrete *PostgresRepository — so
// handler unit tests can inject an in-memory fake, the same pattern
// every prior module in this project uses.
type Repository interface {
	// Transition validates and applies one status change atomically:
	// lock the order row, check the edge is legal, check role is
	// authorized for that specific edge, update orders.status, and
	// insert the resulting tracking event — all inside one transaction.
	// Returns ErrOrderNotFound, ErrInvalidTransition, or
	// ErrForbiddenTransition on failure.
	Transition(ctx context.Context, orderID, actorID string, role users.Role, newStatus Status, metadata json.RawMessage) (Event, error)
	// OrderCustomerID returns the customer_id of the named order, for
	// the ownership check GetTrackingHandler needs before it may show a
	// CUSTOMER caller an order's timeline. Returns ErrOrderNotFound if
	// unknown.
	OrderCustomerID(ctx context.Context, orderID string) (string, error)
	// ListEvents returns every tracking event for one order, oldest
	// first — the chronological order the blueprint's own worked
	// example uses.
	ListEvents(ctx context.Context, orderID string) ([]Event, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const eventColumns = `id, order_id, previous_status, new_status, actor_id, metadata, created_at`

// Transition is the one place this module ever writes. The SELECT ...
// FOR UPDATE lock on the parent order is what actually prevents two
// concurrent, conflicting transition attempts from both succeeding: a
// plain "read status, validate in Go, write" sequence would let two
// concurrent requests both read the same pre-transition status, both
// pass validation, and both commit — silently corrupting the state
// machine (e.g. an order ending up DELIVERED and then immediately
// overwritten to FAILED). The lock serializes every transition attempt
// on a given order; a second request unblocks only after the first
// commits, and re-reads the now-current (already-changed) status, so
// its own validation runs against reality rather than a stale read.
// This is the exact mechanism (and exact reasoning) M05's
// lockRateCard uses for concurrent slab writes.
func (r *PostgresRepository) Transition(ctx context.Context, orderID, actorID string, role users.Role, newStatus Status, metadata json.RawMessage) (Event, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Event{}, fmt.Errorf("begin transition transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	var currentStatus string
	err = tx.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Event{}, ErrOrderNotFound
		}
		return Event{}, fmt.Errorf("lock order: %w", err)
	}

	from := Status(currentStatus)
	if !IsValidTransition(from, newStatus) {
		return Event{}, ErrInvalidTransition
	}
	if !IsRoleAuthorized(from, newStatus, role) {
		return Event{}, ErrForbiddenTransition
	}

	if _, err := tx.Exec(ctx, `UPDATE orders SET status = $1 WHERE id = $2`, string(newStatus), orderID); err != nil {
		return Event{}, fmt.Errorf("update order status: %w", err)
	}

	prevStatus := string(from)
	var metadataParam any
	if len(metadata) > 0 {
		metadataParam = []byte(metadata)
	}

	const stmt = `
		INSERT INTO order_tracking_events (order_id, previous_status, new_status, actor_id, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + eventColumns
	event, err := scanEvent(tx.QueryRow(ctx, stmt, orderID, prevStatus, string(newStatus), actorID, metadataParam))
	if err != nil {
		return Event{}, fmt.Errorf("record tracking event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Event{}, fmt.Errorf("commit transition: %w", err)
	}
	return event, nil
}

func (r *PostgresRepository) OrderCustomerID(ctx context.Context, orderID string) (string, error) {
	var customerID string
	err := r.pool.QueryRow(ctx, `SELECT customer_id FROM orders WHERE id = $1`, orderID).Scan(&customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrOrderNotFound
		}
		return "", fmt.Errorf("find order customer: %w", err)
	}
	return customerID, nil
}

func (r *PostgresRepository) ListEvents(ctx context.Context, orderID string) ([]Event, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+eventColumns+` FROM order_tracking_events WHERE order_id = $1 ORDER BY created_at ASC`, orderID)
	if err != nil {
		return nil, fmt.Errorf("list tracking events: %w", err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tracking event: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tracking events: %w", err)
	}
	return out, nil
}

// rowScanner abstracts over pgx.Row (QueryRow) and pgx.Rows (Query's
// iteration) so single-row and list queries can share one scan
// function — same pattern as internal/orders, internal/rates, and
// internal/zones.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanEvent(row rowScanner) (Event, error) {
	var e Event
	var metadata []byte
	err := row.Scan(&e.ID, &e.OrderID, &e.PreviousStatus, &e.NewStatus, &e.ActorID, &metadata, &e.CreatedAt)
	if err != nil {
		return Event{}, err
	}
	if metadata != nil {
		e.Metadata = json.RawMessage(metadata)
	}
	return e, nil
}
