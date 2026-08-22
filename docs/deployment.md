# Deployment

This project deploys as two separately-hosted pieces — a static frontend
and a containerized backend — exactly the split the assignment's own
example platforms (Vercel for frontend; Render/Railway for backend)
assume. Nothing below requires a new dependency: the backend already has
a working `Dockerfile` (used by `docker-compose.yml` today), and the
frontend already builds to static files via `vite build`.

## Why two origins need explicit configuration

In local dev, Vite's own proxy (`frontend/vite.config.ts`) forwards
`/api/*` and `/health` to `http://localhost:8080`, so every frontend
request is same-origin as far as the browser is concerned — no CORS
configuration is needed, and the frontend never needs to know the
backend's address.

A hosted deployment has no such proxy: the frontend is served from one
origin (e.g. `https://your-app.vercel.app`) and the backend from another
(e.g. `https://your-api.onrender.com`). Two things must be configured
for that to work at all:

1. **The frontend must know the backend's real URL** — `VITE_API_BASE_URL`
   (see `frontend/.env.example`), read once at build time and prepended
   to every request in `services/api.ts`. Unset, it defaults to an empty
   string (today's relative-path behavior) — safe, but means the
   frontend would try to call itself once deployed unless this is set.
2. **The backend must explicitly allow that frontend origin** —
   `CORS_ALLOWED_ORIGINS` (see root `.env.example`), read by
   `internal/server.CORS` and wrapped around the router in
   `cmd/server/main.go`. Unset, no CORS header is ever added (today's
   behavior, unaffected) — the deployed frontend's requests would be
   blocked by the browser without this.

Both are additive, off-by-default, and were verified together against a
live container in this project's own manual verification (a real
preflight `OPTIONS` request and a real `GET /health` both received the
correct `Access-Control-Allow-Origin` header only once
`CORS_ALLOWED_ORIGINS` was set to match).

## Backend (Render, Railway, or similar)

The existing `backend/Dockerfile` builds and runs the server as-is —
point the hosting platform at it directly (most platforms auto-detect a
`Dockerfile` in a subdirectory; set the build context to `backend/`).

Required environment variables (see `.env.example` for the full list
and description of each): `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`,
`DB_PASSWORD`, `DB_SSLMODE` (a hosted Postgres instance typically
requires `require` here, not `disable`), `JWT_SECRET` (a long, random
value — never the development placeholder), `SERVER_HOST=0.0.0.0`,
`SERVER_PORT` (or let the platform inject its own and read it the same
way `SERVER_PORT` already is).

Set once the frontend's URL is known: `CORS_ALLOWED_ORIGINS=https://your-app.vercel.app`
(comma-separate multiple origins if needed — e.g. a preview URL and a
production URL).

Optional: `EMAIL_PROVIDER=resend` + `RESEND_API_KEY` + `RESEND_FROM_EMAIL`
to send real email instead of the log-based default — see
`docs/notifications.md`. Leaving `EMAIL_PROVIDER` unset is completely
safe or leaving it as "log" — the app runs identically either way.

On boot, the backend applies any unapplied migrations from
`backend/migrations/` and seeds the three demo accounts automatically
(the same idempotent startup sequence `docker compose up` already
performs) — no separate migrate step is needed on the hosting platform.

## Frontend (Vercel or similar)

Framework preset: Vite. Root directory: `frontend/`. Build command:
`npm run build` (equivalent to `tsc -b && vite build`). Output directory:
`dist`.

Set `VITE_API_BASE_URL` to the backend's real, deployed URL (e.g.
`https://your-api.onrender.com`) as a build-time environment variable —
Vite inlines `VITE_*` variables into the build, so this must be set
*before* building, not just at runtime.

## Verifying a deployment

1. Visit the hosted frontend URL; the home page's health check
   (`useHealthCheck`) should show both Backend and Database as
   reachable — this alone proves `VITE_API_BASE_URL` and CORS are both
   correctly configured, since it's a real cross-origin request.
2. Log in with a demo account (`admin@lastmile.test` / `Admin123!`, or
   the customer/agent equivalents — see root `README.md`).
3. Walk through one full order lifecycle (create → assign → status
   updates → delivered) to confirm the deployed backend's database and
   business logic are working end to end, not just its health check.

## CI

`.github/workflows/ci.yml` runs the same checks documented under
"Running Tests" in the root `README.md` (backend `gofmt`/`go vet`/
`go build`/`go test`, backend integration + E2E tests against a real
Postgres service container, frontend `tsc`/`oxlint`/`vitest`/`vite build`)
on every push and pull request to `main` — it does not deploy anything
itself; deployment happens through whichever hosting platform's own
GitHub integration is connected (both Vercel and Render/Railway can
auto-deploy on push once the repository is connected in their
dashboards).
