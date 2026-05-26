# Multi-stage build for the demos/shell SSO host stand-in.
#
# Built from the repo root (`docker build -f demos/shell/Dockerfile .`)
# because the Go backend imports `internal/apauth/jwt` and the frontend
# lives inside the root Yarn workspace.
#
# Stages:
#   1. ui-builder      — Node, builds demos/shell/frontend into
#                        demos/shell/backend/embed/dist
#   2. backend-builder — Go, embeds the dist tree, compiles the binary
#   3. runtime         — debian-slim with the binary only

# Stage 1: build the demo-shell frontend.
FROM node:20-alpine AS ui-builder

WORKDIR /build

# Yarn 4 via Corepack (same as the main Dockerfile).
RUN corepack enable

# Workspace metadata first for layer caching.
COPY package.json yarn.lock .yarnrc.yml ./
# All declared workspaces must exist on disk for `yarn install --immutable`
# to succeed — copy them all even though we only build one here.
COPY ui ui
COPY sdks sdks
COPY demos demos

RUN yarn install --immutable
RUN yarn workspace @authproxy/demo-shell build

# Stage 2: build the Go backend with the dist tree embedded.
FROM golang:1.24 AS backend-builder

WORKDIR /build

# Go module deps first for layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Source.
COPY . .

# Overlay the freshly built frontend into the embed package's dist
# directory so the //go:embed all:dist directive picks up the real
# assets at compile time.
COPY --from=ui-builder /build/demos/shell/backend/embed/dist \
                       ./demos/shell/backend/embed/dist

# CGO not required for this binary — it doesn't link any C deps. Disable
# explicitly so the resulting binary is statically-linked and portable.
RUN CGO_ENABLED=0 go build -o /demo-shell ./demos/shell/backend

# Stage 3: minimal runtime.
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=backend-builder /demo-shell /app/demo-shell

# Default listen port — overridable via PORT env var (see
# demos/shell/backend/main.go).
EXPOSE 8888

ENTRYPOINT ["/app/demo-shell"]
