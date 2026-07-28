import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { resolve } from 'node:path'

export default defineConfig({
  base: './',
  plugins: [react()],
  build: {
    outDir: resolve(__dirname, '../cmd/fndns/web/dist'),
    emptyOutDir: true,
    sourcemap: false,
    assetsInlineLimit: 4096,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:18788',
    },
  },
})
