# Server CLI

This is the CLI that starts the services on the server.

Start all services with:

```bash
go run ./cmd/server serve --auto-migrate --config=./dev_config/default.yaml all
```

Or start individual services:
```bash
go run ./cmd/server serve --auto-migrate --config=./dev_config/default.yaml worker
go run ./cmd/server serve --auto-migrate --config=./dev_config/default.yaml api
go run ./cmd/server serve --auto-migrate --config=./dev_config/default.yaml admin-api
```

Production-style startup omits `--auto-migrate`. Inspect and migrate schemas
explicitly before starting services:

```bash
go run ./cmd/server migrate status --config=./dev_config/default.yaml
go run ./cmd/server migrate all --config=./dev_config/default.yaml
go run ./cmd/server serve --config=./dev_config/default.yaml all
```

Development servers can automatically apply resources from a yaml file after the
server has started up:

```bash
go run ./cmd/server serve \
  --auto-migrate \
  --config=./dev_config/default.yaml \
  --apply=./dev_config/resources.yaml \
  all
```

This command optionally takes an apply actor which will be used to apply the
resources via the API after startup. If that actor does not exist it will 
create a`system` and use that.

Production workloads should not use the `--apply` option and use the `ap apply`
cli command separately from cluster deployment.