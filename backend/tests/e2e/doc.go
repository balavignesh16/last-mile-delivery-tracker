// Package e2e is reserved for full-stack HTTP flow tests (register → quote
// → order → assign → status updates → delivered, and the failed →
// reschedule → reassign variant), per the frozen test strategy. These begin
// once there is a business flow to exercise, starting around M06/M07 —
// intentionally empty during M01.
package e2e
