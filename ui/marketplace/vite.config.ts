import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const monorepoRoot = resolve(__dirname, '../..');

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
  // Per-package .env.* files are auto-loaded by Vite as defaults that travel
  // with the repo. The monorepo-root .env is gitignored and is where each
  // local clone (e.g. authproxy1, authproxy2, authproxy3 on one machine)
  // sets its own port slot. Vite's own env-loading lets process.env beat
  // .env files, so seeding process.env from the root .env up front makes
  // root values override the per-package defaults without changing envDir.
  const rootEnv = loadEnv(mode, monorepoRoot, '');
  for (const [k, v] of Object.entries(rootEnv)) {
    process.env[k] = v;
  }

  const port = Number(process.env.AUTHPROXY_MARKETPLACE_UI_PORT) || 5173;

  return {
    plugins: [react()],
    // strictPort makes a port collision fail loudly instead of silently
    // shifting onto another install's slot — see the multi-clone setup above.
    server: {
      port,
      strictPort: true,
    },
    resolve: {
      alias: [
        { find: '@authproxy/api', replacement: resolve(__dirname, '../../sdks/js/src') },
      ],
    },
    test: {
      environment: 'jsdom',
      globals: true,
      setupFiles: ['./src/tests/setup.ts'],
      css: true,
    },
  };
});
