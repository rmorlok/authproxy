---
title: "Tutorial: your first connection"
description: Run AuthProxy locally, define an API-key connector, create a connection, and send a request through it.
---

In this tutorial you will run AuthProxy on your machine, define a connector
for a test API, connect it with a fake API key, and send a request through the
connection. At the end you will see the third-party API receive a credential
that your request never contained.

The tutorial uses [httpbin.org](https://httpbin.org), a public service that
echoes back the request it receives. This makes it easy to see exactly what
AuthProxy forwarded. Do not use a real API key.

It takes about 15 minutes, most of it waiting for the first Docker build.

## Before you begin

You need:

- [Git](https://git-scm.com/) and [Docker](https://www.docker.com/products/docker-desktop/) with Compose
- [Go](https://go.dev/dl/), to run the `ap` command-line tool from source
- `curl`, and `jq` to read IDs out of JSON responses

## 1. Start AuthProxy

Clone the repository and start the full stack:

```bash
git clone https://github.com/rmorlok/authproxy.git
cd authproxy
docker compose --profile server up --build -d
```

The first run builds AuthProxy and starts it with PostgreSQL, Redis, and its
other dependencies. When it is ready, the API answers on port 8081:

```bash
curl http://localhost:8081/ping
```

```json
{"message":"pong","service":"api"}
```

If the build fails or a port is already in use, see the [development quick
start](/development/quick-start/).

## 2. Configure the `ap` CLI

The `ap` CLI signs requests to AuthProxy for you. The local stack trusts a
development administrator named `bobdole` whose key pair is checked into the
repository, so you can sign requests as that actor.

Create `~/.authproxy.yaml`, replacing the path with your clone's location:

```yaml
adminUsername: bobdole
adminPrivateKeyPath: /path/to/authproxy/dev_config/keys/admin/bobdole
server:
  api: http://localhost:8081
```

Then define a shell alias so the rest of the commands are shorter. Run it from
the repository root:

```bash
alias ap='go run ./cmd/cli'
```

## 3. Define a connector

A connector tells AuthProxy how to authenticate to one provider. Save this as
`tutorial.yaml`. It creates a `root.tutorial` namespace and a connector inside
it:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Namespace
metadata:
  name: tutorial
  namespace: root
---
apiVersion: authproxy.net/v1alpha1
kind: Connector
metadata:
  name: httpbin
  namespace: root.tutorial
spec:
  definition:
    displayName: httpbin
    description: An echo API that shows the credential AuthProxy adds.
    auth:
      type: api-key
      placement:
        type: bearer
  release:
    desiredState: primary
```

The important parts:

- `auth.type: api-key` means users connect by entering an API key.
- `placement.type: bearer` tells AuthProxy to send that key as
  `Authorization: Bearer <key>` on every proxied request. Other placements
  send it in a custom header, a query parameter, or HTTP basic auth.
- `release.desiredState: primary` publishes the connector so new connections
  can use it. Without it, the connector is saved as a `draft` and cannot be
  connected yet.

Apply it:

```bash
ap apply -f tutorial.yaml
```

```text
namespace/root.tutorial created
connector/cxr_tQxdfv3gfRHu9hI8 created
```

Your connector ID will differ.

## 4. Create a connection

In a real product, your users create connections by clicking **Connect** in
the Marketplace UI. Here you will make the same calls the Marketplace makes,
so you can see each step.

First, get a short-lived token for the API service:

```bash
TOKEN=$(ap sign-jwt --apis api --expires-in 1h)
```

Start setup for a new connection. The request names the connector by its
namespace and name, and gives the connection a human-readable name:

```bash
curl -s -X POST http://localhost:8081/api/v1/connections/_initiate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "authproxy.net/v1alpha1",
    "kind": "ConnectionInitiate",
    "metadata": {
      "target": {
        "apiVersion": "authproxy.net/v1alpha1",
        "kind": "Connector",
        "namespace": "root.tutorial",
        "name": "httpbin"
      }
    },
    "spec": {
      "name": "my-httpbin",
      "returnToUrl": "http://localhost:3000/integrations"
    }
  }' | tee initiate.json | jq
```

AuthProxy creates the connection in the `setup` state and replies with the
first setup step. For an API-key connector, that step is a form asking for the
key. The response includes the form's JSON Schema, which the Marketplace uses
to render it:

```json
{
  "kind": "ConnectionSetup",
  "metadata": {
    "target": { "kind": "Connection", "id": "cxn_Z3tMSSzTDa8piyM1" }
  },
  "status": {
    "type": "form",
    "stepId": "_authproxy_api_key_credentials",
    "stepTitle": "Enter your API key",
    "jsonSchema": {
      "type": "object",
      "required": ["api_key"],
      "properties": { "api_key": { "type": "string", "title": "API Key" } }
    }
  }
}
```

The response is abbreviated here. Save the connection ID:

```bash
CONNECTION_ID=$(jq -r .metadata.target.id initiate.json)
```

Now submit the form with a fake key, as a user would:

```bash
curl -s -X POST "http://localhost:8081/api/v1/connections/$CONNECTION_ID/_submit" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "authproxy.net/v1alpha1",
    "kind": "ConnectionSetupSubmit",
    "metadata": {
      "target": {
        "apiVersion": "authproxy.net/v1alpha1",
        "kind": "Connection",
        "id": "'"$CONNECTION_ID"'"
      }
    },
    "spec": {
      "stepId": "_authproxy_api_key_credentials",
      "data": { "api_key": "not-a-real-key" }
    }
  }' | jq .status
```

```json
{ "type": "complete" }
```

AuthProxy encrypted the key and stored it on the connection. Check the
connection's status:

```bash
curl -s "http://localhost:8081/api/v1/connections/$CONNECTION_ID" \
  -H "Authorization: Bearer $TOKEN" | jq '.status.lifecycle, .status.health'
```

```json
{ "state": "configured" }
{ "state": "healthy" }
```

## 5. Send a request through the connection

Ask AuthProxy to call httpbin's `/headers` endpoint, which returns the headers
it received. Notice that this request contains no httpbin credential. The only
`Authorization` header is your AuthProxy token, and AuthProxy does not forward
it.

```bash
curl -s -X POST "http://localhost:8081/api/v1/connections/$CONNECTION_ID/_proxy" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{ "url": "https://httpbin.org/headers", "method": "GET" }' | jq
```

```json
{
  "statusCode": 200,
  "headers": { "Content-Type": "application/json" },
  "bodyJson": {
    "headers": {
      "Authorization": "Bearer not-a-real-key",
      "Host": "httpbin.org"
    }
  }
}
```

httpbin received `Authorization: Bearer not-a-real-key`. AuthProxy looked up
the connection, decrypted its key, and added it according to the connector's
`placement`. The outer response wraps httpbin's status code, headers, and
body.

You can do the same thing with the CLI, which uses the streaming proxy
endpoint and passes the response through unchanged:

```bash
ap proxy --connection "$CONNECTION_ID" curl -s https://httpbin.org/headers
```

## 6. See what AuthProxy recorded

Each proxied request produces a request event. List the most recent one:

```bash
curl -s "http://localhost:8081/api/v1/metrics/request-events?limit=1" \
  -H "Authorization: Bearer $TOKEN" | jq '.items[0].metadata.labels'
```

The event's labels identify the connection, connector, namespace, and actor
involved, which lets you filter request history by any of them. See
[Application metrics](/operations/app-metrics/) for what else is recorded.

## Clean up

Stop the stack and keep its data:

```bash
docker compose --profile server down
```

Or remove the containers and all data volumes:

```bash
./scripts/teardown-docker.sh
```

## What changes in a real integration

This tutorial took two shortcuts that a production integration does not:

- **You signed tokens as a development administrator.** In production, your
  backend signs tokens for each of your users with a key AuthProxy trusts,
  and each user's permissions are limited to their own namespace. See
  [Authenticate requests to AuthProxy](/integration/authentication/).
- **You created the connection with raw API calls.** Your users would click
  **Connect** in the [Marketplace](/integration/marketplace/), which runs
  the same steps and also handles OAuth redirects.

Next, read [Plan your integration](/integration/) for the decisions you will
make when adding AuthProxy to your product, or [Define a
connector](/integration/connectors/) to set up an OAuth provider.
