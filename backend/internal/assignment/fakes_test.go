package assignment

import (
	"context"
	"errors"

	"lastmiletracker/internal/orders"
	"lastmiletracker/internal/users"
)

// fakeAssignmentRepo is an in-memory Repository used only to test
// AssignHandler/AutoAssignHandler's own concerns — request validation,
// RBAC (enforced by routes.go's middleware, not this fake), and
// status-code mapping — never the real transactional/locking logic,
// which is only meaningfully exercised against real Postgres (see
// tests/integration/assignment_integration_test.go). Same fake-repo
// pattern every prior module's handler_test.go uses.
type fakeAssignmentRepo struct {
	orders map[string]orders.Order
	agents map[string]bool // agentID -> exists

	assignErr     error
	autoAssignErr error
	// lastAssignAgentID records the agent_id AssignHandler forwarded, so
	// tests can confirm the handler passed the request body through
	// unmodified rather than mutating it.
	lastAssignAgentID string
}

func newFakeAssignmentRepo() *fakeAssignmentRepo {
	return &fakeAssignmentRepo{orders: map[string]orders.Order{}, agents: map[string]bool{}}
}

func (f *fakeAssignmentRepo) Assign(_ context.Context, orderID, agentID, _ string, _ users.Role) (orders.Order, error) {
	f.lastAssignAgentID = agentID
	if f.assignErr != nil {
		return orders.Order{}, f.assignErr
	}
	o, ok := f.orders[orderID]
	if !ok {
		return orders.Order{}, ErrOrderNotFound
	}
	if !f.agents[agentID] {
		return orders.Order{}, ErrAgentNotFound
	}
	assigned := agentID
	o.AssignedAgentID = &assigned
	o.Status = "ASSIGNED"
	f.orders[orderID] = o
	return o, nil
}

func (f *fakeAssignmentRepo) AutoAssign(_ context.Context, orderID, _ string, _ users.Role) (orders.Order, error) {
	if f.autoAssignErr != nil {
		return orders.Order{}, f.autoAssignErr
	}
	o, ok := f.orders[orderID]
	if !ok {
		return orders.Order{}, ErrOrderNotFound
	}
	o.Status = "ASSIGNED"
	f.orders[orderID] = o
	return o, nil
}

var errFakeInternal = errors.New("fake internal error")
