# Dashboards & Evaluation Layer (M12)

## Scope

M12 is a thin, role-specific navigation and read-only composition layer
over the API M01–M11 already built — it is not a new backend module,
and it introduces no new business capability of its own except closing
one pre-existing frontend gap (delivery-agent status updates) and
adding one genuinely new, source-required backend capability (admin
order filtering). Everything else is links and small, derived-data
widgets over already-existing, already-tested endpoints.

## Customer dashboard

`pages/customer/DashboardPage.tsx` (`/customer/dashboard`, `CUSTOMER`
only) links to `CreateOrderPage` (`/orders/new`) and `OrdersPage`
(`/orders`, customer-scoped). Order Details, the Tracking Timeline, and
Reschedule Failed Order — the blueprint's other three Customer
Dashboard sub-items — are reached through My Orders → an order's own
detail page, exactly as before this page existed. This page duplicates
none of `OrderDetailPage`'s content, and every order shown is still
resolved by the backend's own customer-scoped `GET /orders` query,
never widened here.

## Delivery-agent dashboard

`pages/agent/DashboardPage.tsx` (`/agent/dashboard`, `DELIVERY_AGENT`
only) links to `OrdersPage` (agent-scoped to their own assigned orders,
unchanged) and the existing `OperationsPage` (`/agent`, availability +
location, M03). Delivery Details and Update Status are reached through
Assigned Deliveries → an order's own detail page.

### Agent status-update UI (the one real gap this module closes)

Before M12, `internal/tracking/statemachine.go`'s `edgeAuthorizedRoles`
already authorized `DELIVERY_AGENT` for five edges (`ASSIGNED→PICKED_UP`
through `OUT_FOR_DELIVERY→DELIVERED`/`FAILED`), but `OrderDetailPage`'s
status-update section rendered only for `isAdmin` — an agent had no UI
path to use a capability the backend always granted them. M12 widens
that one render condition to `isAdmin || isDeliveryAgent`, and adds
`transitionsForRole` (`types/tracking.ts`) — a frontend mirror of
`edgeAuthorizedRoles`, narrowing `LEGAL_TRANSITIONS` to only the edges a
given role may perform, the same "UX convenience, not a security
boundary" disclaimer `LEGAL_TRANSITIONS` itself already carries. `GET
/orders/:id` already 404s an agent out of any order not assigned to
them, so by the time an order renders on this page for a
`DELIVERY_AGENT`, ownership is already backend-guaranteed. No backend
file changed: `legalTransitions`, `edgeAuthorizedRoles`,
`IsValidTransition`, and `IsRoleAuthorized` are byte-for-byte untouched.
One real, pre-existing bug surfaced by this change was fixed:
`handleTransition` previously refetched the tracking timeline
unconditionally after every transition, which would have 403'd for an
agent (`GET /orders/:id/tracking` stays `ADMIN`/`CUSTOMER`-only,
unchanged); it now only refetches tracking when `canViewTracking`.

## Admin dashboard

`pages/admin/DashboardPage.tsx` (`/admin/dashboard`, `ADMIN` only)
computes order statistics from a real, unfiltered `GET /orders` call —
simple counts by status, using the same eight-value status vocabulary
`ORDER_STATUSES` (`types/order.ts`) mirrors from the backend — and
links to `OrdersPage`, `AgentsPage`, `ZonesPage` (Areas are already
managed inside it), and `RatesPage`. It carries no order list of its
own, which would duplicate `OrdersPage` rather than compose it.
Assignment and status override remain reached through Orders → an
order's own detail page, unchanged.

## Admin order filtering

`GET /api/v1/orders` gained three optional, combinable query
parameters — `status`, `zone`, `agent` — honored for `ADMIN` only.
`internal/orders/repository.go`'s new `OrderFilter` struct and
`ListAllOrders(ctx, filter)` build the `WHERE` clause dynamically from
whichever fields are non-empty; a zero-value filter is the exact
unfiltered query every prior milestone relied on, so existing,
parameter-free calls are provably unchanged. `zone` matches an order
whose *pickup or drop* zone is the given zone. An invalid `status`
value is rejected `422` via `tracking.ParseStatus` — the same canonical
status vocabulary `tracking` already owns, reused rather than
duplicated a third time. An unknown `zone`/`agent` id yields an empty
result, the same "unknown id, not an error" convention
`ListOrdersForAgent`/`ListOrdersForCustomer` already use. A `CUSTOMER`
or `DELIVERY_AGENT` supplying these parameters gets them silently
ignored — their pre-M12, role-scoped result, never widened; the backend
decides visibility by role, never by a client-supplied filter.
`OrdersPage.tsx` renders the three filter `<select>`s only for an
`ADMIN` viewer.

## RBAC boundaries

No dashboard, filter, or statistics feature changes who can see what.
Every data source M12 touches was already ownership-scoped server-side;
the one new query surface (order filtering) narrows an `ADMIN`'s
already-unrestricted view and is a no-op for every other role. The one
new client-facing action (agent status updates) targets an endpoint
whose authorization was already fully enforced and tested before M12.

## Why no new database tables

Every field these dashboards need already exists: `orders.status`,
`orders.pickup_zone_id`/`drop_zone_id`, `orders.assigned_agent_id`.
Order statistics are computed client-side over already-fetched data at
this project's evaluator-demo scale — no aggregate/analytics table, no
migration.

## Why no dashboard backend module

The blueprint's own architecture diagram, API structure, and repository
structure never name a dashboards backend package — M12 is presentation
composed over the existing API surface, the same way `docs/api.md`
already organizes existing endpoints by capability rather than by page.

## Explicitly out of scope

Charts, graphs, maps, search, pagination, real-time/WebSocket updates,
a notification bell/history/preferences UI (M11 remains untouched and
still has no frontend surface), export functionality, extra analytics
or KPIs beyond simple status counts, and any new frontend dependency.
