# AuthProxy Backend API

## Running Locally

Start the AuthProxy backend

```bash
go run ./cli/server serve --config=./dev_config/default.yaml all
```

Run the client to proxy authenticated calls to the backend:

```bash
go run ./cli/client raw-proxy --proxyTo=api
```

## Client Config

The client cli looks for a config file at `~/.authproxy.yaml`:

```yaml
admin_username: bobdole
admin_private_key_path: /path/to/private/key
server:
  api: http://localhost:8081
```