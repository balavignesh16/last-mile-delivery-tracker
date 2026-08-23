package assignment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lastmiletracker/internal/agents"
	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/tracking"
	"lastmiletracker/internal/users"
)

var (
	// ErrOrderNotFound mirrors orders.ErrOrderNotFound/tracking.ErrOrderNotFound
	// — this package defines its own so callers never need to import
	// two other packages' sentinel errors just to handle "order not
	// found" once.
	ErrOrderNotFound = errors.New("order not found")
	// ErrAgentNotFound is returned when the supplied agent_id does not
	// reference a real delivery_agents row.
	ErrAgentNotFound = errors.New("delivery agent not found")
	// ErrAgentNotEligible is returned when the targeted agent exists but
	// fails the eligibility rule (see candidate.go's IsEligible) —
	// inactive, not AVAILABLE, or has no usable zone. Applies equally to
	// a manually-targeted agent and to auto-assignment's own candidate
	// pool, since both call IsEligible.
	ErrAgentNotEligible = errors.New("delivery agent is not eligible for assignment")
)

// Repository is the storage interface the assignment handlers depend
// on. An interface — rather than the concrete *PostgresRepository — so
// handler unit tests can inject an in-memory fake, the same pattern
// every prior module in this project uses. Both methods return the
// updated order (the finalized M09 response shape) or one of this
// package's/tracking's sentinel errors.
type Repository interface {
	Assign(ctx context.Context, orderID, agentID, actorID string, actorRole users.Role) (orders.Order, error)
	AutoAssign(ctx context.Context, orderID, actorID string, actorRole users.Role) (orders.Order, error)
}

type PostgresRepository struct {
	pool         *pgxpool.Pool
	agentsRepo   agents.Repository
	ordersRepo   orders.Repository
	trackingRepo tracking.Repository
	// onTransition (M11) is nil-safe — see tracking.TransitionHook's own
	// doc comment. Wired in by main.go; this package has no dependency
	// on internal/notifications at all.
	onTransition tracking.TransitionHook
}

func NewPostgresRepository(pool *pgxpool.Pool, agentsRepo agents.Repository, ordersRepo orders.Repository, trackingRepo tracking.Repository, onTransition tracking.TransitionHook) *PostgresRepository {
	return &PostgresRepository{pool: pool, agentsRepo: agentsRepo, ordersRepo: ordersRepo, trackingRepo: trackingRepo, onTransition: onTransition}
}

// assignmentMetadata builds the {"assigned_agent_id": "..."} payload
// every ASSIGNED tracking event carries — this is what makes
// order_tracking_events double as the assignment history without a
// separate table (see docs/assignment-engine.md).
func assignmentMetadata(agentID string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"assigned_agent_id": agentID}) //nolint:errcheck // map[string]string never fails to marshal
	return b
}

// Assign performs manual assignment: lock the named agent, re-check
// eligibility under that lock, transition the order to ASSIGNED via
// tracking.TransitionTx (M08's own authoritative, unmodified
// state-machine write — never reimplemented here), write
// orders.assigned_agent_id, and mark the agent BUSY — all inside one
// transaction. Any failure rolls the entire operation back; nothing is
// left half-assigned.
//
// Lock ordering: the agent row is locked first, then TransitionTx locks
// the order row. Every assignment code path (this one and AutoAssign)
// acquires locks in that exact same order, which is what actually
// prevents a cross-transaction deadlock — not that locking exists, but
// that it is always acquired in the same sequence.
func (r *PostgresRepository) Assign(ctx context.Context, orderID, agentID, actorID string, actorRole users.Role) (orders.Order, error) {
	if _, err := r.ordersRepo.FindOrderByID(ctx, orderID); err != nil {
		if errors.Is(err, orders.ErrOrderNotFound) {
			return orders.Order{}, ErrOrderNotFound
		}
		return orders.Order{}, fmt.Errorf("find order: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return orders.Order{}, fmt.Errorf("begin assignment transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	candidate, err := lockCandidate(ctx, tx, agentID)
	if err != nil {
		return orders.Order{}, err
	}
	if !IsEligible(candidate) {
		return orders.Order{}, ErrAgentNotEligible
	}

	event, err := r.trackingRepo.TransitionTx(ctx, tx, orderID, actorID, actorRole, actorRole, tracking.StatusAssigned, assignmentMetadata(agentID))
	if err != nil {
		return orders.Order{}, mapTrackingError(err)
	}

	if err := writeAssignment(ctx, tx, orderID, agentID); err != nil {
		return orders.Order{}, err
	}

	updated, err := loadOrderTx(ctx, tx, orderID)
	if err != nil {
		return orders.Order{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return orders.Order{}, fmt.Errorf("commit assignment: %w", err)
	}

	// Fires strictly after the transaction above has already committed
	// — see tracking.TransitionHook's doc comment.
	if r.onTransition != nil {
		r.onTransition(ctx, event)
	}

	return updated, nil
}

// AutoAssign performs automatic assignment: read the order (for its
// pickup zone and a fast, non-authoritative "is this even assignable"
// check — reusing tracking.IsValidTransition directly, never a
// reimplementation), fetch every agent, rank with SelectCandidate, then
// lock and re-check the top candidate under the lock. If a candidate
// was raced away (no longer eligible by the time it's locked), the next
// candidate is tried — never assigning an agent that failed its own
// eligibility re-check. Everything after the order read happens inside
// one transaction; the whole operation rolls back on any failure.
func (r *PostgresRepository) AutoAssign(ctx context.Context, orderID, actorID string, actorRole users.Role) (orders.Order, error) {
	order, err := r.ordersRepo.FindOrderByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, orders.ErrOrderNotFound) {
			return orders.Order{}, ErrOrderNotFound
		}
		return orders.Order{}, fmt.Errorf("find order: %w", err)
	}
	// Fast, non-authoritative fail-fast: reuses M08's own exported pure
	// function, never a parallel copy of the edge table. The real,
	// race-safe enforcement still happens inside TransitionTx below.
	if !tracking.IsValidTransition(tracking.Status(order.Status), tracking.StatusAssigned) {
		return orders.Order{}, tracking.ErrInvalidTransition
	}

	agentRows, err := r.agentsRepo.List(ctx)
	if err != nil {
		return orders.Order{}, fmt.Errorf("list agents: %w", err)
	}
	pool := make([]Candidate, len(agentRows))
	for i, a := range agentRows {
		pool[i] = toCandidate(a)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return orders.Order{}, fmt.Errorf("begin auto-assignment transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	remaining := pool
	for {
		winner, err := SelectCandidate(remaining, order.PickupZoneID)
		if err != nil {
			return orders.Order{}, err // ErrNoEligibleCandidate
		}

		locked, err := lockCandidate(ctx, tx, winner.AgentID)
		if err != nil {
			return orders.Order{}, err
		}
		if !IsEligible(locked) {
			// Raced away since the bulk read — drop it and retry with
			// whichever candidate ranks next, rather than assigning an
			// agent that just failed its own eligibility re-check.
			remaining = withoutAgent(remaining, winner.AgentID)
			continue
		}

		event, err := r.trackingRepo.TransitionTx(ctx, tx, orderID, actorID, actorRole, actorRole, tracking.StatusAssigned, assignmentMetadata(locked.AgentID))
		if err != nil {
			return orders.Order{}, mapTrackingError(err)
		}
		if err := writeAssignment(ctx, tx, orderID, locked.AgentID); err != nil {
			return orders.Order{}, err
		}
		updated, err := loadOrderTx(ctx, tx, orderID)
		if err != nil {
			return orders.Order{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return orders.Order{}, fmt.Errorf("commit auto-assignment: %w", err)
		}

		// Fires strictly after the transaction above has already
		// committed — see tracking.TransitionHook's doc comment.
		if r.onTransition != nil {
			r.onTransition(ctx, event)
		}

		return updated, nil
	}
}

func toCandidate(a agents.AgentWithUser) Candidate {
	return Candidate{
		AgentID:        a.ID,
		Active:         a.Active,
		Availability:   a.Availability,
		CurrentZoneID:  a.CurrentZoneID,
		LastAssignedAt: a.LastAssignedAt,
	}
}

func withoutAgent(pool []Candidate, agentID string) []Candidate {
	out := make([]Candidate, 0, len(pool))
	for _, c := range pool {
		if c.AgentID != agentID {
			out = append(out, c)
		}
	}
	return out
}

// lockCandidate takes SELECT ... FOR UPDATE on one delivery_agents row
// and returns just the fields IsEligible needs — a minimal, direct SQL
// read against a table this package doesn't own the migration for, the
// same pattern internal/tracking already established against orders
// (see its own repository.go doc comment) rather than growing
// agents.Repository's interface for one caller-transaction-scoped
// method.
func lockCandidate(ctx context.Context, tx pgx.Tx, agentID string) (Candidate, error) {
	var c Candidate
	err := tx.QueryRow(ctx,
		`SELECT id, active, availability, current_zone_id, last_assigned_at FROM delivery_agents WHERE id = $1 FOR UPDATE`,
		agentID,
	).Scan(&c.AgentID, &c.Active, &c.Availability, &c.CurrentZoneID, &c.LastAssignedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Candidate{}, ErrAgentNotFound
		}
		return Candidate{}, fmt.Errorf("lock agent: %w", err)
	}
	return c, nil
}

// writeAssignment sets orders.assigned_agent_id and marks the agent
// BUSY, recording last_assigned_at — the exact field M03's own
// AgentWithUser doc comment reserved for M09 to write ("M09 sets
// LastAssignedAt on assignment").
func writeAssignment(ctx context.Context, tx pgx.Tx, orderID, agentID string) error {
	if _, err := tx.Exec(ctx, `UPDATE orders SET assigned_agent_id = $1 WHERE id = $2`, agentID, orderID); err != nil {
		return fmt.Errorf("write assigned_agent_id: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE delivery_agents SET availability = $1, last_assigned_at = now() WHERE id = $2`,
		agents.AvailabilityBusy, agentID,
	); err != nil {
		return fmt.Errorf("mark agent busy: %w", err)
	}
	return nil
}

// loadOrderTx re-reads the full order row inside the assignment
// transaction so the returned response reflects exactly what was just
// committed (status, assigned_agent_id, everything) — the same
// "re-select after write" precedent internal/agents.Create and
// internal/rates use.
func loadOrderTx(ctx context.Context, tx pgx.Tx, orderID string) (orders.Order, error) {
	row := tx.QueryRow(ctx, `SELECT `+orders.Columns+` FROM orders WHERE id = $1`, orderID)
	return orders.ScanOrder(row)
}

// mapTrackingError passes tracking's own sentinel errors straight
// through (ErrOrderNotFound remapped to this package's own, for a
// single error type at the handler boundary) — never translated into a
// new, parallel meaning.
func mapTrackingError(err error) error {
	if errors.Is(err, tracking.ErrOrderNotFound) {
		return ErrOrderNotFound
	}
	return err
}
