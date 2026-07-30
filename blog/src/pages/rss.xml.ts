import type { APIRoute } from 'astro';
import { getPublishedPosts, postUrl } from '../lib/posts';

export const prerender = true;

function escapeXml(value: string): string {
  return value.replace(/[<>&'\"]/g, (character) => {
    const entities: Record<string, string> = {
      '&': '&amp;',
      "'": '&apos;',
      '"': '&quot;',
      '<': '&lt;',
      '>': '&gt;',
    };
    return entities[character];
  });
}

export const GET: APIRoute = async () => {
  const posts = await getPublishedPosts();
  const site = 'https://blog.authproxy.net';
  const items = posts
    .map(
      (post) => `
      <item>
        <title>${escapeXml(post.data.title)}</title>
        <description>${escapeXml(post.data.description)}</description>
        <link>${site}${postUrl(post)}</link>
        <guid isPermaLink="true">${site}${postUrl(post)}</guid>
        <pubDate>${post.data.publishedAt.toUTCString()}</pubDate>
        <author>${escapeXml(post.data.author)}</author>
        ${post.data.tags.map((tag) => `<category>${escapeXml(tag)}</category>`).join('')}
      </item>`,
    )
    .join('');

  const body = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>AuthProxy Blog</title>
    <description>Practical guidance for building and operating secure third-party API integrations with AuthProxy.</description>
    <link>${site}/</link>
    <language>en-us</language>${items}
  </channel>
</rss>`;

  return new Response(body, {
    headers: { 'Content-Type': 'application/rss+xml; charset=utf-8' },
  });
};
