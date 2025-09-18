# AuthProxy

AuthProxy is an open source, embeddable integration platform-as-a-service (iPaaS). It offloads the work of managing
the connection lifecycle to 3rd party systems so your application can call to those systems using an authenticating
proxy and stay focussed on the business logic of your product.

![marketplace-connectors.jpg](docs/images/marketplace-connectors.jpg)

## Running Locally

Create a network for the asynq system to interact with redis:

```bash
docker network create authproxy
```

Start redis (requires search module):

```bash
docker run --name redis-server -p 6379:6379 --network authproxy -d redis/redis-stack-server:latest
```

Start the AuthProxy backend

```bash
go run ./cli/server serve --config=./dev_config/default.yaml all
```

Run the client to proxy authenticated calls to the backend:

```bash
go run ./cli/client raw-proxy --enableLoginRedirect=true --proxyTo=api
```

Run the marketplace UI:

```bash
cd ui/marketplace
nvm use v18.16.0
yarn
yarn dev
```

### Viewing Redis Data

```bash
docker run -d --name redisinsight -p 5540:5540 -v redisinsight:/data --network authproxy redis/redisinsight:latest 
```

Add a connection to redis. Connect to the redis server using the following URI:

```
redis://default@redis-server:6379
```

![redis-insight-add-db.jpg](docs/images/redis-insight-add-db.jpg)

### Viewing Background Tasks
To manage tasks in asynq, install the [asynq cli](https://github.com/hibiken/asynq/blob/master/tools/asynq/README.md):

```bash
go install github.com/hibiken/asynq/tools/asynq@latest
```

and run the cli:

```bash
asynq dash
````

run the web monitoring tool:

```bash
docker run --rm \
    -d \
    --name asynqmon \
    --network authproxy \
    -p 8090:8080 \
    hibiken/asynqmon \
    --redis-addr=redis-server:6379
```

open the web ui:

```bash
open http://localhost:8090
```

![asynqmon.jpg](docs/images/asynqmon.jpg)

## Client Config

The client cli looks for a config file at `~/.authproxy.yaml`:

```yaml
admin_username: bobdole
admin_private_key_path: /path/to/private/key
server:
  api: http://localhost:8081
```