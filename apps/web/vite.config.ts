/// <reference types="vitest/config" />

import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    proxy: {
      '/healthz': 'http://localhost:8080',
      '/readyz': 'http://localhost:8080',
      '/auth': 'http://localhost:8080',
      '/me': 'http://localhost:8080',
      '/recordings': 'http://localhost:8080',
      '/workspaces': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'jsdom',
  },
});
