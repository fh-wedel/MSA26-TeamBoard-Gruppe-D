import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': { target: 'http://traefik', changeOrigin: true },
      '/ws':  { target: 'ws://traefik',   changeOrigin: true, ws: true },
    },
  },
})
