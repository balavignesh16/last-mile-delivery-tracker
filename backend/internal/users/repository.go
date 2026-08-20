package users

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrEmailTaken is returned by Create when the email already belongs to
// another user (the users.email UNIQUE constraint fired).
var ErrEmailTaken = errors.New("email already registered")

// ErrNotFound is returned by FindByEmail/FindByID when no matching row
// exists.
var ErrNotFound = errors.New("user not found")

// postgresUniqueViolation is the PostgreSQL SQLSTATE for a UNIQUE
// constraint violation. See https://www.postgresql.org/docs/current/errcodes-appendix.html
const postgresUniqueViolation = "23505"

// Repository is the storage interface auth handlers depend on. An
// interface — rather than the concrete *PostgresRepository — specifically
// so handler unit tests can inject an in-memory fake instead of requiring
// a real database for every test.
type Repository interface {
	Create(ctx context.Context, u NewUser) (User, error)
	FindByEmail(ctx context.Context, email string) (User, error)
	FindByID(ctx context.Context, id string) (User, error)
}

// PostgresRepository is the real Repository implementation.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, u NewUser) (User, error) {
	const q = `
		INSERT INTO users (email, password_hash, full_name, phone, role)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, email, password_hash, full_name, phone, role, created_at`

	var out User
	err := r.pool.QueryRow(ctx, q, u.Email, u.PasswordHash, u.FullName, u.Phone, u.Role).Scan(
		&out.ID, &out.Email, &out.PasswordHash, &out.FullName, &out.Phone, &out.Role, &out.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return out, nil
}

func (r *PostgresRepository) FindByEmail(ctx context.Context, email string) (User, error) {
	const q = `
		SELECT id, email, password_hash, full_name, phone, role, created_at
		FROM users WHERE email = $1`
	return r.scanOne(ctx, q, email)
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (User, error) {
	const q = `
		SELECT id, email, password_hash, full_name, phone, role, created_at
		FROM users WHERE id = $1`
	return r.scanOne(ctx, q, id)
}

func (r *PostgresRepository) scanOne(ctx context.Context, query string, arg any) (User, error) {
	var out User
	err := r.pool.QueryRow(ctx, query, arg).Scan(
		&out.ID, &out.Email, &out.PasswordHash, &out.FullName, &out.Phone, &out.Role, &out.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("query user: %w", err)
	}
	return out, nil
}
