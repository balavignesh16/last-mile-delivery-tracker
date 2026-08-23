package orders

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrOrderNotFound is returned when no orders row matches the given id.
var ErrOrderNotFound = errors.New("order not found")

// OrderFilter narrows ListAllOrders to the ADMIN-facing status/zone/agent
// filters M12 adds (see docs/dashboards.md). A zero-value OrderFilter
// (every field "") matches every order — the exact same result
// ListAllOrders always returned before M12 — so the unfiltered
// GET /orders behavior every prior milestone relies on is provably
// unchanged. ZoneID matches an order whose pickup OR drop zone is the
// given zone (an admin filtering "orders touching this zone" cares
// about either side, not just pickup).
type OrderFilter struct {
	Status  string
	ZoneID  string
	AgentID string
}

// Repository is the storage interface order handlers depend on. An
// interface — rather than the concrete *PostgresRepository — so handler
// unit tests can inject an in-memory fake, the same pattern every prior
// module in this project uses.
type Repository interface {
	// CreateOrder persists input.Quote exactly as given — see
	// CreateOrderInput's doc — and, in the same transaction, records the
	// order's first tracking event (NULL -> CREATED, actor =
	// input.CreatedBy). M08 added this second write: the blueprint's own
	// tracking example opens an order's timeline with a CREATED entry,
	// so order creation has to produce one itself rather than starting
	// an order with an empty history. Every *later* transition is
	// M08's own job (internal/tracking's Transition), not this method's
	// — this is the one, single exception where internal/orders writes
	// to order_tracking_events directly, for the one event that only
	// order creation itself can know about.
	CreateOrder(ctx context.Context, input CreateOrderInput) (Order, error)
	// ListOrdersForCustomer returns one customer's own orders, newest
	// first — the CUSTOMER-facing view of GET /orders.
	ListOrdersForCustomer(ctx context.Context, customerID string) ([]Order, error)
	// ListAllOrders returns every order, newest first — the ADMIN-facing
	// view of GET /orders — narrowed by filter (M12; see OrderFilter's own
	// doc comment for why a zero-value filter is behavior-identical to
	// the pre-M12, unfiltered call every prior milestone relied on).
	ListAllOrders(ctx context.Context, filter OrderFilter) ([]Order, error)
	// ListOrdersForAgent returns only the orders currently assigned to
	// one delivery agent, newest first — the DELIVERY_AGENT-facing view
	// of GET /orders (M09). Added alongside ListOrdersForCustomer/
	// ListAllOrders rather than a single parameterized method, matching
	// this project's existing "one method per caller-role view" style.
	ListOrdersForAgent(ctx context.Context, agentID string) ([]Order, error)
	FindOrderByID(ctx context.Context, id string) (Order, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Columns and ScanOrder are exported (M09) so internal/assignment can
// re-select and scan a full order row inside its own transaction after
// writing an assignment, without a second, drifting copy of this
// column list — the same reuse precedent internal/rates set for
// ValidateQuoteFields/MapQuoteError (M06).
const Columns = `
	id, customer_id, created_by, order_type, payment_type,
	pickup_area_id, drop_area_id, pickup_zone_id, drop_zone_id, zone_relationship,
	length_cm::float8, breadth_cm::float8, height_cm::float8, actual_weight_kg::float8,
	volumetric_weight_kg::float8, chargeable_weight_kg::float8,
	rate_card_id, base_rate::float8, cod_surcharge::float8, final_amount::float8,
	assigned_agent_id, status, created_at`

// CreateOrder writes every pricing-derived column directly from
// input.Quote (a rates.QuoteResult) — it never independently computes
// any of zone_relationship, volumetric_weight_kg, chargeable_weight_kg,
// rate_card_id, base_rate, cod_surcharge, or final_amount. status is
// always StatusCreated; this method writes no other status value.
//
// Runs inside a transaction (added for M08): the INSERT into orders and
// the paired INSERT into order_tracking_events (previous_status = NULL,
// new_status = 'CREATED', actor_id = input.CreatedBy) must succeed or
// fail together — an order can never exist without its own opening
// tracking event, and no tracking event can reference an order that
// doesn't exist. This is the one place internal/orders writes to
// order_tracking_events directly; every later transition belongs to
// internal/tracking (M08), never here.
func (r *PostgresRepository) CreateOrder(ctx context.Context, input CreateOrderInput) (Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin order creation transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	q := input.Quote
	const stmt = `
		INSERT INTO orders (
			customer_id, created_by, order_type, payment_type,
			pickup_area_id, drop_area_id, pickup_zone_id, drop_zone_id, zone_relationship,
			length_cm, breadth_cm, height_cm, actual_weight_kg,
			volumetric_weight_kg, chargeable_weight_kg,
			rate_card_id, base_rate, cod_surcharge, final_amount, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		RETURNING ` + Columns

	o, err := ScanOrder(tx.QueryRow(ctx, stmt,
		input.CustomerID, input.CreatedBy, q.OrderType, q.PaymentType,
		q.PickupArea.ID, q.DropArea.ID, q.PickupZone.ID, q.DropZone.ID, q.ZoneRelationship,
		q.LengthCM, q.BreadthCM, q.HeightCM, q.ActualWeightKG,
		q.VolumetricWeightKG, q.ChargeableWeightKG,
		q.RateCard.ID, q.BaseRate, q.CODSurcharge, q.FinalAmount, StatusCreated,
	))
	if err != nil {
		return Order{}, fmt.Errorf("create order: %w", err)
	}

	const eventStmt = `
		INSERT INTO order_tracking_events (order_id, previous_status, new_status, actor_id, actor_role, metadata)
		VALUES ($1, NULL, $2, $3, $4, NULL)`
	if _, err := tx.Exec(ctx, eventStmt, o.ID, StatusCreated, input.CreatedBy, string(input.CreatedByRole)); err != nil {
		return Order{}, fmt.Errorf("record initial tracking event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit order creation: %w", err)
	}
	return o, nil
}

func (r *PostgresRepository) ListOrdersForCustomer(ctx context.Context, customerID string) ([]Order, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+Columns+` FROM orders WHERE customer_id = $1 ORDER BY created_at DESC`, customerID)
	if err != nil {
		return nil, fmt.Errorf("list orders for customer: %w", err)
	}
	defer rows.Close()
	return scanOrders(rows)
}

// ListAllOrders builds its WHERE clause dynamically from filter's
// non-empty fields — no query-builder dependency, just positional
// placeholders appended one at a time, the same plain-SQL style every
// other repository in this project already uses. An empty filter field
// contributes no clause at all, so a zero-value OrderFilter produces the
// exact unfiltered query this method always ran before M12.
func (r *PostgresRepository) ListAllOrders(ctx context.Context, filter OrderFilter) ([]Order, error) {
	query := `SELECT ` + Columns + ` FROM orders WHERE 1=1`
	var args []any

	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if filter.ZoneID != "" {
		args = append(args, filter.ZoneID)
		query += fmt.Sprintf(" AND (pickup_zone_id = $%d OR drop_zone_id = $%d)", len(args), len(args))
	}
	if filter.AgentID != "" {
		args = append(args, filter.AgentID)
		query += fmt.Sprintf(" AND assigned_agent_id = $%d", len(args))
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list all orders: %w", err)
	}
	defer rows.Close()
	return scanOrders(rows)
}

func (r *PostgresRepository) ListOrdersForAgent(ctx context.Context, agentID string) ([]Order, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+Columns+` FROM orders WHERE assigned_agent_id = $1 ORDER BY created_at DESC`, agentID)
	if err != nil {
		return nil, fmt.Errorf("list orders for agent: %w", err)
	}
	defer rows.Close()
	return scanOrders(rows)
}

func (r *PostgresRepository) FindOrderByID(ctx context.Context, id string) (Order, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+Columns+` FROM orders WHERE id = $1`, id)
	o, err := ScanOrder(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Order{}, ErrOrderNotFound
		}
		return Order{}, fmt.Errorf("find order: %w", err)
	}
	return o, nil
}

// rowScanner abstracts over pgx.Row (QueryRow) and pgx.Rows (Query's
// iteration) so single-row and list queries can share one scan function
// — same pattern as internal/rates and internal/zones.
type rowScanner interface {
	Scan(dest ...any) error
}

func ScanOrder(row rowScanner) (Order, error) {
	var o Order
	err := row.Scan(
		&o.ID, &o.CustomerID, &o.CreatedBy, &o.OrderType, &o.PaymentType,
		&o.PickupAreaID, &o.DropAreaID, &o.PickupZoneID, &o.DropZoneID, &o.ZoneRelationship,
		&o.LengthCM, &o.BreadthCM, &o.HeightCM, &o.ActualWeightKG,
		&o.VolumetricWeightKG, &o.ChargeableWeightKG,
		&o.RateCardID, &o.BaseRate, &o.CODSurcharge, &o.FinalAmount,
		&o.AssignedAgentID, &o.Status, &o.CreatedAt,
	)
	return o, err
}

func scanOrders(rows pgx.Rows) ([]Order, error) {
	var out []Order
	for rows.Next() {
		o, err := ScanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	return out, nil
}
