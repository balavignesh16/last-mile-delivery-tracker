// Package e2e implements the full-stack HTTP flow tests named in the
// original test strategy — register → quote → order → assign → status
// updates → delivered, and the failed → reschedule → reassign variant —
// via files gated behind the "e2e" build tag (helpers_test.go,
// lifecycle_test.go). Run with `go test -tags=e2e ./tests/e2e/...`
// against a real Postgres instance (TEST_DATABASE_URL), the same
// convention `tests/integration` already uses.
//
// These landed in M12, not M06/M07 as an earlier draft of this comment
// claimed — the flows exercise assignment (M09), status transitions
// (M08), and rescheduling (M10) end to end, so a genuine full lifecycle
// test could not exist before all of those modules did. This file
// itself carries no test code — Go requires at least one untagged file
// for `tests/e2e` to remain a valid package outside the "e2e" build tag.
package e2e
