# Documentation Agent Guide

This guide applies to AuthProxy's public documentation and to changes that move
content between the repository landing page and the documentation site. The
site is built with Astro Starlight and published at
[docs.authproxy.net](https://docs.authproxy.net).

## Documentation Surfaces

- `README.md` is the repository landing page. It is not the full manual.
- `docs/src/content/docs/` is the source of the public documentation website.
- `docs/README.md` explains how to work on the documentation project; it is not
  a second public home page.
- Package-level READMEs stay beside code when they explain implementation or
  contributor details that are useful only while working in that package.
- `docs/screenshots/` holds review artifacts referenced by pull requests. Put
  images published on the documentation site beside their content under
  `docs/src/content/docs/` and reference them with relative paths.

Do not maintain the same explanation in multiple places. Choose one canonical
page and link to it from other pages.

## Root README Contract

Keep the substantive body of the root `README.md` focused on three jobs:

1. Explain what AuthProxy is, when someone would use it, and how its native-API,
   self-hosted approach differs from unified APIs, hosted credential brokers,
   and workflow products. State what AuthProxy does not replace.
2. Show the product demos. Include the Marketplace and Admin UI GIFs, current
   demo links, the host-to-AuthProxy SSO handoff, Grafana availability, and the
   fake `go-oauth2-server` accounts that let visitors test OAuth without a real
   third-party identity.
3. Give the shortest reliable developer quick start: clone the repository,
   start the full stack, verify it, and stop it. Link to the development docs
   for every optional or alternate setup.

Brief project metadata such as the license may remain. Move detailed concepts,
configuration, deployment variants, operational procedures, API usage, and
security analysis into the documentation site.

## Write for a Specific Reader

Every page should have one primary reader and one outcome. Route content by the
question that reader is trying to answer:

| Reader | Typical question | Primary sections |
|---|---|---|
| Application engineer | How do I add and call an integration? | `concepts/`, `integration/`, `sdks/` |
| DevOps or IT operator | How do I deploy, configure, monitor, and upgrade it? | `deployment/`, `operations/` |
| Security reviewer | What are the trust boundaries, permissions, secret controls, and data risks? | `security/` |
| Engineering decision maker | What does AuthProxy own, and should we build or adopt it? | documentation home, `concepts/`, `reference/related-projects/` |
| AuthProxy contributor | How do I run, test, and change the codebase? | `development/` |

When several readers need the same prerequisite, explain it once in `concepts/`
and link to it from their task pages. For example, define actors and namespaces
in the core model, then show their host-application mapping in the integration
guide and their authorization consequences in the security guide.

## Information Architecture

Use the existing sections consistently:

| Section | Content that belongs there |
|---|---|
| `getting-started/` | Hosted demo and first evaluation paths |
| `concepts/` | Stable vocabulary and mental models: namespaces, actors, connectors, connections, and ownership |
| `integration/` | Embedding AuthProxy, mapping host entities, connector behavior, setup flows, and Marketplace integration |
| `sdks/` | Making requests through AuthProxy with SDKs, the proxy API, and request examples |
| `deployment/` | Installing and packaging with Helm, Kustomize, containers, registries, and infrastructure prerequisites |
| `operations/` | Day-2 monitoring, telemetry, storage, tasks, limits, migrations, lifecycle actions, backup, and upgrades |
| `security/` | Threat boundaries, authentication, authorization, encryption, sensitive data, auditability, and review guidance |
| `development/` | Contributor quick starts, local setup variants, tests, workflows, code layout, and design notes |
| `reference/` | API/configuration entry points and factual comparative material |

Put proposals and historical architecture decisions under `development/design/`
and label their status explicitly. Do not present an unimplemented design as
current product behavior.

## Authoring Style

- Be direct and concise. Start with the result the reader will achieve, then
  give prerequisites and the shortest working path.
- Prefer a small, complete example over an exhaustive option list. Explain what
  the important values mean immediately after the example.
- Use AuthProxy's terms consistently. Define an actor, namespace, connector, or
  connection before relying on the term, and avoid implying that host entities
  must map one-to-one when the mapping is application-defined.
- Separate integration responsibilities clearly: identify what the host
  application does, what AuthProxy does, and what the third-party provider does.
- Call out security-sensitive steps and link to the relevant security page
  instead of burying trust assumptions in setup instructions.
- Link to the canonical reference for exhaustive configuration or API fields;
  do not copy a long reference table into a task guide.
- Use examples with fake identifiers and credentials. Never suggest entering
  real credentials into the shared demo.
- Avoid claims based on connector counts, benchmarks, or current hosted-demo
  state unless the claim is verified and the page states when or how.

## Starlight Conventions

- Public pages belong in `docs/src/content/docs/` and require a `title` in
  frontmatter. Add a concise `description` to landing pages and task guides.
- Use `.md` for ordinary pages and `.mdx` only when Starlight components or
  other JSX are necessary.
- Use site routes for internal links, such as `/concepts/core-model/`, rather
  than filesystem-relative links to Markdown files.
- Use absolute GitHub URLs when a public page must link to source files outside
  the documentation site.
- Update `docs/astro.config.mjs` when a public page is added, moved, renamed, or
  removed so the sidebar continues to reflect the information architecture.
- Keep headings descriptive and shallow. A reader should be able to scan the
  table of contents and understand the task flow.

Use a Mermaid diagram when it materially clarifies a boundary, mapping, or
workflow involving several participants or steps. Keep the diagram smaller
than the explanation it replaces, quote labels containing punctuation, and
follow it with prose that explains the important consequence. The site uses
Mermaid's strict security mode, so build the site to catch unsupported markup.
Do not add decorative diagrams to simple one-step instructions.

## Verification

For every public documentation change:

1. Check commands, routes, names, and links against the current code or deployed
   environment as appropriate.
2. Run `yarn docs:build` from the repository root. The build validates the
   Starlight content schema and compiles MDX and Mermaid diagrams.
3. Run the repository-required `./scripts/preflight.sh` before committing.

Use `yarn docs:dev` for local authoring and `yarn docs:preview` to inspect the
production build when layout, navigation, images, or responsive behavior
changed.
