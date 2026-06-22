# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:24-bookworm AS frontend-builder
WORKDIR /src/plugins/grafana/authproxy-datasource

COPY plugins/grafana/authproxy-datasource/package.json plugins/grafana/authproxy-datasource/package-lock.json ./
RUN npm ci

COPY plugins/grafana/authproxy-datasource/ ./
RUN npm run frontend:build

FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS backend-builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src/plugins/grafana/authproxy-datasource

COPY plugins/grafana/authproxy-datasource/go.mod plugins/grafana/authproxy-datasource/go.sum ./
RUN go mod download

COPY plugins/grafana/authproxy-datasource/ ./
RUN test -n "${TARGETOS}" \
  && test -n "${TARGETARCH}" \
  && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
  go build -o "dist/gpx_authproxy_datasource_${TARGETOS}_${TARGETARCH}" ./pkg

FROM docker.io/grafana/grafana:11.5.2

USER root
COPY --from=frontend-builder /src/plugins/grafana/authproxy-datasource/dist /var/lib/grafana/plugins/rmorlok-authproxy-datasource
COPY --from=backend-builder /src/plugins/grafana/authproxy-datasource/dist/gpx_authproxy_datasource_* /var/lib/grafana/plugins/rmorlok-authproxy-datasource/
RUN chmod +x /var/lib/grafana/plugins/rmorlok-authproxy-datasource/gpx_authproxy_datasource_* \
  && chown -R grafana:root /var/lib/grafana/plugins/rmorlok-authproxy-datasource
USER grafana
