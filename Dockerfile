# Stage 1: Build the admin + marketplace UIs
FROM node:20-alpine AS ui-builder

WORKDIR /build

# Yarn 4 is selected via the repo's `packageManager` field in package.json.
# Corepack provisions it transparently on first `yarn` invocation.
RUN corepack enable

# Copy workspace metadata first for better layer caching.
COPY package.json yarn.lock .yarnrc.yml ./
COPY ui ui
COPY sdks sdks

RUN yarn install --immutable
RUN yarn workspace @authproxy/admin build \
 && yarn workspace @authproxy/marketplace build

# Stage 2: Build the Go server, embedding the UI artifacts produced above.
FROM golang:1.24 AS builder

WORKDIR /build

# Download Go module dependencies first for better layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Copy source.
COPY . .

# Overlay the freshly built UI bundles into the embed package directories so
# the //go:embed all:dist directives pick up the real assets at compile time.
COPY --from=ui-builder /build/ui/admin/embed/dist ./ui/admin/embed/dist
COPY --from=ui-builder /build/ui/marketplace/embed/dist ./ui/marketplace/embed/dist

RUN CGO_ENABLED=1 go build -o /authproxy ./cmd/server

# Stage 3: Minimal runtime image.
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /authproxy /app/authproxy

EXPOSE 8080 8081 8082 8083

ENTRYPOINT ["/app/authproxy"]
CMD ["serve", "--config=/app/dev_config/docker.yaml", "all"]
