---
title: Map users and tenants
description: Decide how your users, tenants, and installed integrations correspond to AuthProxy actors, namespaces, and connections.
---

Your application remains the source of truth for users, tenants, roles, and
which integrations are installed. AuthProxy needs only enough of that
information to decide who may use which connection. This page explains how to
choose that mapping.

## The mapping at a glance

| In your application | In AuthProxy | Advice |
|---|---|---|
| A user or service account | An actor, identified by its external ID | Use your immutable primary key, not an email or display name. |
| A tenant or organization | A namespace, such as `root.tenants.acme_42` | Derive the segment from a stable ID. Do not use a name that can change. |
| A team | A child namespace, or just a label | Use a namespace only if the team is an access boundary. |
| An installed integration | A connection | Store the `cxn_...` ID in your database. |
| Your own IDs and tags | Labels | Keep them short and non-sensitive. They appear in request logs. |

For example, user `usr_7` in tenant `acme_42` might become:

```text
actor namespace:  root.tenants.acme_42
external ID:      usr_7
```

Namespace segments may contain letters, digits, `_`, and `-`, and cannot start
with `-`. If your tenant IDs contain other characters, derive a stable,
namespace-safe key from them and use that consistently.

## Shared or private connections

The most important decision is who shares a connection, because it decides
which namespace each connection is created in.

**Tenant-shared.** One authorization covers a whole customer, such as a
company-wide Salesforce or Slack installation. Create the connection in the
tenant namespace and give every actor in the tenant access to it:

```text
root.tenants.acme_42
├── actors: usr_7, usr_9
└── connections: Salesforce, Slack
```

**Per-user.** Each person connects their own account, such as Google Drive or
a personal calendar. Create each connection in a namespace for that user, and
give each actor access only to their own:

```text
root.tenants.acme_42
└── users
    ├── usr_7
    │   └── connection: Google Drive
    └── usr_9
        └── connection: Google Drive
```

Most products use both, depending on the provider. A connector can live in a
shared parent namespace, such as `root`, while its connections are created in
tenant or user namespaces below it.

Connections are not owned by actors. They belong to namespaces, and actors get
access through permissions on those namespaces. When a user leaves, nothing
happens to "their" connections automatically. Your application has to decide
whether to disconnect them.

## Provision actors

An actor must exist before it can do anything. There are two ways to create
actors.

**Ahead of time.** Your backend creates and updates actors through the API
whenever users join, change role, or leave. Tokens then identify the actor by
`sub` and `namespace` only. A token for an actor that does not exist is
rejected. This keeps all permission changes in one place that you can audit.

**On first use.** Your backend includes the complete actor definition in each
token. AuthProxy creates or updates the actor when it sees the token. This
saves you a sync job, but it means the code that signs tokens is also the
code that decides permissions.

The token format for both is described in [Authenticate requests to
AuthProxy](/integration/authentication/).

Do not copy your whole user profile into the actor. It needs the stable ID,
its namespace, its permissions, and perhaps a few labels.

## Grant the least access needed

A typical tenant member who can browse connectors and manage their tenant's
connections might have:

```json
[
  {
    "namespace": "root.**",
    "resources": ["connectors"],
    "verbs": ["list", "get"]
  },
  {
    "namespace": "root.tenants.acme_42.**",
    "resources": ["connections"],
    "verbs": ["create", "list", "get", "update", "disconnect", "proxy"]
  }
]
```

`root.tenants.acme_42.**` matches that namespace and everything below it.
Connector and connection permissions are listed separately so that connectors
can live in a shared namespace while connections stay restricted to the
tenant.

Remove any verbs your product does not need. Keep administrative actions,
such as publishing connectors or managing actors and namespaces, on a
separate operator identity rather than on end users.

If many actors follow the same pattern, a permission can refer to the actor's
own data, such as `root.tenants.{{labels.tenant_id}}.**`. AuthProxy fills in
the value from the actor's labels when it checks the permission. If you use
this, make sure users cannot set that label themselves. See [Authentication
and authorization](/security/authentication-and-authorization/#labels-and-permission-templates).

## Keep track of installations

You will need to find a connection again, for example when a sync job runs.
There are three ways to do this, and using all three is reasonable:

- **Store the connection ID** on your own "installed integration" record.
  This is the fastest lookup, and it is what the proxy endpoints take.
- **Give the connection a name**, such as `production-crm`, when you create
  it. Names are readable in the Admin UI and can be looked up within a
  namespace. They can be renamed, so do not use them as your only reference.
- **Add a label**, such as `acme.com/installation-id=ins_123`, and search with
  a label selector. This is useful when reconciling your records against
  AuthProxy.

Labels help you find things. They never grant access. See [Labels and
annotations](/concepts/labels-and-annotations/).

## When users and tenants change

When someone's role changes, update the actor's permissions before you issue
their next token. When someone leaves:

1. stop signing tokens for them and remove the actor's permissions;
2. disconnect connections that were private to them;
3. leave tenant-shared connections alone if other users still rely on them;
   and
4. keep or delete their request history according to your data policy.

## Next steps

- [Authenticate requests to AuthProxy](/integration/authentication/)
- [Embed the Marketplace](/integration/marketplace/)
- [The core resource model](/concepts/core-model/) has the exact rules for
  namespaces, permissions, and names.
