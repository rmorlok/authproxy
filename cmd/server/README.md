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
