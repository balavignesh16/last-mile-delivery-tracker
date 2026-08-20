// Package users owns the persistence layer for the users table: the User
// domain model, the three supported roles, and a Repository for reading
// and writing them. It has no HTTP handlers of its own in M02 — the
// register/login/me handlers live in internal/auth, which depends on this
// package for storage. See internal/auth's package doc for why.
package users

import "time"

// Role is one of the three roles the assignment requires. Stored as a
// plain string constrained by a CHECK constraint in the database, rather
// than a Postgres ENUM type, so adding a role later never requires an
// ALTER TYPE migration.
type Role string

const (
	RoleCustomer      Role = "CUSTOMER"
	RoleDeliveryAgent Role = "DELIVERY_AGENT"
	RoleAdmin         Role = "ADMIN"
)

// User is the full stored record, including the password hash. Handlers
// must never marshal this directly to JSON — always map to a response DTO
// that omits PasswordHash.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	FullName     string
	Phone        *string
	Role         Role
	CreatedAt    time.Time
}

// NewUser is the input to Repository.Create — deliberately has no ID or
// CreatedAt (assigned by the database) and no way to set Role from
// unchecked external input; callers must pass an explicit Role constant.
type NewUser struct {
	Email        string
	PasswordHash string
	FullName     string
	Phone        *string
	Role         Role
}
