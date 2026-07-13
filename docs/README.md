# AuthProxy documentation source

This directory is the Starlight project that builds
[docs.authproxy.net](https://docs.authproxy.net). Public pages live in
[`src/content/docs/`](src/content/docs/); this README is source-only and is not
published as a second home page.

## Run the site locally

From the repository root:

```bash
yarn install
yarn docs:dev
```

Build and preview the production output with:

```bash
yarn docs:build
yarn docs:preview
```

## Information architecture

| Section | Audience and purpose |
|---|---|
| [`getting-started/`](src/content/docs/getting-started/) | Demo and first-run paths for every audience |
| [`concepts/`](src/content/docs/concepts/) | Stable product vocabulary and mental models |
| [`integration/`](src/content/docs/integration/) | Host application and connector-authoring guides |
| [`sdks/`](src/content/docs/sdks/) | Proxy request patterns, client SDKs, and API use |
| [`deployment/`](src/content/docs/deployment/) | Installation, packaging, and infrastructure |
| [`operations/`](src/content/docs/operations/) | Day-2 behavior, monitoring, and lifecycle actions |
| [`security/`](src/content/docs/security/) | Trust, authorization, encryption, and review guidance |
| [`development/`](src/content/docs/development/) | Local development, testing, and codebase internals |
| [`reference/`](src/content/docs/reference/) | Generated-reference entry points and comparative material |

Package-level READMEs remain next to code when they explain implementation
details. Public explanations belong in the Starlight content collection and
should link to source code only when a reader needs contributor-level detail.

## Authoring conventions

- Add the required `title` and, for landing and task pages, a concise
  `description` in page frontmatter.
- Give each page one audience and one outcome.
- Lead with the shortest working example, then explain it.
- Use website routes such as `/concepts/core-model/` for links between pages.
- Use absolute GitHub URLs for repository files outside the documentation site.
- Keep images beside the content they support and use relative image paths.
- Use fenced `mermaid` blocks for diagrams; the site renders them in light and
  dark themes.
- Distinguish shipped behavior from proposals and historical design notes.
- Avoid claims that depend on a hand-maintained connector count, benchmark, or
  hosted-demo state unless the page says how and when it was verified.

Navigation is defined in [`astro.config.mjs`](astro.config.mjs). When adding or
moving a public page, update the sidebar and run `yarn docs:build` before
submitting the change.
