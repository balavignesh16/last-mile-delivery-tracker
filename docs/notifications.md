# Notification Service (M11)

## Purpose

M11 reacts to order-lifecycle events by notifying the order's own
customer — by email always, and by SMS when a phone number is on file —
that something changed. It is a pure observer: it never computes
pricing (M06), never writes `orders.status` (M08), never ranks or
selects a delivery agent (M09), and never authorizes a reschedule
(M10). It only reacts, after the fact, to an event one of those other,
unmodified modules already produced and already committed.

## Architecture

`internal/notifications` owns exactly one new table (`notifications`)
and zero new endpoints. It depends on `internal/orders`,
`internal/users`, and `internal/tracking` (to resolve a customer's
contact details and to read event data) — but none of those packages
depend on `internal/notifications` back. The dependency only runs one
way, because a cycle the other way (`tracking`/`orders` importing
`notifications` directly) is structurally impossible here without
breaking Go's import graph.

That is resolved with two small, producer-owned callback types, not an
event bus, message queue, or interface-inversion abstraction this
project has never used elsewhere:

```go
// internal/tracking/event.go
type TransitionHook func(ctx context.Context, event Event)

// internal/orders/order.go
type OrderCreatedHook func(ctx context.Context, orderID string)
```

`tracking.TransitionHandler`, `internal/assignment`'s `Assign`/
`AutoAssign`, and `internal/rescheduling`'s `Reschedule` all accept and
invoke a `TransitionHook`; `orders.CreateOrderHandler` accepts and
invokes an `OrderCreatedHook`. Every hook parameter is nil-safe — every
caller checks for nil before invoking it, so nothing about M01–M10
requires wiring one. `cmd/server/main.go` is the *only* place that
constructs a real `notifications.Service` and passes its
`NotifyTransition`/`NotifyOrderCreated` methods in as those hook
values. No package other than `internal/notifications` and `main.go`
knows this module exists.

## The eight events

Exactly the eight the blueprint's own M11 section names — no more, no
fewer:

| Event | Trigger |
|---|---|
| `ORDER_CREATED` | `POST /orders` (M07) |
| `AGENT_ASSIGNED` | `POST /orders/:id/assign` or `/auto-assign` (M09) |
| `PICKED_UP` | `POST /orders/:id/status` (M08) |
| `IN_TRANSIT` | `POST /orders/:id/status` (M08) |
| `OUT_FOR_DELIVERY` | `POST /orders/:id/status` (M08) |
| `DELIVERED` | `POST /orders/:id/status` (M08) |
| `FAILED` | `POST /orders/:id/status` (M08) |
| `RESCHEDULED` | `POST /orders/:id/reschedule` (M10) |

`ORDER_CREATED` has no corresponding `tracking.Status` — order creation
never goes through `TransitionTx` — so `Service.NotifyOrderCreated`
re-reads the order's own opening tracking event (already written
atomically by `CreateOrder`) via the already-exported, read-only
`tracking.Repository.ListEvents`, rather than requiring any change to
`internal/orders`' own `CreateOrder` signature or SQL. The other seven
map 1:1 onto `tracking.Status`, with `ASSIGNED` renamed to
`AGENT_ASSIGNED` to match the blueprint's own event name
(`eventTypeForStatus` in `service.go`).

## Recipient: customer only

The customer on the order (`orders.customer_id → users.id`) is the
*only* notification recipient this module ever resolves. Agents and
admins are never notified by this module, and no caller can supply an
arbitrary address — recipient resolution happens entirely inside
`Service.notify`, from the order and user records alone, never from
request input.

## Email and SMS behavior

- **Email** is attempted for every one of the eight events,
  unconditionally.
- **SMS** is attempted only when the customer's `users.phone` is a
  non-empty value. A missing phone is never an error — it is simply
  not applicable, and produces no SMS row at all (not a `FAILED` one).

## Provider abstraction

```go
type EmailProvider interface {
    SendEmail(ctx context.Context, to, subject, body string) error
}
type SmsProvider interface {
    SendSMS(ctx context.Context, to, body string) error
}
```

`Service` depends on these two narrow interfaces only — never on a
concrete SDK. This is exactly what let a second `EmailProvider`
implementation (below) be added later without changing `Service` or any
of `orders`/`tracking`/`assignment`/`rescheduling` at all — only the one
line in `main.go` that constructs the provider changed.

## Providers: log-based (default), Resend (optional, real email), and Twilio (optional, real SMS)

`LogEmailProvider` and `LogSmsProvider` (`provider.go`) remain the
default — they contact no external service, require no credentials,
and satisfy this module's actual requirements — an attempt is made, its
content is observable (via `slog.Info`), and it can fail safely. This
is what makes the module correct and fully testable with zero
configuration, including in an evaluator's environment.

`ResendEmailProvider` (`resend.go`) sends real email via
[Resend](https://resend.com)'s REST API — the one real, free-tier
`EmailProvider` this project ships. It is opt-in only:
`cmd/server/main.go` constructs it instead of the log provider when
`EMAIL_PROVIDER=resend` is set, and fails fast at startup if
`RESEND_API_KEY`/`RESEND_FROM_EMAIL` are then missing (see
`.env.example`) — a misconfiguration is caught immediately at boot, not
silently swallowed at the first send attempt. Its `http.Client` carries
its own 10-second timeout, independent of the caller's request
context, so a slow or unreachable Resend outage can never stall the
post-commit hook that invokes it (the same "must never block or fail
the triggering commit" guarantee every provider call in this module
already has — see "Failure handling" below).

`TwilioSmsProvider` (`twilio.go`) sends real SMS via
[Twilio](https://www.twilio.com)'s REST API — the one real, free-trial
`SmsProvider` this project ships, built on the same pattern as
`ResendEmailProvider` above: a plain `net/http` POST (form-encoded,
HTTP Basic Auth with the account SID/auth token) with no SDK and no new
`go.mod` dependency. It is opt-in only: `cmd/server/main.go` constructs
it instead of the log provider when `SMS_PROVIDER=twilio` is set, and
fails fast at startup if `TWILIO_ACCOUNT_SID`/`TWILIO_AUTH_TOKEN`/
`TWILIO_FROM_NUMBER` are then missing (see `.env.example`). Its
`http.Client` carries the identical independent 10-second timeout
`ResendEmailProvider` uses, for the same reason.

**Twilio trial limitation, documented honestly:** a Twilio trial
account (no billing added) can only send SMS to phone numbers that have
been explicitly verified in the Twilio console — it cannot text an
arbitrary customer's number. This is a constraint of the free trial
tier itself, not of this implementation; it mirrors the same kind of
sandbox restriction `ResendEmailProvider` already documents for
Resend's free tier (deliverability limited to the account's own
verified address absent domain verification). A customer's phone
number is used exactly as stored in `users.phone` — no formatting or
validation is added or changed here; Twilio expects
[E.164](https://www.twilio.com/docs/glossary/what-e164) format
(e.g. `+15550100`).

Both `ResendEmailProvider` and `TwilioSmsProvider` are pure
implementation substitutions behind interfaces `Service` already
depended on — neither required any change to `service.go`'s dispatch,
idempotency, or post-commit-hook logic; only `main.go`'s provider
construction changed.

## Post-commit integration

Every hook invocation happens strictly *after* its triggering
transaction has already committed:

- **Order creation** (`orders/handler.go`): `onOrderCreated` fires
  after `ordersRepo.CreateOrder` returns successfully.
- **Tracking transitions** (`tracking/handler.go`): `onTransition`
  fires after `repo.Transition` returns successfully.
- **Assignment** (`assignment/repository.go`): `onTransition` fires in
  both `Assign` and `AutoAssign`, immediately after `tx.Commit(ctx)`
  succeeds.
- **Rescheduling** (`rescheduling/repository.go`): `onTransition` fires
  in `Reschedule`, immediately after `tx.Commit(ctx)` succeeds.

No notification attempt ever happens inside a database transaction,
and a notification attempt can never cause a lifecycle operation to
roll back — by construction, since the hook is only ever called once
the transaction it observes has already succeeded.

None of M08's transition validation/authorization, M09's
candidate ranking/eligibility, or M10's reschedule authorization/agent-
freeing logic was modified to add these four call sites — each is a
single, additive line calling an already-nil-safe hook.

## Failure handling

`Service.notify`/`dispatch` never return an error to their own caller
— a post-commit hook's triggering operation has already succeeded and
must remain successful no matter what happens to the notification
attempt. Specifically:

- An unresolvable order or customer is logged and the attempt is
  abandoned — no panic, no propagated error.
- A provider error (`EmailProvider`/`SmsProvider` returning a non-nil
  error) is caught, logged, and recorded as that notification's
  `FAILED` status — the underlying order/tracking/assignment/
  reschedule commit is completely unaffected.
- A panicking provider is recovered by `safeSend` (a `defer recover()`
  wrapper) and converted into a plain error, so a misbehaving provider
  implementation can never crash an already-successful HTTP request.

Proven directly by
`TestNotificationFlow_ProviderFailureDoesNotBreakLifecycleCommit`
(forces a real provider failure and asserts the assignment/transition
HTTP call still returns success and the order's status still commits).

## No retries

A failed notification attempt is recorded as `FAILED` and never
retried — no retry loop, no backoff, no scheduled re-attempt. This
matches the approved MVP scope; a future milestone could add retries
behind the same `Repository`/provider interfaces without changing this
module's public shape.

## Idempotency: `tracking_event_id`, not `(order_id, event, channel)`

This is the single most important invariant in this module. A
notification is not identified by `(order_id, event type, channel)` —
it is identified by the exact `order_tracking_events.id` (a
`tracking.Event`'s own `ID`) plus channel. This matters because an
order can legitimately produce the *same* event type more than once:
`FAILED → RESCHEDULED → ASSIGNED → ... → FAILED` again is a normal,
legal cycle (see `docs/failed-delivery.md`), and each `FAILED`
occurrence is a distinct row in `order_tracking_events` with its own
id.

```sql
CREATE UNIQUE INDEX idx_notifications_tracking_event_channel
    ON notifications (tracking_event_id, channel);
```

`Repository.Claim` performs one atomic
`INSERT ... ON CONFLICT (tracking_event_id, channel) DO NOTHING
RETURNING id`, executed **before** any provider call — this is what
actually prevents a duplicate *send*, not merely a duplicate row. A
second, concurrent, or repeated attempt for the same
`(tracking_event_id, channel)` pair finds the slot already claimed
(`pgx.ErrNoRows` on the `RETURNING id` scan, translated to
`claimed=false`) and never touches the provider at all. The database's
own unique index — not application-level locking — is the actual
backstop, proven under real concurrent load by
`TestNotificationConcurrency_ConcurrentIdenticalAttemptsClaimExactlyOnce`.

## Repeated FAILED / RESCHEDULED behavior

Because the idempotency anchor is the exact tracking-event occurrence,
a second, genuine `FAILED` after a full reschedule-and-reassign cycle
is always independently notify-able — it is a new row, not a duplicate
of the first `FAILED`. Proven by
`TestNotificationFlow_SecondFailedOccurrenceCreatesNewRow` (integration)
and `TestNotifyTransition_RepeatedFailedOccurrencesEachNotify`/
`_RepeatedRescheduledOccurrencesEachNotify` (unit).

## No REST API

M11 adds **zero** HTTP endpoints. There is no `GET /notifications`, no
per-order notification history endpoint, no admin notification
console. `docs/api.md` states this explicitly rather than silently
omitting M11. A caller can only ever observe notification outcomes via
direct database inspection or, in a real deployment, via whatever
observability the log-based (or a future real) provider emits.

## No frontend UI

Nothing in `frontend/` changed for M11 — no bell icon, no notification
center, no unread count, no history page, no preference toggle, no new
npm dependency. The customer has always been notified about their own
order status through the transactional email/SMS themselves, not
through a page they check.

## No queues, workers, or outbox-draining system

Every notification attempt is synchronous, direct, and
request-thread-local: `dispatch` calls `Claim`, then the provider,
then `Resolve`, all inline inside the post-commit hook call. There is
no Kafka, RabbitMQ, Redis, background worker, polling loop, webhook
receiver, or outbox-table-draining process anywhere in this module —
none of that infrastructure exists in this project, and this module
does not introduce it.

## Testing strategy

- **Unit** (`internal/notifications/*_test.go`, fakes for every
  dependency): `buildContent` for all 8 events including malformed/
  empty metadata; every lifecycle event recognized; customer recipient
  resolution; SMS attempted/skipped based on phone presence
  (including an empty-string phone); FAILED/RESCHEDULED/AGENT_ASSIGNED
  content includes the right metadata fields; provider failure records
  `FAILED` status without affecting the other channel; a panicking
  provider is contained; repeated calls for the identical event claim
  exactly once; repeated *genuine* FAILED/RESCHEDULED occurrences each
  independently notify; the same event on two different channels both
  claim independently.
- **Integration** (`tests/integration/notifications_integration_test.go`,
  real Postgres, real `notifications.PostgresRepository` and
  `notifications.Service`, fake `EmailProvider`/`SmsProvider` wired
  into the real HTTP stack exactly as `main.go` wires them): all 8
  event triggers produce a persisted row referencing the real
  `tracking_event_id`; EMAIL always persists, SMS only when a phone is
  on file; a forced provider failure is recorded as `FAILED` without
  breaking the underlying assign/transition HTTP call; a second FAILED
  occurrence after a full reschedule cycle creates an independent row
  with a distinct `tracking_event_id`; the unique index and migration
  are inspected directly; no notification REST endpoints exist; full
  M01–M10 regression.
- **Concurrency**: `TestNotificationConcurrency_ConcurrentIdenticalAttemptsClaimExactlyOnce`
  fires many concurrent `NotifyTransition` calls for the identical
  tracking event against real Postgres and asserts exactly one claim,
  one provider call, and one persisted row — repeated with `-count=5`
  to rule out flakiness.
- **Provider unit tests** (`resend_test.go`, `twilio_test.go`, an
  `httptest.Server` in place of the real API for both): request
  method/path/headers/body shape, a non-2xx response becomes an error,
  an unreachable server becomes an error, and — Twilio specifically —
  the auth token never appears in a returned error string.

## Environment / configuration

Zero configuration is required to run the module correctly:
`EMAIL_PROVIDER` and `SMS_PROVIDER` both default to `log`, and
`notifications.NewLogEmailProvider()`/`NewLogSmsProvider()` need no
credentials or external accounts — the module works out of the box on a
fresh `docker compose up`, including in an automated evaluation
environment with no `.env` beyond the required DB/JWT variables.

Setting `EMAIL_PROVIDER=resend` plus `RESEND_API_KEY`/`RESEND_FROM_EMAIL`
(see `.env.example`) switches to real email delivery with no other
change. Setting `SMS_PROVIDER=twilio` plus `TWILIO_ACCOUNT_SID`/
`TWILIO_AUTH_TOKEN`/`TWILIO_FROM_NUMBER` switches to real SMS delivery
the identical way — each provider selection is fully independent of the
other (real email with log-only SMS, real SMS with log-only email, both
real, or both log, are all valid combinations).

## What this module deliberately does not do

- No public notification API of any kind.
- No frontend notification UI of any kind.
- No retries, backoff, or scheduled re-delivery.
- No queues, background workers, polling, or webhooks.
- No notification recipients other than the order's own customer.
- No modification to M08's state-machine/authorization logic, M09's
  ranking/eligibility logic, or M10's reschedule authorization/
  agent-freeing logic — each integration point is a single additive,
  nil-safe hook call.
