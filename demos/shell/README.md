# demos/shell — SSO stand-in host for the AuthProxy demo

AuthProxy is normally embedded in a host application that handles user
authentication. The demo environment needs a stand-in for that host —
something with a "pick your demo identity" dropdown that mints a JWT
vouching for the selected user and redirects them into the AuthProxy
marketplace or admin UI.

**Never ship this to customers.** It only lives in the demo environment.

## Architecture

```
   ┌──────────────────────┐        POST /sso         ┌──────────────────────┐
   │  Frontend (Vite)     │  ─────────────────────►  │  Backend (Go)         │
   │  actor + destination │                          │  - signs JWT          │
   │  dropdowns           │                          │  - 303 redirect       │
   └──────────────────────┘                          └──────────┬───────────┘
                                                                │
                                                                ▼
                            ┌──────────────────────────────────────────────────┐
                            │  Marketplace or Admin UI                          │
                            │  picks up ?auth_token=… → establishes session     │
                            └──────────────────────────────────────────────────┘
```

The backend holds **one** admin private key. AuthProxy's auth model
trusts an admin-signed JWT to bear any `actor_external_id` claim — so
one key is enough to "be" any of the three demo identities.

## Running locally

Pre-requisites:
- AuthProxy server running locally (`go run ./cmd/server serve --config=./dev_config/default.yaml all`)
- A configured admin actor in your AuthProxy config that the demo shell can use as its signing identity
- Node + Yarn pinned via Volta (see root `package.json`)

### 1. Generate the demo admin keypair (one-time, local)

```bash
mkdir -p demos/shell/dev_keys
openssl genrsa -out demos/shell/dev_keys/demo-shell 2048
openssl rsa -in demos/shell/dev_keys/demo-shell -pubout -out demos/shell/dev_keys/demo-shell.pub
```

Add the public key as an admin actor in `dev_config/default.yaml`:

```yaml
system_auth:
  actors:
    - external_id: demo-shell
      key:
        public_key:
          path: ./demos/shell/dev_keys/demo-shell.pub
      permissions:
        - namespace: "root.**"
          resources: ["*"]
          verbs: ["*"]
```

Restart the AuthProxy server so it picks up the new actor.

### 2. Start the demo-shell frontend (vite dev for HMR)

```bash
yarn install                          # picks up demos/shell/frontend as @authproxy/demo-shell
yarn workspace @authproxy/demo-shell dev
# → listens on http://localhost:5175
```

### 3. Start the demo-shell backend

```bash
ADMIN_USERNAME=demo-shell \
ADMIN_PRIVATE_KEY_PATH=./demos/shell/dev_keys/demo-shell \
AUTHPROXY_ADMIN_UI_URL=http://localhost:5174 \
AUTHPROXY_MARKETPLACE_URL=http://localhost:5173 \
AUTHPROXY_AUTH_URL=http://localhost:8080 \
DEV_FRONTEND_URL=http://localhost:5175 \
go run ./demos/shell/backend
# → listens on http://localhost:8888
```

`DEV_FRONTEND_URL` makes the backend's `GET /` redirect to the vite dev
server so HMR works. Leave it unset in production — the backend serves
the embedded build at the same root.

### 4. Drive the flow

Open <http://localhost:8888>. Pick `demo-admin` + Admin UI → submit →
you're redirected to the AuthProxy admin UI as `demo-admin` with a fresh
session. Pick `fresh-user` + Marketplace → empty marketplace, no
connections.

## Local smoke via docker-compose

Self-contained recipe that pulls `authproxy` + `authproxy-demo-shell`
from GHCR — no local Go / Node toolchain required.

```bash
cd demos/shell/compose
./init-keys.sh         # one-time keypair generation into ./keys/
docker compose up
open http://localhost:8888
```

The recipe wires:
- `authproxy:main` (postgres + redis + AuthProxy) on ports 8080/8081/8082
- `authproxy-demo-shell:main` on port 8888, mounting the generated
  private key + pointing at the host-mapped AuthProxy URLs

`./keys/demo-shell.pub` is bind-mounted into AuthProxy's actors directory
so `dev_config/docker.yaml`'s `keys_path` picks it up and registers
`demo-shell` as an admin actor. The smoke recipe lives entirely under
`demos/shell/compose/` — pin to a non-`:main` image via
`IMAGE_TAG=pr-NNN docker compose up` when testing branches.

## Configuration reference

The backend reads the following env vars:

| Var                       | Required | Notes                                                                       |
|---------------------------|----------|-----------------------------------------------------------------------------|
| `ADMIN_USERNAME`          | ✅        | external_id of the admin actor whose key is mounted at `ADMIN_PRIVATE_KEY_PATH` |
| `ADMIN_PRIVATE_KEY_PATH`  | ✅        | File path; PEM RSA or EC                                                    |
| `AUTHPROXY_ADMIN_UI_URL`  | ✅        | Base URL of the admin SPA                                                   |
| `AUTHPROXY_MARKETPLACE_URL` | ✅      | Base URL of the marketplace SPA                                             |
| `AUTHPROXY_AUTH_URL`      | ⛔        | Optional today; kept for future routes that call back into AuthProxy        |
| `DEV_FRONTEND_URL`        | ⛔        | If set, `GET /` redirects here instead of serving the embedded build        |
| `PORT`                    | ⛔        | Default `8888`                                                              |

## Why one signing key, not three

It's tempting to give the demo shell one keypair per demo identity. The
existing AuthProxy auth model already covers the multi-identity case via
admin-actor vouching: an admin's JWT can bear any `actor_external_id`
claim and AuthProxy will trust the user assignment. That's the same
pattern `cmd/cli/sign-marketplace-login-url` uses, so the demo shell
mirrors it.

## Why this lives in `demos/`, not `cmd/` or `internal/`

Top-level `demos/` lets future demo wrappers (each their own stand-in
host) live next to this one without polluting the main service tree.
Anything under `demos/` is allowed to import from `internal/` but the
reverse is forbidden — main code never depends on demo code.
