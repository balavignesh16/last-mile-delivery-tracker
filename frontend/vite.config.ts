import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // Dev-time proxy so the frontend can call same-origin paths instead of
    // needing CORS configuration on the backend. No rewrite: the backend's
    // real routes are already at /api/v1/* (per the frozen API design) and
    // /health, so the proxy forwards both prefixes byte-for-byte.
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
      '/health': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
  test: {
    // jsdom (not 'node') because M02 introduces real component/context
    // state (AuthContext, ProtectedRoute) worth rendering and asserting
    // on, not just pure-function tests.
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
  },
})
