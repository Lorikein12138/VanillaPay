import { fileURLToPath, URL } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

const backend = 'http://localhost:8080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    outDir: '../internal/webassets/dist',
    emptyOutDir: true,
    // The Go handler marks only hex-hashed asset names as immutable.
    rolldownOptions: {
      output: {
        hashCharacters: 'hex',
      },
    },
  },
  server: {
    proxy: {
      '/api': backend,
      '/media': backend,
      '/health': backend,
    },
  },
})
