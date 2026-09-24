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
  build: { outDir: 'dist', sourcemap: false },
  server: { port: 5173, proxy },
  preview: { port: 4173, proxy },
});
