# AuthProxy Grafana Datasource

First-party Grafana datasource plugin for querying AuthProxy application metrics.

The backend stores the AuthProxy base URL in datasource JSON settings and the JWT in Grafana secure JSON data. It supports:

- metrics time series through `POST /api/v1/metrics/query`
- request-event metadata tables through `GET /api/v1/metrics/request-events`
- variable query modes for namespaces, connectors, connections, actors, and rate limits

The frontend source provides config, query, and variable editors. The backend can be tested independently:

```bash
go test ./pkg/...
go build -o dist/gpx_authproxy_datasource ./pkg
```

## Provisioning

```yaml
apiVersion: 1
datasources:
  - name: AuthProxy
    type: rmorlok-authproxy-datasource
    access: proxy
    jsonData:
      baseUrl: http://authproxy-api:8081
    secureJsonData:
      jwt: ${AUTHPROXY_GRAFANA_JWT}
```

The JWT should be minted with the aggregate preset for metric dashboards, or the logs preset when request-event metadata tables are needed.
