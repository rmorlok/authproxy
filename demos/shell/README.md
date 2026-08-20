# demos/shell — SSO stand-in host for the AuthProxy demo

AuthProxy is normally embedded in a host application that handles user
authentication. The demo environment needs a stand-in for that host —
something that guides a visitor to an AuthProxy surface. It mints a JWT
vouching for the chosen Marketplace user (or the fixed demo administrator)
before redirecting them into the appropriate AuthProxy UI.

**Never ship this to customers.** It only lives in the demo environment.

## Architecture

```
   ┌──────────────────────┐        POST /sso         ┌──────────────────────┐
   │  Frontend (Vite)     │  ─────────────────────►  │  Backend (Go)         │
   │  guided demo journey │                          │  - signs JWT          │
   │  radio-card choices  │                          │  - 303 redirect       │
   └──────────────────────┘                          └──────────┬───────────┘
                                                                │
                                                                ▼
                            ┌──────────────────────────────────────────────────┐
                            │  Marketplace or Admin UI                          │
                            │  picks up ?authToken=… → establishes session     │
                            └──────────────────────────────────────────────────┘
```

The backend holds the demo actor private key for pre-provisioned identities and
the host JWT private key for just-in-time provisioning. Selecting **Fresh user**
creates a random `fresh-user-<uuid>` external ID, includes the complete actor in
the JWT, and lets AuthProxy create that actor during session initiation.

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
systemAuth:
  actors:
    - externalId: demo-shell
      key:
        publicKey:
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
AUTHPROXY_JWT_PRIVATE_KEY_PATH=./dev_config/keys/system \
AUTHPROXY_ADMIN_UI_URL=http://localhost:5174 \
AUTHPROXY_MARKETPLACE_URL=http://localhost:5173 \
AUTHPROXY_AUTH_URL=http://localhost:8080 \
AUTHPROXY_GRAFANA_URL=http://localhost:3000 \
DEV_FRONTEND_URL=http://localhost:5175 \
go run ./demos/shell/backend
# → listens on http://localhost:8888
```

`DEV_FRONTEND_URL` makes the backend's `GET /` redirect to the vite dev
server so HMR works. Leave it unset in production — the backend serves
the embedded build at the same root.

### 4. Drive the flow

Open <http://localhost:8888>. Choose **Admin UI** → submit → you're redirected
to the AuthProxy admin UI as `demo-admin` with a fresh session. Choose
**Integration Marketplace**, then **Fresh user** → submit → an empty
Marketplace opens with no connections under a newly generated actor external
ID. When configured, the shell also links directly to Grafana's telemetry
views and the demo OAuth provider.

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
| `ADMIN_USERNAME`          | ✅        | externalId of the admin actor whose key is mounted at `ADMIN_PRIVATE_KEY_PATH` |
| `ADMIN_PRIVATE_KEY_PATH`  | ✅        | File path; PEM RSA or EC                                                    |
| `AUTHPROXY_JWT_PRIVATE_KEY_PATH` | ✅ | Host JWT private key used to provision each randomly generated fresh actor |
| `AUTHPROXY_ADMIN_UI_URL`  | ✅        | Base URL of the admin SPA                                                   |
| `AUTHPROXY_MARKETPLACE_URL` | ✅      | Base URL of the marketplace SPA                                             |
| `AUTHPROXY_AUTH_URL`      | ⛔        | Optional today; kept for future routes that call back into AuthProxy        |
| `AUTHPROXY_GRAFANA_URL`   | ⛔        | Optional Grafana base URL; enables telemetry links in the shell             |
| `AUTHPROXY_GRAFANA_APP_METRICS_URL` | ⛔ | Optional override for the app-metrics dashboard link                        |
| `AUTHPROXY_GRAFANA_EXPLORE_URL` | ⛔   | Optional override for the Grafana Explore link                              |
| `AUTHPROXY_GO_OAUTH2_SERVER_URL` | ⛔ | Optional direct URL for the disposable OAuth provider; enables its shell card |
| `DEV_FRONTEND_URL`        | ⛔        | If set, `GET /` redirects here instead of serving the embedded build        |
| `PORT`                    | ⛔        | Default `8888`                                                              |

The frontend reads `/config.json` from the backend at runtime. When
`AUTHPROXY_GRAFANA_URL` is set, the backend returns links to Grafana,
the provisioned AuthProxy App Metrics dashboard, and Explore. When
`AUTHPROXY_GO_OAUTH2_SERVER_URL` is set, it enables the disposable OAuth
provider card. In Vite dev mode, `/config.json` and `/sso` are proxied to the
backend; override `AUTHPROXY_DEMO_SHELL_BACKEND_URL` if the backend is not on
`http://localhost:8888`.

## Why two signing paths

The pre-provisioned identities use actor-signed JWTs and the shared demo actor
key. A random fresh actor cannot have a public key provisioned ahead of time,
so that path uses the host JWT key and includes a complete actor claim. This is
the just-in-time provisioning pattern described in the host application
integration guide.

## Why this lives in `demos/`, not `cmd/` or `internal/`

Top-level `demos/` lets future demo wrappers (each their own stand-in
host) live next to this one without polluting the main service tree.
Anything under `demos/` is allowed to import from `internal/` but the
reverse is forbidden — main code never depends on demo code.
