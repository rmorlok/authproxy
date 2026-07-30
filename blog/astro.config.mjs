import { defineConfig } from 'astro/config';
import sitemap from '@astrojs/sitemap';
import astroMermaid from 'astro-mermaid';

export default defineConfig({
  site: 'https://blog.authproxy.net',
  output: 'static',
  trailingSlash: 'always',
  integrations: [
    sitemap(),
    astroMermaid({
      autoTheme: true,
      enableLog: false,
      mermaidConfig: {
        securityLevel: 'strict',
      },
    }),
  ],
});
