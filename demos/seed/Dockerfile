# Build the provider bootstrap helper and the ap CLI from the same revision.
#
# Built from the repo root (`docker build -f demos/seed/Dockerfile .`)
# so both executables use the same resource contracts and CLI implementation.
#
# No frontend stage — the seed binary is a one-shot CLI.

FROM golang:1.24-bookworm AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# The CLI currently links the SQLite driver through its resource registry.
# Build it with CGO against the same Debian release as the runtime image.
RUN CGO_ENABLED=0 go build -o /demo-seed ./demos/seed/backend \
    && CGO_ENABLED=1 go build -o /ap ./cmd/cli

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /demo-seed /app/demo-seed
COPY --from=builder /ap /usr/local/bin/ap

ENTRYPOINT ["/app/demo-seed"]
