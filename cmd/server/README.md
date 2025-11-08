# Server CLI

This is the CLI that starts the services on the server.

Start all services with:

```bash
go run ./cmd/server serve --config=./dev_config/default.yaml all
```

Or start individual services:
```bash
go run ./cmd/server serve --config=./dev_config/default.yaml worker
go run ./cmd/server serve --config=./dev_config/default.yaml api
go run ./cmd/server serve --config=./dev_config/default.yaml admin-api
```