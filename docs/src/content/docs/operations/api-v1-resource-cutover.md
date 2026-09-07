---
title: API v1 resource-contract cutover
description: Destructive pre-production migration guidance for the authproxy.net/v1alpha1 resource contract.
---

AuthProxy's `/api/v1` API now uses `authproxy.net/v1alpha1` Kubernetes-style
resources. This is a breaking change to HTTP API v1, not a new `/api/v2`.

AuthProxy was not deployed to production when this contract was introduced, so
the cutover deliberately has no compatibility window. Old request decoders,
response aliases, dual reads, and background conversion jobs are not included.

:::danger[Destroy old data-bearing environments]

Do not start this release against a demo or development environment containing
data written by the old contract. Destroy and recreate its AuthProxy databases,
Redis state, blob data, and Terraform-managed AuthProxy resources. Back up
anything you need for reference first; the old data is not importable through
the new API.

:::

## Why recreation is required

Connector provider definitions changed from the old serialized shape to
canonical `Connector.spec.definition`. The encrypted database column can store
the new representation, so no SQL schema migration is needed solely for this
change. That does not make existing ciphertext compatible: AuthProxy has no
legacy definition decoder and no task that rewrites encrypted connector rows.

Other persisted rows remain implementation models rather than public resource
documents, but the project validates this cutover only on fresh SQLite and
PostgreSQL databases. Treat recreation as one indivisible environment change.

## Recreate a local environment

For the repository Docker Compose environment, the teardown helper removes the
containers and named volumes:

```bash
./scripts/teardown-docker.sh
docker compose up -d
```

Then start AuthProxy with migrations enabled and let canonical configured
resources reconcile from `dev_config`:

```bash
go run ./cmd/server serve --auto-migrate --config=./dev_config/default.yaml all
```

For another SQLite or PostgreSQL setup, remove the disposable database and
associated Redis/blob state through that environment's normal teardown
mechanism, create empty stores, run the current database migrations, and start
all services from the same release. Do not manually copy connector rows from
the old database.

For Helm, Kustomize, or Terraform-managed demos, destroy the complete demo
stack—including persistent volumes or external disposable databases—then
recreate it from current manifests. A schema-migration Job cannot convert the
old resource contract.

## Update clients and configuration

Update every producer and consumer before recreating the environment:

| Old concept | Current contract |
|---|---|
| Flat managed-resource body | `apiVersion`, `kind`, `metadata`, `spec`, and response `status` |
| Top-level resource ID/name/namespace | `metadata.id`, `metadata.name`, `metadata.namespace` |
| Connector version DTO/kind | `kind: Connector` with `metadata.generation` |
| Bare list array or flat pagination | `<Kind>List` with `metadata` and `items` |
| Scalar reference | typed `ObjectReference` |
| Flat lifecycle body/result | typed action with target in `metadata`, input in `spec`, result in `status` |
| Label/annotation/key subroutes | parent-resource `PATCH` of `metadata` or `Namespace.spec.encryptionKeyRef` |
| Flat actor JWT claim | restricted `authproxy.net/v1alpha1` Actor resource |
| Flat configured connector/actor/rate limit | canonical resource manifest in `loadFromList` or actor configuration |

The JavaScript SDK exposes discriminated resource, list, action, and reference
types. Update property access such as `connection.id` to
`connection.metadata.id` and `connection.state` to
`connection.status.lifecycle.state`. Update create and patch calls to send
typed resource bodies.

Terraform remains idiomatic HCL; users do not write literal `apiVersion` and
`kind` blocks. The provider translates HCL into the resource contract. After
destroying an old AuthProxy environment, recreate its Terraform-managed
resources rather than assuming old remote IDs still exist.

JWT issuers must update atomically with AuthProxy. The top-level registered JWT
claims remain normal JWT fields, while `actor` uses the restricted Actor
resource. `sub` must equal `actor.spec.externalId`, and the top-level
`namespace` must equal `actor.metadata.namespace`.

## Protocols that remain unwrapped

Not every JSON document is a managed resource. Structured and streaming proxy
traffic represents third-party HTTP, metrics query results are analytical
payloads, and session/OAuth/setup callbacks are authentication protocols.
Health checks and Swagger assets also retain their protocol-native shapes.
Read-only AuthProxy observations such as RequestEvent, Notification, Task, and
WorkflowInstance use typed projections without becoming client-managed
resources.

## Cutover verification

Before treating an environment as migrated:

1. Confirm all configured resource entries decode as
   `authproxy.net/v1alpha1` manifests.
2. Create and list every managed resource kind and verify TypeMeta,
   metadata/spec/status placement, and list `items`.
3. Issue and validate both subject-only and embedded-Actor JWTs.
4. Exercise Connection setup, data-source lists, OAuth scopes, proxy traffic,
   connector lifecycle, generation migration, and RateLimit dry runs.
5. Run the Terraform plan/apply/read/update/import tests against the fresh API.
6. Regenerate and inspect Swagger, then run the repository preflight.

The route-by-route classification and implementation history are preserved in
the [Kubernetes-style resource API design
record](/development/design/kubernetes-resource-api/).
