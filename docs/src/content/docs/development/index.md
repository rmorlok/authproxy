---
title: Develop AuthProxy
description: Set up a development environment, understand the codebase, and evolve AuthProxy safely.
---

Use this section when you are changing AuthProxy itself rather than embedding or
operating an existing deployment.

- [Quick start](/development/quick-start/) — build the current checkout and start all
  services with Docker.
- [Local development](/development/local-development/) — run Go and frontend processes
  from source, sign into the local UIs, select optional services, and run tests.
- [Codebase layout](/development/codebase/) — understand service boundaries, packages,
  schemas, SDKs, and deployment assets.
- [CLI](/development/cli/) — configure and use `ap` for JWTs, signed API access, UI login,
  and connection-scoped proxying.
- [Workflow versioning](/development/workflows/) — evolve durable background workflows
  without breaking replay.
- [Design notes](/development/design/) — implementation proposals and known gaps; these are
  not stable product reference pages.

Before committing, run the repository preflight:

```bash
./scripts/preflight.sh
```

It regenerates Swagger artifacts and checks that the integration-test module is
consistent.
