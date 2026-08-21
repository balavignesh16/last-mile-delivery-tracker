package rescheduling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lastmiletracker/internal/agents"
	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/tracking"
	"lastmiletracker/internal/users"
)

var (
	// ErrOrderNotFound mirrors orders.ErrOrderNotFound/tracking.ErrOrderNotFound
	// — this package defines its own so a handler never needs to import
	// two other packages' sentinel errors just to map "order not found"
	// once. Same precedent as internal/assignment's own ErrOrderNotFound.
	ErrOrderNotFound = errors.New("order not found")
	// ErrInvalidTransition mirrors tracking.ErrInvalidTransition — returned
	// when the order is not currently FAILED, so FAILED->RESCHEDULED is
	// not a legal transition from its current status.
	ErrInvalidTransition = errors.New("order is not currently FAILED")
)

// RescheduleInput is Reschedule's only input. actorID is always the real,
// authenticated caller's user id (customer or admin) — the repository
// never receives or asserts a caller role, because by the time it is
// invoked, the handler has already independently authorized the call
// (CUSTOMER owns the order, or caller is ADMIN); see Reschedule's own
// doc comment for what the repository does with that authorization.
type RescheduleInput struct {
	OrderID       string
	ActorID       string
	RequestedDate time.Time
	Reason        *string
	// PreviousAgentID is order.AssignedAgentID as read by the handler's
	// own pre-check (GET-before-write, the same non-locking pattern
	// internal/assignment's Assign/AutoAssign already use for their own
	// order pre-check). It cannot go stale between that read and this
	// call: nothing can write orders.assigned_agent_id for an order
	// while its status is FAILED — internal/assignment's own write path
	// only ever fires from CREATED/RESCHEDULED, both mutually exclusive
	// with FAILED — so this value is guaranteed accurate for as long as
	// the order actually remains FAILED, which TransitionTx itself
	// re-verifies under lock before this value is ever acted on.
	PreviousAgentID *string
}

// Repository is the storage interface the rescheduling handlers depend
// on. An interface — rather than the concrete *PostgresRepository — so
// handler unit tests can inject an in-memory fake, the same pattern
// every prior module in this project uses.
type Repository interface {
	// Reschedule performs the entire M10 write atomically: lock and free
	// the previously-assigned agent (if any), transition the order
	// FAILED->RESCHEDULED via tracking.Repository's own TransitionTx
	// (M08's unmodified, authoritative state-machine write — never
	// reimplemented here), persist the reschedule_requests row, and
	// return the updated order. Any failure rolls the entire operation
	// back — nothing is left half-rescheduled.
	Reschedule(ctx context.Context, input RescheduleInput) (orders.Order, error)
	// ListReschedules returns every reschedule request for one order,
	// oldest first — the same chronological convention
	// tracking.Repository.ListEvents already uses for GET
	// /orders/:id/tracking.
	ListReschedules(ctx context.Context, orderID string) ([]Reschedule, error)
}

type PostgresRepository struct {
	pool         *pgxpool.Pool
	trackingRepo tracking.Repository
}

func NewPostgresRepository(pool *pgxpool.Pool, trackingRepo tracking.Repository) *PostgresRepository {
	return &PostgresRepository{pool: pool, trackingRepo: trackingRepo}
}

// rescheduleMetadata builds the tracking-event metadata every
// FAILED->RESCHEDULED transition carries — requested_date and, when
// supplied, reason. This is what lets order_tracking_events double as
// part of the reschedule's own audit trail without this package
// reaching into internal/tracking's storage directly (mirrors
// internal/assignment's own assignmentMetadata helper exactly).
func rescheduleMetadata(requestedDate time.Time, reason *string) json.RawMessage {
	payload := map[string]string{"requested_date": requestedDate.Format(dateLayout)}
	if reason != nil {
		payload["reason"] = *reason
	}
	b, _ := json.Marshal(payload) //nolint:errcheck // map[string]string never fails to marshal
	return b
}

// Reschedule's lock ordering matches internal/assignment's own,
// established convention exactly: the agent row (if any) is locked
// first, then TransitionTx locks the order row second. Every M09
// assignment code path already acquires locks in that same sequence;
// reusing it here — rather than inventing a different order — is what
// actually rules out a cross-module deadlock between a concurrent M09
// assignment attempt and this M10 reschedule attempt, should they ever
// contend for the same agent row.
func (r *PostgresRepository) Reschedule(ctx context.Context, input RescheduleInput) (orders.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return orders.Order{}, fmt.Errorf("begin reschedule transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	if input.PreviousAgentID != nil {
		if err := freeAgent(ctx, tx, *input.PreviousAgentID); err != nil {
			return orders.Order{}, err
		}
	}

	metadata := rescheduleMetadata(input.RequestedDate, input.Reason)
	if _, err := r.trackingRepo.TransitionTx(ctx, tx, input.OrderID, input.ActorID, users.RoleAdmin, tracking.StatusRescheduled, metadata); err != nil {
		return orders.Order{}, mapTrackingError(err)
	}

	if err := insertRescheduleRequest(ctx, tx, input); err != nil {
		return orders.Order{}, err
	}

	updated, err := loadOrderTx(ctx, tx, input.OrderID)
	if err != nil {
		return orders.Order{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return orders.Order{}, fmt.Errorf("commit reschedule: %w", err)
	}
	return updated, nil
}

func (r *PostgresRepository) ListReschedules(ctx context.Context, orderID string) ([]Reschedule, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, order_id, requested_by, requested_date, reason, created_at
		 FROM reschedule_requests WHERE order_id = $1 ORDER BY created_at ASC`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("list reschedules: %w", err)
	}
	defer rows.Close()

	var out []Reschedule
	for rows.Next() {
		var re Reschedule
		if err := rows.Scan(&re.ID, &re.OrderID, &re.RequestedBy, &re.RequestedDate, &re.Reason, &re.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan reschedule: %w", err)
		}
		out = append(out, re)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list reschedules: %w", err)
	}
	return out, nil
}

// freeAgent takes SELECT ... FOR UPDATE on one delivery_agents row and
// sets it back to AVAILABLE — a minimal, direct SQL write against a
// table this package doesn't own the migration for, the exact same
// pattern internal/assignment's own lockCandidate/writeAssignment
// already established against the identical table, rather than growing
// agents.Repository's interface for one caller-transaction-scoped
// method. Deliberately does not touch active, current_zone_id,
// current_lat, or current_lng — only availability, per the finalized
// M10 decision. If the referenced agent row is somehow missing (cannot
// happen in practice: delivery_agents rows are never deleted, and
// orders.assigned_agent_id is an FK to it), freeing is skipped rather
// than failing the whole reschedule — releasing agent capacity is a
// secondary side effect of this operation, not its primary purpose.
func freeAgent(ctx context.Context, tx pgx.Tx, agentID string) error {
	var lockedID string
	err := tx.QueryRow(ctx, `SELECT id FROM delivery_agents WHERE id = $1 FOR UPDATE`, agentID).Scan(&lockedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("lock previously assigned agent: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE delivery_agents SET availability = $1 WHERE id = $2`, agents.AvailabilityAvailable, lockedID); err != nil {
		return fmt.Errorf("free previously assigned agent: %w", err)
	}
	return nil
}

func insertRescheduleRequest(ctx context.Context, tx pgx.Tx, input RescheduleInput) error {
	const stmt = `
		INSERT INTO reschedule_requests (order_id, requested_by, requested_date, reason)
		VALUES ($1, $2, $3, $4)`
	if _, err := tx.Exec(ctx, stmt, input.OrderID, input.ActorID, input.RequestedDate, input.Reason); err != nil {
		return fmt.Errorf("insert reschedule request: %w", err)
	}
	return nil
}

// loadOrderTx re-reads the full order row inside the reschedule
// transaction so the returned response reflects exactly what was just
// committed (status RESCHEDULED, unchanged assigned_agent_id, everything
// else) — the same "re-select after write" precedent
// internal/assignment.loadOrderTx already set.
func loadOrderTx(ctx context.Context, tx pgx.Tx, orderID string) (orders.Order, error) {
	row := tx.QueryRow(ctx, `SELECT `+orders.Columns+` FROM orders WHERE id = $1`, orderID)
	return orders.ScanOrder(row)
}

// mapTrackingError translates tracking's own sentinel errors into this
// package's — ErrOrderNotFound stays a not-found; ErrInvalidTransition
// (the order isn't currently FAILED) stays a conflict. Every other
// tracking error (including ErrForbiddenTransition, which should be
// unreachable here since this package always passes users.RoleAdmin —
// already authorized for FAILED->RESCHEDULED in M08's own, unmodified
// matrix — regardless of the real caller's role) passes through
// unmapped, for the handler's default 500 case.
func mapTrackingError(err error) error {
	switch {
	case errors.Is(err, tracking.ErrOrderNotFound):
		return ErrOrderNotFound
	case errors.Is(err, tracking.ErrInvalidTransition):
		return ErrInvalidTransition
	default:
		return err
	}
}
