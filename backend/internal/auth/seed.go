package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"lastmiletracker/internal/users"
)

// Demo credentials. Intentionally public and hard-coded — the whole point
// of seed data is that an evaluator can log in as any of the three roles
// without a real admin-provisioning flow, which doesn't exist until M03's
// POST /agents. These are documented in the README as demo-only; never
// reuse this pattern for real production credentials.
const (
	seedAdminEmail       = "admin@lastmile.test"
	seedAdminPassword    = "Admin123!"
	seedAgentEmail       = "agent@lastmile.test"
	seedAgentPassword    = "Agent123!"
	seedCustomerEmail    = "customer@lastmile.test"
	seedCustomerPassword = "Customer123!"
)

type seedUser struct {
	email    string
	password string
	fullName string
	role     users.Role
}

var demoUsers = []seedUser{
	{seedAdminEmail, seedAdminPassword, "Demo Admin", users.RoleAdmin},
	{seedAgentEmail, seedAgentPassword, "Demo Agent", users.RoleDeliveryAgent},
	{seedCustomerEmail, seedCustomerPassword, "Demo Customer", users.RoleCustomer},
}

// SeedDemoUsers creates one demo account per role if it does not already
// exist. Idempotent and safe to call on every startup — same pattern as
// database.Migrate. This is the project's answer to "how does an ADMIN
// account ever get created": not through public registration (which only
// ever creates CUSTOMER accounts, see RegisterHandler), but through this
// fixed, version-controlled seed step.
func SeedDemoUsers(ctx context.Context, repo users.Repository, logger *slog.Logger) error {
	for _, d := range demoUsers {
		_, err := repo.FindByEmail(ctx, d.email)
		if err == nil {
			continue // already seeded
		}
		if !errors.Is(err, users.ErrNotFound) {
			return fmt.Errorf("check seed user %s: %w", d.email, err)
		}

		hash, err := HashPassword(d.password)
		if err != nil {
			return fmt.Errorf("hash seed password for %s: %w", d.email, err)
		}

		if _, err := repo.Create(ctx, users.NewUser{
			Email:        d.email,
			PasswordHash: hash,
			FullName:     d.fullName,
			Role:         d.role,
		}); err != nil {
			return fmt.Errorf("create seed user %s: %w", d.email, err)
		}
		logger.Info("seeded demo user", "email", d.email, "role", d.role)
	}
	return nil
}
