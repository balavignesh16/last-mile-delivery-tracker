# Seed data

Reserved for demo seed data per the frozen architecture's evaluator demo
scenarios (zones, rate cards, and other data later modules introduce).

M02's demo accounts (one Admin, one Delivery Agent, one Customer — see
[`docs/authentication.md`](../../docs/authentication.md)) do **not** live
here as a script. They're seeded by `internal/auth.SeedDemoUsers`, called
automatically from `main.go` on every backend startup, right alongside the
database migrations — no separate command to remember to run, consistent
with the project's one-command-startup goal. This directory is still the
right home for later modules whose seed data is larger or more elaborate
(e.g. zones and rate cards in M04/M05), where a standalone script becomes
more appropriate than folding it into startup.
