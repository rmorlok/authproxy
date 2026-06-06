import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const monorepoRoot = resolve(__dirname, '../../..');

export default defineConfig(({ mode }) => {
  // Same root-env override pattern as ui/marketplace — lets multi-clone
  // setups (authproxy1, authproxy2, …) override port slots from one place.
  const rootEnv = loadEnv(mode, monorepoRoot, '');
  for (const [k, v] of Object.entries(rootEnv)) {
    process.env[k] = v;
  }

  const port = Number(process.env.AUTHPROXY_DEMO_SHELL_UI_PORT) || 5175;
  const base = process.env.VITE_BASE_URL || '/';
  const backendUrl = process.env.AUTHPROXY_DEMO_SHELL_BACKEND_URL || 'http://localhost:8888';

  return {
    base,
    plugins: [react()],
    server: {
      port,
      strictPort: true,
      proxy: {
        '/config.json': backendUrl,
        '/sso': backendUrl,
      },
    },
    build: {
      // Output into ../backend/embed/dist so the Go //go:embed directive in
      // demos/shell/backend/embed/embed.go can pick the build up at compile
      // time without copying files around.
      outDir: '../backend/embed/dist',
      emptyOutDir: false,
    },
  };
});
