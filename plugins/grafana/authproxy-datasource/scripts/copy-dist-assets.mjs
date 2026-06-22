import { copyFileSync, cpSync, existsSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));
const pluginRoot = join(root, '..');
const dist = join(pluginRoot, 'dist');

mkdirSync(dist, { recursive: true });

for (const file of ['plugin.json', 'README.md']) {
  const source = join(pluginRoot, file);
  if (existsSync(source)) {
    copyFileSync(source, join(dist, file));
  }
}

const imageDir = join(pluginRoot, 'img');
if (existsSync(imageDir)) {
  cpSync(imageDir, join(dist, 'img'), { recursive: true });
}
