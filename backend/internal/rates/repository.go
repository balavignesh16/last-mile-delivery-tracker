package rates

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresUniqueViolation is a PostgreSQL SQLSTATE. See
// https://www.postgresql.org/docs/current/errcodes-appendix.html
//
// There is no foreign-key-violation mapping here, unlike
// internal/agents/internal/zones: every write path that could violate
// rate_card_slabs' FK to rate_cards goes through lockRateCard first,
// which already turns an unknown rate card into ErrRateCardNotFound
// before any INSERT/UPDATE is attempted.
const postgresUniqueViolation = "23505"

// Constraint names referenced by name so error-mapping doesn't rely on
// SQLSTATE alone — rate_cards/rate_card_slabs each have more than one
// unique index, and a name match is what keeps the mapping precise if a
// third one is ever added.
const (
	activeCombinationIndexName = "idx_rate_cards_one_active_per_combination"
	openEndedSlabIndexName     = "idx_rate_card_slabs_one_open_ended"
)

var (
	ErrRateCardNotFound = errors.New("rate card not found")
	ErrSlabNotFound     = errors.New("slab not found")
	// ErrActiveCombinationExists is returned by UpdateRateCard when
	// activating a card would leave two active cards for the same
	// (order_type, zone_relationship) — caught by the database's
	// partial unique index, not just a pre-check, so it is race-safe
	// under concurrent activation attempts (see repository_test-level
	// concurrency coverage in tests/integration).
	ErrActiveCombinationExists = errors.New("another rate card is already active for this order type and zone relationship")
)

// Repository is the storage interface rate/slab handlers depend on. An
// interface — rather than the concrete *PostgresRepository — so handler
// unit tests can inject an in-memory fake, the same pattern every prior
// module in this project uses.
type Repository interface {
	CreateRateCard(ctx context.Context, input CreateRateCardInput) (RateCard, error)
	ListRateCards(ctx context.Context) ([]RateCard, error)
	FindRateCardByID(ctx context.Context, id string) (RateCard, error)
	UpdateRateCard(ctx context.Context, id string, update RateCardUpdate) (RateCard, error)
	// FindActiveCard is the consumption point M06's calculation engine
	// is expected to call directly (Go-level, not HTTP — see
	// docs/rate-configuration.md for why there is no public "resolve"
	// endpoint, the same reasoning M04 used for zone resolution).
	FindActiveCard(ctx context.Context, orderType OrderType, zoneRelationship ZoneRelationship) (RateCard, error)

	// CreateSlab's rateCardID comes from the URL path, not the request
	// body — see CreateSlabInput's doc.
	CreateSlab(ctx context.Context, rateCardID string, input CreateSlabInput) (Slab, error)
	ListSlabsByRateCard(ctx context.Context, rateCardID string) ([]Slab, error)
	FindSlabByID(ctx context.Context, id string) (Slab, error)
	UpdateSlab(ctx context.Context, slabID string, update SlabUpdate) (Slab, error)
	DeleteSlab(ctx context.Context, slabID string) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const rateCardColumns = `id, order_type, zone_relationship, cod_surcharge::float8, active, created_at`

// CreateRateCard always inserts active=false, regardless of anything on
// CreateRateCardInput (which has no Active field at all — see its doc).
// An admin activates a card as a separate, deliberate UpdateRateCard
// call. This means CreateRateCard itself can never conflict with
// idx_rate_cards_one_active_per_combination — only activation can, and
// that path is handled in UpdateRateCard.
func (r *PostgresRepository) CreateRateCard(ctx context.Context, input CreateRateCardInput) (RateCard, error) {
	const stmt = `
		INSERT INTO rate_cards (order_type, zone_relationship, cod_surcharge, active)
		VALUES ($1, $2, $3, false)
		RETURNING ` + rateCardColumns
	rc, err := scanRateCard(r.pool.QueryRow(ctx, stmt, input.OrderType, input.ZoneRelationship, input.CODSurcharge))
	if err != nil {
		return RateCard{}, fmt.Errorf("create rate card: %w", err)
	}
	return rc, nil
}

func (r *PostgresRepository) ListRateCards(ctx context.Context) ([]RateCard, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+rateCardColumns+` FROM rate_cards ORDER BY order_type, zone_relationship, created_at`)
	if err != nil {
		return nil, fmt.Errorf("list rate cards: %w", err)
	}
	defer rows.Close()

	var out []RateCard
	for rows.Next() {
		rc, err := scanRateCard(rows)
		if err != nil {
			return nil, fmt.Errorf("scan rate card: %w", err)
		}
		out = append(out, rc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list rate cards: %w", err)
	}
	return out, nil
}

func (r *PostgresRepository) FindRateCardByID(ctx context.Context, id string) (RateCard, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+rateCardColumns+` FROM rate_cards WHERE id = $1`, id)
	rc, err := scanRateCard(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RateCard{}, ErrRateCardNotFound
		}
		return RateCard{}, fmt.Errorf("find rate card: %w", err)
	}
	return rc, nil
}

func (r *PostgresRepository) FindActiveCard(ctx context.Context, orderType OrderType, zoneRelationship ZoneRelationship) (RateCard, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+rateCardColumns+` FROM rate_cards WHERE order_type = $1 AND zone_relationship = $2 AND active`,
		orderType, zoneRelationship)
	rc, err := scanRateCard(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RateCard{}, ErrRateCardNotFound
		}
		return RateCard{}, fmt.Errorf("find active rate card: %w", err)
	}
	return rc, nil
}

// UpdateRateCard always writes cod_surcharge; active is only overwritten
// when the caller provided one (Active != nil) — COALESCE($2, active)
// is the SQL side of RateCardUpdate.Active's "nil means unchanged"
// contract, the same pattern zones.UpdateZone uses.
//
// Activating a card (Active pointing at true) that would collide with
// idx_rate_cards_one_active_per_combination fails atomically at the
// database — there is no separate pre-check that could race against a
// concurrent activation of a different card for the same combination;
// Postgres's own unique-index enforcement is what makes exactly one
// concurrent winner deterministic.
func (r *PostgresRepository) UpdateRateCard(ctx context.Context, id string, update RateCardUpdate) (RateCard, error) {
	const stmt = `
		UPDATE rate_cards SET cod_surcharge = $1, active = COALESCE($2, active)
		WHERE id = $3
		RETURNING ` + rateCardColumns
	rc, err := scanRateCard(r.pool.QueryRow(ctx, stmt, update.CODSurcharge, update.Active, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RateCard{}, ErrRateCardNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation && pgErr.ConstraintName == activeCombinationIndexName {
			return RateCard{}, ErrActiveCombinationExists
		}
		return RateCard{}, fmt.Errorf("update rate card: %w", err)
	}
	return rc, nil
}

const slabColumns = `id, rate_card_id, min_weight::float8, max_weight::float8, price::float8, created_at`

// CreateSlab locks the parent rate_cards row (SELECT ... FOR UPDATE)
// before reading the card's existing slabs and validating the new one
// against them, all inside one transaction. This is what actually
// prevents the check-then-insert race a plain "SELECT existing, check
// in Go, INSERT" sequence would leave open: two concurrent requests
// creating overlapping slabs under the same rate card would otherwise
// both pass the overlap check (each reading the same pre-insert state)
// before either commits. The lock serializes slab writes per rate card
// — concurrent writes to *different* rate cards are unaffected.
func (r *PostgresRepository) CreateSlab(ctx context.Context, rateCardID string, input CreateSlabInput) (Slab, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Slab{}, fmt.Errorf("begin slab creation transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	if err := lockRateCard(ctx, tx, rateCardID); err != nil {
		return Slab{}, err
	}

	existing, err := listSlabs(ctx, tx, rateCardID)
	if err != nil {
		return Slab{}, err
	}

	if err := validateSlabAgainstExisting(input.MinWeight, input.MaxWeight, "", existing); err != nil {
		return Slab{}, err
	}

	const stmt = `
		INSERT INTO rate_card_slabs (rate_card_id, min_weight, max_weight, price)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + slabColumns
	slab, err := scanSlab(tx.QueryRow(ctx, stmt, rateCardID, input.MinWeight, input.MaxWeight, input.Price))
	if err != nil {
		if mapped := mapSlabWriteError(err); mapped != nil {
			return Slab{}, mapped
		}
		return Slab{}, fmt.Errorf("create slab: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Slab{}, fmt.Errorf("commit slab creation: %w", err)
	}
	return slab, nil
}

func (r *PostgresRepository) ListSlabsByRateCard(ctx context.Context, rateCardID string) ([]Slab, error) {
	return listSlabs(ctx, r.pool, rateCardID)
}

func (r *PostgresRepository) FindSlabByID(ctx context.Context, id string) (Slab, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+slabColumns+` FROM rate_card_slabs WHERE id = $1`, id)
	s, err := scanSlab(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Slab{}, ErrSlabNotFound
		}
		return Slab{}, fmt.Errorf("find slab: %w", err)
	}
	return s, nil
}

// UpdateSlab takes the same per-rate-card lock CreateSlab does, for the
// same reason: re-validating an edited slab's range against its
// siblings must see a consistent, locked snapshot, not one that could
// change out from under it before the UPDATE commits.
func (r *PostgresRepository) UpdateSlab(ctx context.Context, slabID string, update SlabUpdate) (Slab, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Slab{}, fmt.Errorf("begin slab update transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	current, err := findSlab(ctx, tx, slabID)
	if err != nil {
		return Slab{}, err
	}

	if err := lockRateCard(ctx, tx, current.RateCardID); err != nil {
		return Slab{}, err
	}

	existing, err := listSlabs(ctx, tx, current.RateCardID)
	if err != nil {
		return Slab{}, err
	}

	if err := validateSlabAgainstExisting(update.MinWeight, update.MaxWeight, slabID, existing); err != nil {
		return Slab{}, err
	}

	const stmt = `
		UPDATE rate_card_slabs SET min_weight = $1, max_weight = $2, price = $3
		WHERE id = $4
		RETURNING ` + slabColumns
	updated, err := scanSlab(tx.QueryRow(ctx, stmt, update.MinWeight, update.MaxWeight, update.Price, slabID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Slab{}, ErrSlabNotFound
		}
		if mapped := mapSlabWriteError(err); mapped != nil {
			return Slab{}, mapped
		}
		return Slab{}, fmt.Errorf("update slab: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Slab{}, fmt.Errorf("commit slab update: %w", err)
	}
	return updated, nil
}

func (r *PostgresRepository) DeleteSlab(ctx context.Context, slabID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM rate_card_slabs WHERE id = $1`, slabID)
	if err != nil {
		return fmt.Errorf("delete slab: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSlabNotFound
	}
	return nil
}

// mapSlabWriteError maps the one Postgres constraint violation that can
// still reach an INSERT/UPDATE despite the application-level check
// above (idx_rate_card_slabs_one_open_ended, defense-in-depth per
// slab_validation.go's doc) to the matching domain error. Any other
// constraint violation is left unmapped (returned nil) for the caller
// to wrap generically — this project prefers an honest 500 over a
// mis-mapped domain error.
func mapSlabWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation && pgErr.ConstraintName == openEndedSlabIndexName {
		return ErrMultipleOpenEndedSlabs
	}
	return nil
}

// querier abstracts over the one method both *pgxpool.Pool and pgx.Tx
// implement identically — the same pattern internal/users and
// internal/agents use to share logic between pool-direct and
// transaction-scoped callers.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// lockRateCard verifies the rate card exists and takes a row lock on it
// for the remainder of the caller's transaction, serializing concurrent
// slab writes against the same rate card. Must be called inside a
// transaction — the lock is released on commit/rollback.
func lockRateCard(ctx context.Context, tx pgx.Tx, rateCardID string) error {
	var exists string
	err := tx.QueryRow(ctx, `SELECT id FROM rate_cards WHERE id = $1 FOR UPDATE`, rateCardID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRateCardNotFound
		}
		return fmt.Errorf("lock rate card: %w", err)
	}
	return nil
}

func listSlabs(ctx context.Context, q querier, rateCardID string) ([]Slab, error) {
	rows, err := q.Query(ctx, `SELECT `+slabColumns+` FROM rate_card_slabs WHERE rate_card_id = $1 ORDER BY min_weight`, rateCardID)
	if err != nil {
		return nil, fmt.Errorf("list slabs: %w", err)
	}
	defer rows.Close()

	var out []Slab
	for rows.Next() {
		s, err := scanSlab(rows)
		if err != nil {
			return nil, fmt.Errorf("scan slab: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list slabs: %w", err)
	}
	return out, nil
}

func findSlab(ctx context.Context, q querier, id string) (Slab, error) {
	row := q.QueryRow(ctx, `SELECT `+slabColumns+` FROM rate_card_slabs WHERE id = $1`, id)
	s, err := scanSlab(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Slab{}, ErrSlabNotFound
		}
		return Slab{}, fmt.Errorf("find slab: %w", err)
	}
	return s, nil
}

// rowScanner abstracts over pgx.Row (QueryRow) and pgx.Rows (Query's
// iteration) so list and find helpers can share one scan function each
// — same pattern as internal/agents and internal/zones.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRateCard(row rowScanner) (RateCard, error) {
	var rc RateCard
	err := row.Scan(&rc.ID, &rc.OrderType, &rc.ZoneRelationship, &rc.CODSurcharge, &rc.Active, &rc.CreatedAt)
	return rc, err
}

func scanSlab(row rowScanner) (Slab, error) {
	var s Slab
	err := row.Scan(&s.ID, &s.RateCardID, &s.MinWeight, &s.MaxWeight, &s.Price, &s.CreatedAt)
	return s, err
}
