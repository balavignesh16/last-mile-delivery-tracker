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

// Repository is the storage interface order handlers depend on. An
// interface — rather than the concrete *PostgresRepository — so handler
// unit tests can inject an in-memory fake, the same pattern every prior
// module in this project uses.
type Repository interface {
	// CreateOrder persists input.Quote exactly as given — see
	// CreateOrderInput's doc. A single INSERT: no transaction is needed
	// because there is no second table to keep in sync (no packages
	// table — see docs/order-management.md) and CalculateQuote has
	// already completed by the time this is called, so nothing here
	// re-reads or re-derives pricing.
	CreateOrder(ctx context.Context, input CreateOrderInput) (Order, error)
	// ListOrdersForCustomer returns one customer's own orders, newest
	// first — the CUSTOMER-facing view of GET /orders.
	ListOrdersForCustomer(ctx context.Context, customerID string) ([]Order, error)
	// ListAllOrders returns every order, newest first — the ADMIN-facing
	// view of GET /orders. No filtering: M07 explicitly does not add
	// status/zone/agent query parameters (see docs/order-management.md).
	ListAllOrders(ctx context.Context) ([]Order, error)
	FindOrderByID(ctx context.Context, id string) (Order, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const orderColumns = `
	id, customer_id, created_by, order_type, payment_type,
	pickup_area_id, drop_area_id, pickup_zone_id, drop_zone_id, zone_relationship,
	length_cm::float8, breadth_cm::float8, height_cm::float8, actual_weight_kg::float8,
	volumetric_weight_kg::float8, chargeable_weight_kg::float8,
	rate_card_id, base_rate::float8, cod_surcharge::float8, final_amount::float8,
	status, created_at`

// CreateOrder writes every pricing-derived column directly from
// input.Quote (a rates.QuoteResult) — this is the only place an orders
// row is ever written, and it never independently computes any of
// zone_relationship, volumetric_weight_kg, chargeable_weight_kg,
// rate_card_id, base_rate, cod_surcharge, or final_amount. status is
// always StatusCreated; this module writes no other value.
func (r *PostgresRepository) CreateOrder(ctx context.Context, input CreateOrderInput) (Order, error) {
	q := input.Quote
	const stmt = `
		INSERT INTO orders (
			customer_id, created_by, order_type, payment_type,
			pickup_area_id, drop_area_id, pickup_zone_id, drop_zone_id, zone_relationship,
			length_cm, breadth_cm, height_cm, actual_weight_kg,
			volumetric_weight_kg, chargeable_weight_kg,
			rate_card_id, base_rate, cod_surcharge, final_amount, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		RETURNING ` + orderColumns

	o, err := scanOrder(r.pool.QueryRow(ctx, stmt,
		input.CustomerID, input.CreatedBy, q.OrderType, q.PaymentType,
		q.PickupArea.ID, q.DropArea.ID, q.PickupZone.ID, q.DropZone.ID, q.ZoneRelationship,
		q.LengthCM, q.BreadthCM, q.HeightCM, q.ActualWeightKG,
		q.VolumetricWeightKG, q.ChargeableWeightKG,
		q.RateCard.ID, q.BaseRate, q.CODSurcharge, q.FinalAmount, StatusCreated,
	))
	if err != nil {
		return Order{}, fmt.Errorf("create order: %w", err)
	}
	return o, nil
}

func (r *PostgresRepository) ListOrdersForCustomer(ctx context.Context, customerID string) ([]Order, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+orderColumns+` FROM orders WHERE customer_id = $1 ORDER BY created_at DESC`, customerID)
	if err != nil {
		return nil, fmt.Errorf("list orders for customer: %w", err)
	}
	defer rows.Close()
	return scanOrders(rows)
}

func (r *PostgresRepository) ListAllOrders(ctx context.Context) ([]Order, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+orderColumns+` FROM orders ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list all orders: %w", err)
	}
	defer rows.Close()
	return scanOrders(rows)
}

func (r *PostgresRepository) FindOrderByID(ctx context.Context, id string) (Order, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id = $1`, id)
	o, err := scanOrder(row)
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

func scanOrder(row rowScanner) (Order, error) {
	var o Order
	err := row.Scan(
		&o.ID, &o.CustomerID, &o.CreatedBy, &o.OrderType, &o.PaymentType,
		&o.PickupAreaID, &o.DropAreaID, &o.PickupZoneID, &o.DropZoneID, &o.ZoneRelationship,
		&o.LengthCM, &o.BreadthCM, &o.HeightCM, &o.ActualWeightKG,
		&o.VolumetricWeightKG, &o.ChargeableWeightKG,
		&o.RateCardID, &o.BaseRate, &o.CODSurcharge, &o.FinalAmount,
		&o.Status, &o.CreatedAt,
	)
	return o, err
}

func scanOrders(rows pgx.Rows) ([]Order, error) {
	var out []Order
	for rows.Next() {
		o, err := scanOrder(rows)
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
