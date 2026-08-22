package agents

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"lastmiletracker/internal/users"
)

// postgresForeignKeyViolation is a PostgreSQL SQLSTATE. See
// https://www.postgresql.org/docs/current/errcodes-appendix.html — same
// constant internal/zones defines for itself rather than exporting one,
// since each package owns its own error mapping for constraints on its
// own tables.
const postgresForeignKeyViolation = "23503"

// ErrNotFound is returned when no delivery_agents row matches the given
// ID.
var ErrNotFound = errors.New("agent not found")

// ErrInvalidZone is returned when the given zone_id doesn't reference a
// real zone. UpdateZoneHandler already checks the zone exists (and is
// active) via zones.Repository before calling UpdateZone, so this is
// defense-in-depth — the current_zone_id foreign key should never
// actually fire in practice, same reasoning as zones.CreateArea's
// ErrZoneNotFound mapping.
var ErrInvalidZone = errors.New("zone not found")

// Repository is the storage interface agent handlers depend on. Every
// method returns the same joined AgentWithUser shape for response-mapping
// consistency across create/list/update.
type Repository interface {
	Create(ctx context.Context, input CreateAgentInput) (AgentWithUser, error)
	List(ctx context.Context) ([]AgentWithUser, error)
	FindByID(ctx context.Context, id string) (AgentWithUser, error)
	// FindByUserID looks an agent up by the linked users.id rather than
	// the agent's own id — what GetMyAgentHandler needs, since a caller's
	// JWT only carries their user ID, and the frontend has no other way
	// to discover its own agent.id before it can call the :id-keyed
	// availability/location endpoints.
	FindByUserID(ctx context.Context, userID string) (AgentWithUser, error)
	UpdateAvailability(ctx context.Context, id string, availability Availability) (AgentWithUser, error)
	UpdateLocation(ctx context.Context, id string, lat, lng float64) (AgentWithUser, error)
	// UpdateZone sets current_zone_id — the column assignment.IsEligible
	// actually reads. It is the only write path for that column in this
	// application; see docs/user-agent-management.md for why it was
	// missing until now.
	UpdateZone(ctx context.Context, id, zoneID string) (AgentWithUser, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const agentColumns = `
	da.id, da.user_id, u.full_name, u.email, u.phone,
	da.availability, da.current_lat, da.current_lng, da.current_zone_id,
	da.location_updated_at, da.last_assigned_at, da.active, da.created_at`

const agentJoin = `FROM delivery_agents da JOIN users u ON u.id = da.user_id`

// Create provisions a DELIVERY_AGENT identity: a users row and a
// delivery_agents row, in one transaction. If the delivery_agents insert
// fails after the users insert succeeds, the transaction rolls back —
// there is no code path that leaves a user row with no matching agent
// record, or vice versa. This is the STEP 6 requirement ("do not leave
// inconsistent partial state") enforced structurally, not by a
// best-effort cleanup step.
func (r *PostgresRepository) Create(ctx context.Context, input CreateAgentInput) (AgentWithUser, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AgentWithUser{}, fmt.Errorf("begin agent creation transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	createdUser, err := users.CreateTx(ctx, tx, users.NewUser{
		Email:        input.Email,
		PasswordHash: input.PasswordHash,
		FullName:     input.FullName,
		Phone:        input.Phone,
		Role:         users.RoleDeliveryAgent, // server-controlled; never from client input
	})
	if err != nil {
		if errors.Is(err, users.ErrEmailTaken) {
			return AgentWithUser{}, users.ErrEmailTaken
		}
		return AgentWithUser{}, fmt.Errorf("create agent's user record: %w", err)
	}

	var agentID string
	err = tx.QueryRow(ctx, `INSERT INTO delivery_agents (user_id) VALUES ($1) RETURNING id`, createdUser.ID).Scan(&agentID)
	if err != nil {
		return AgentWithUser{}, fmt.Errorf("create agent record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return AgentWithUser{}, fmt.Errorf("commit agent creation: %w", err)
	}

	return r.FindByID(ctx, agentID)
}

func (r *PostgresRepository) List(ctx context.Context) ([]AgentWithUser, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+agentColumns+` `+agentJoin+` ORDER BY da.created_at`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var out []AgentWithUser
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	return out, nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (AgentWithUser, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+agentColumns+` `+agentJoin+` WHERE da.id = $1`, id)
	a, err := scanAgent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AgentWithUser{}, ErrNotFound
		}
		return AgentWithUser{}, fmt.Errorf("find agent: %w", err)
	}
	return a, nil
}

func (r *PostgresRepository) FindByUserID(ctx context.Context, userID string) (AgentWithUser, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+agentColumns+` `+agentJoin+` WHERE da.user_id = $1`, userID)
	a, err := scanAgent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AgentWithUser{}, ErrNotFound
		}
		return AgentWithUser{}, fmt.Errorf("find agent by user id: %w", err)
	}
	return a, nil
}

func (r *PostgresRepository) UpdateAvailability(ctx context.Context, id string, availability Availability) (AgentWithUser, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE delivery_agents SET availability = $1 WHERE id = $2`, availability, id)
	if err != nil {
		return AgentWithUser{}, fmt.Errorf("update availability: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return AgentWithUser{}, ErrNotFound
	}
	return r.FindByID(ctx, id)
}

// UpdateLocation sets location_updated_at from the database's own clock
// (now()), never from any client-supplied value — STEP 5's explicit
// requirement that timestamps be server-generated.
func (r *PostgresRepository) UpdateLocation(ctx context.Context, id string, lat, lng float64) (AgentWithUser, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE delivery_agents SET current_lat = $1, current_lng = $2, location_updated_at = now() WHERE id = $3`,
		lat, lng, id,
	)
	if err != nil {
		return AgentWithUser{}, fmt.Errorf("update location: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return AgentWithUser{}, ErrNotFound
	}
	return r.FindByID(ctx, id)
}

// UpdateZone sets current_zone_id to a caller-supplied zone id. The
// caller (UpdateZoneHandler) already validated the zone exists and is
// active via zones.Repository — the foreign-key mapping below is
// defense-in-depth, not the primary check.
func (r *PostgresRepository) UpdateZone(ctx context.Context, id, zoneID string) (AgentWithUser, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE delivery_agents SET current_zone_id = $1 WHERE id = $2`, zoneID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			return AgentWithUser{}, ErrInvalidZone
		}
		return AgentWithUser{}, fmt.Errorf("update zone: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return AgentWithUser{}, ErrNotFound
	}
	return r.FindByID(ctx, id)
}

// rowScanner abstracts over pgx.Row (QueryRow) and pgx.Rows (Query's
// iteration) — both implement Scan identically, so List and FindByID can
// share one scan function.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanAgent(row rowScanner) (AgentWithUser, error) {
	var a AgentWithUser
	err := row.Scan(
		&a.ID, &a.UserID, &a.FullName, &a.Email, &a.Phone,
		&a.Availability, &a.CurrentLat, &a.CurrentLng, &a.CurrentZoneID,
		&a.LocationUpdatedAt, &a.LastAssignedAt, &a.Active, &a.CreatedAt,
	)
	return a, err
}
