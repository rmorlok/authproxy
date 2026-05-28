# Multi-stage build for the demos/seed binary.
#
# Built from the repo root (`docker build -f demos/seed/Dockerfile .`)
# because the Go binary imports `internal/apauth/jwt` and the schema
# packages from the root module.
#
# No frontend stage — the seed binary is a one-shot CLI.

FROM golang:1.24 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO not required — no C deps. Statically link for the minimal runtime image.
RUN CGO_ENABLED=0 go build -o /demo-seed ./demos/seed/backend

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /demo-seed /app/demo-seed

ENTRYPOINT ["/app/demo-seed"]
