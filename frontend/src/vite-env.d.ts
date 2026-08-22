/// <reference types="vite/client" />

interface ImportMetaEnv {
  // Optional, build-time API origin (e.g. "https://api.example.com").
  // Left unset, every request stays a same-origin relative path exactly
  // as before — correct for local dev (Vite's own proxy, see
  // vite.config.ts) and for any deployment where the frontend is served
  // from the same origin as the backend. Set it only when the frontend
  // and backend are hosted on different origins — see services/api.ts.
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
