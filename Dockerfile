# Stage 1: Build
FROM golang:1.24 AS builder

WORKDIR /build

# Download dependencies first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=1 go build -o /authproxy ./cmd/server

# Stage 2: Runtime
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /authproxy /app/authproxy

EXPOSE 8080 8081 8082 8083

ENTRYPOINT ["/app/authproxy"]
CMD ["serve", "--config=/app/dev_config/docker.yaml", "all"]
