import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

// Backend API origin used by the dev server proxy. Override with
// VITE_API_PROXY_TARGET when the API runs somewhere other than :8080.
const apiProxyTarget = process.env.VITE_API_PROXY_TARGET ?? 'http://localhost:8080'

// https://vite.dev/config/
export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  server: {
    proxy: {
      // Forward API calls to the Go backend so the SPA can call same-origin
      // paths in development without CORS configuration.
      '/api': {
        target: apiProxyTarget,
        changeOrigin: true,
      },
    },
  },
})
