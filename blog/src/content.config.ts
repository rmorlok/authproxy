import { defineCollection } from 'astro:content';
import { glob } from 'astro/loaders';
import { z } from 'astro/zod';

const tag = z.string().regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, {
  message: 'Tags must be lowercase URL-safe slugs such as "connector-design".',
});

const posts = defineCollection({
  loader: glob({
    base: './src/content/posts',
    pattern: '**/index.md',
  }),
  schema: ({ image }) =>
    z
      .object({
        title: z.string().min(1),
        description: z.string().min(1).max(160),
        publishedAt: z.coerce.date(),
        updatedAt: z.coerce.date().optional(),
        author: z.string().min(1),
        tags: z.array(tag).min(1),
        draft: z.boolean().default(false),
        cover: image().optional(),
        coverAlt: z.string().min(1).optional(),
      })
      .superRefine((post, context) => {
        if (post.cover && !post.coverAlt) {
          context.addIssue({
            code: z.ZodIssueCode.custom,
            message: 'coverAlt is required when cover is provided.',
            path: ['coverAlt'],
          });
        }
      }),
});

export const collections = { posts };
