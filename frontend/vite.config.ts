import { defineConfig, type ProxyOptions } from 'vite';
import solid from 'vite-plugin-solid';

// Everything here is served by the Go backend, not Vite.
const BACKEND_PREFIXES = ['/api', '/auth', '/media', '/tiles', '/webhooks'] as const;
const API_TARGET = 'http://127.0.0.1:8080';

const proxy: Record<string, string | ProxyOptions> = Object.fromEntries(
  BACKEND_PREFIXES.map((prefix) => [prefix, { target: API_TARGET, changeOrigin: true } satisfies ProxyOptions]),
);

export default defineConfig({
  plugins: [solid()],
  base: '/',
  // emptyOutDir is off because frontend/dist holds a committed .gitkeep that
  // the Go build's //go:embed needs: wiping the directory would break the next
  // `go build` until the web assets were built again. Stale hashed assets are
  // harmless, and .gitignore covers the directory's contents.
  build: { outDir: 'dist', sourcemap: false, emptyOutDir: false },
  server: { port: 5173, proxy },
  preview: { port: 4173, proxy },
});
