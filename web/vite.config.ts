import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// In development the Go server runs on :8787 and Vite forwards API and login
// requests to it. Production builds go to dist/, which the Go binary embeds.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5180,
    proxy: {
      '/api': 'http://localhost:8787',
      '/auth': 'http://localhost:8787',
    },
  },
  build: { outDir: 'dist', emptyOutDir: true },
})
