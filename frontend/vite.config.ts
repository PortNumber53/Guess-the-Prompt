import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'

import { cloudflare } from "@cloudflare/vite-plugin";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), cloudflare()],
  server: {
    host: '0.0.0.0',
    port: 21050,
    allowedHosts: ['guess14.dev.portnumber53.com'],
    proxy: {
      '/v1': {
        target: 'http://localhost:21051',
        changeOrigin: true,
      },
      '/objects': {
        target: 'http://localhost:21051',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://localhost:21051',
        ws: true,
      },
    },
  },
})