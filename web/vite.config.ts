/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import { fileURLToPath, URL } from 'node:url';
import { readFileSync } from 'node:fs';

// BuildOS Office Console — Vite config.
//
// The only required runtime config is VITE_API_BASE_URL (FRONTEND_ARCHITECTURE §6.1).
// Everything else (which features are on) is discovered at runtime from the server
// so a single build is deployable across forks. In production the console is served
// same-origin behind the Go server, so the default base is the relative "/".

// Build-time app version for the feedback widget's captureContext (it reads
// import.meta.env.VITE_APP_VERSION). Sourced from the environment when CI
// stamps one, falling back to the package.json version — without this define
// the env var is defined nowhere and context.app_version is always 'dev'.
const pkg = JSON.parse(
  readFileSync(fileURLToPath(new URL('./package.json', import.meta.url)), 'utf-8'),
) as { version: string };

export default defineConfig({
  define: {
    'import.meta.env.VITE_APP_VERSION': JSON.stringify(process.env.VITE_APP_VERSION ?? pkg.version),
  },
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
    // 'hidden' emits .map files WITHOUT the sourceMappingURL comment in
    // the bundles: local/CI builds keep maps for debugging, but the
    // production image deletes them (Dockerfile webbuilder stage) — the
    // maps embed the full original TypeScript (sourcesContent) and the
    // server would otherwise serve them unauthenticated under /assets/.
    sourcemap: 'hidden',
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
