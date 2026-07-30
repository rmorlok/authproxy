import type { APIRoute } from 'astro';

export const prerender = true;

export const GET: APIRoute = () =>
  new Response('User-agent: *\nAllow: /\n\nSitemap: https://blog.authproxy.net/sitemap-index.xml\n', {
    headers: { 'Content-Type': 'text/plain; charset=utf-8' },
  });
