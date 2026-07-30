import { getCollection, type CollectionEntry } from 'astro:content';

export type Post = CollectionEntry<'posts'>;

export function getPostSlug(post: Post): string {
  return post.id.replace(/\/index$/, '');
}

export async function getPublishedPosts(): Promise<Post[]> {
  const posts = await getCollection('posts', ({ data }) =>
    import.meta.env.PROD ? !data.draft : true,
  );

  return posts.sort(
    (first, second) => second.data.publishedAt.valueOf() - first.data.publishedAt.valueOf(),
  );
}

export function postUrl(post: Post): string {
  return `/posts/${getPostSlug(post)}/`;
}
