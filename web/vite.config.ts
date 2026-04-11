import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// Backend URL: set VITE_BACKEND_URL env var to proxy to a remote backend (e.g. NAS).
// Defaults to http://localhost:8080 for local development.
// Example: VITE_BACKEND_URL=http://192.168.1.100:8080 npm run dev
const backendUrl = process.env.VITE_BACKEND_URL || 'http://localhost:8080';
const wsBackendUrl = backendUrl.replace(/^http/, 'ws');

export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy: {
      '/api': {
        target: backendUrl,
        changeOrigin: true,
      },
      '/stream': {
        target: backendUrl,
        changeOrigin: true,
      },
      '/ws': {
        target: wsBackendUrl,
        ws: true,
      },
    },
  },
  build: {
    outDir: 'dist',
  },
});
