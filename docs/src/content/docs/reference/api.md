---
title: API reference
---

AuthProxy exposes separate application-facing and administrative HTTP APIs.
Both publish generated Swagger 2.0 documentation when the service is running.

| API | Local Swagger UI | Checked-in JSON |
|---|---|---|
| Application API | `http://localhost:8081/swagger/index.html` | [`internal/service/api/swagger/docs.json`](https://github.com/rmorlok/authproxy/blob/main/internal/service/api/swagger/docs.json) |
| Admin API | `https://localhost:8082/swagger/index.html` | [`internal/service/admin_api/swagger/docs.json`](https://github.com/rmorlok/authproxy/blob/main/internal/service/admin_api/swagger/docs.json) |

The Admin URL uses the self-signed development certificate in a local checkout.
Production URLs depend on deployment routing.

## Which API to use

- Use the **application API** for host-application resource access and
  connection-scoped proxy requests.
- Use the **Admin API** for operator workflows and broader management
  endpoints.
- Use the **public service** for Marketplace sessions and OAuth browser
  callbacks; its browser-oriented routes are not a substitute for the
  application API.

Every protected request needs an actor JWT or a browser session with the
required namespace, resource, resource-ID, and verb scope. See
[authentication and authorization](/security/authentication-and-authorization/).

## Regenerate specifications

Swagger artifacts are generated from Go route annotations. Run:

```bash
./scripts/generate-swagger.sh
```

The repository preflight runs the same generation and fails if the checked-in
artifacts are stale:

```bash
./scripts/preflight.sh
```

For task-oriented request examples, start with
[proxying requests](/sdks/proxying/) and the
[JavaScript SDK](/sdks/javascript/).
