package zones

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresUniqueViolation and postgresForeignKeyViolation are PostgreSQL
// SQLSTATEs. See https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	postgresUniqueViolation     = "23505"
	postgresForeignKeyViolation = "23503"
)

var (
	// ErrZoneNotFound is returned when no zones row matches the given id.
	ErrZoneNotFound = errors.New("zone not found")
	// ErrAreaNotFound is returned when no areas row matches the given id.
	ErrAreaNotFound = errors.New("area not found")
	// ErrZoneNameTaken is returned by CreateZone/UpdateZone when the
	// zones.name UNIQUE constraint fired.
	ErrZoneNameTaken = errors.New("a zone with this name already exists")
	// ErrAreaNameTaken is returned by CreateArea/UpdateArea when the
	// areas (zone_id, name) UNIQUE constraint fired.
	ErrAreaNameTaken = errors.New("an area with this name already exists in this zone")
)

// Repository is the storage interface zone/area handlers depend on. An
// interface — rather than the concrete *PostgresRepository — so handler
// unit tests can inject an in-memory fake instead of requiring a real
// database, the same pattern internal/users and internal/agents use.
type Repository interface {
	CreateZone(ctx context.Context, input CreateZoneInput) (Zone, error)
	ListZones(ctx context.Context) ([]Zone, error)
	FindZoneByID(ctx context.Context, id string) (Zone, error)
	UpdateZone(ctx context.Context, id string, update ZoneUpdate) (Zone, error)

	// CreateArea's zoneID comes from the URL path, not the request body
	// — see CreateAreaInput's doc.
	CreateArea(ctx context.Context, zoneID string, input CreateAreaInput) (Area, error)
	ListAreasByZone(ctx context.Context, zoneID string) ([]Area, error)
	FindAreaByID(ctx context.Context, id string) (Area, error)
	UpdateArea(ctx context.Context, areaID string, update AreaUpdate) (Area, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const zoneColumns = `id, name, active, created_at`

func (r *PostgresRepository) CreateZone(ctx context.Context, input CreateZoneInput) (Zone, error) {
	const stmt = `INSERT INTO zones (name) VALUES ($1) RETURNING ` + zoneColumns
	z, err := scanZone(r.pool.QueryRow(ctx, stmt, input.Name))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			return Zone{}, ErrZoneNameTaken
		}
		return Zone{}, fmt.Errorf("create zone: %w", err)
	}
	return z, nil
}

func (r *PostgresRepository) ListZones(ctx context.Context) ([]Zone, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+zoneColumns+` FROM zones ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()

	var out []Zone
	for rows.Next() {
		z, err := scanZone(rows)
		if err != nil {
			return nil, fmt.Errorf("scan zone: %w", err)
		}
		out = append(out, z)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	return out, nil
}

func (r *PostgresRepository) FindZoneByID(ctx context.Context, id string) (Zone, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+zoneColumns+` FROM zones WHERE id = $1`, id)
	z, err := scanZone(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Zone{}, ErrZoneNotFound
		}
		return Zone{}, fmt.Errorf("find zone: %w", err)
	}
	return z, nil
}

// UpdateZone always writes name; active is only overwritten when the
// caller provided one (Active != nil) — COALESCE($2, active) is the SQL
// side of ZoneUpdate.Active's "nil means unchanged" contract.
func (r *PostgresRepository) UpdateZone(ctx context.Context, id string, update ZoneUpdate) (Zone, error) {
	const stmt = `
		UPDATE zones SET name = $1, active = COALESCE($2, active)
		WHERE id = $3
		RETURNING ` + zoneColumns
	z, err := scanZone(r.pool.QueryRow(ctx, stmt, update.Name, update.Active, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Zone{}, ErrZoneNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			return Zone{}, ErrZoneNameTaken
		}
		return Zone{}, fmt.Errorf("update zone: %w", err)
	}
	return z, nil
}

const areaColumns = `id, name, zone_id, active, created_at`

// CreateArea maps a foreign-key violation to ErrZoneNotFound as
// defense-in-depth. CreateAreaHandler already checks the zone exists
// before calling this, so the FK should never actually fire in practice
// — but the mapping means this method is safe to call directly (e.g.
// from a future module or a test) without relying on that caller-side
// check.
func (r *PostgresRepository) CreateArea(ctx context.Context, zoneID string, input CreateAreaInput) (Area, error) {
	const stmt = `INSERT INTO areas (name, zone_id) VALUES ($1, $2) RETURNING ` + areaColumns
	a, err := scanArea(r.pool.QueryRow(ctx, stmt, input.Name, zoneID))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case postgresUniqueViolation:
				return Area{}, ErrAreaNameTaken
			case postgresForeignKeyViolation:
				return Area{}, ErrZoneNotFound
			}
		}
		return Area{}, fmt.Errorf("create area: %w", err)
	}
	return a, nil
}

func (r *PostgresRepository) ListAreasByZone(ctx context.Context, zoneID string) ([]Area, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+areaColumns+` FROM areas WHERE zone_id = $1 ORDER BY name`, zoneID)
	if err != nil {
		return nil, fmt.Errorf("list areas: %w", err)
	}
	defer rows.Close()

	var out []Area
	for rows.Next() {
		a, err := scanArea(rows)
		if err != nil {
			return nil, fmt.Errorf("scan area: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list areas: %w", err)
	}
	return out, nil
}

func (r *PostgresRepository) FindAreaByID(ctx context.Context, id string) (Area, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+areaColumns+` FROM areas WHERE id = $1`, id)
	a, err := scanArea(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Area{}, ErrAreaNotFound
		}
		return Area{}, fmt.Errorf("find area: %w", err)
	}
	return a, nil
}

// UpdateArea always writes name; active is only overwritten when the
// caller provided one (Active != nil) — same COALESCE contract as
// UpdateZone.
func (r *PostgresRepository) UpdateArea(ctx context.Context, areaID string, update AreaUpdate) (Area, error) {
	const stmt = `
		UPDATE areas SET name = $1, active = COALESCE($2, active)
		WHERE id = $3
		RETURNING ` + areaColumns
	a, err := scanArea(r.pool.QueryRow(ctx, stmt, update.Name, update.Active, areaID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Area{}, ErrAreaNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			return Area{}, ErrAreaNameTaken
		}
		return Area{}, fmt.Errorf("update area: %w", err)
	}
	return a, nil
}

// rowScanner abstracts over pgx.Row (QueryRow) and pgx.Rows (Query's
// iteration) so List and FindByID can share one scan function each —
// same pattern as internal/agents.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanZone(row rowScanner) (Zone, error) {
	var z Zone
	err := row.Scan(&z.ID, &z.Name, &z.Active, &z.CreatedAt)
	return z, err
}

func scanArea(row rowScanner) (Area, error) {
	var a Area
	err := row.Scan(&a.ID, &a.Name, &a.ZoneID, &a.Active, &a.CreatedAt)
	return a, err
}
