/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import { fileURLToPath, URL } from 'node:url';

// BuildOS Office Console — Vite config.
//
// The only required runtime config is VITE_API_BASE_URL (FRONTEND_ARCHITECTURE §6.1).
// Everything else (which features are on) is discovered at runtime from the server
// so a single build is deployable across forks. In production the console is served
// same-origin behind the Go server, so the default base is the relative "/".
export default defineConfig({
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      // Dev convenience: proxy API calls to the local Go server (default :8080).
      '/api': {
        target: process.env.VITE_DEV_API_PROXY ?? 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    target: 'es2022',
    sourcemap: true,
  },
  test: {
    environment: 'happy-dom',
    include: ['tests/**/*.test.ts', 'src/**/*.test.ts'],
    setupFiles: ['./tests/setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html'],
    },
  },
});
