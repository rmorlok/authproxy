# AuthProxy Development Guidelines

This document provides guidelines and instructions for developing and maintaining the AuthProxy project. AuthProxy is a tool to be embedded in other 
applications to manage the authentication to other 3rd party systems. It is an embedded iPaaS without the focus on moving data. 

This project exposes several backend services that can either be run individually or as one from the command line. The `admin-api` is a service 
intended for running in  a restricted environment of the host application to provide ways to administer and monitor session proxy itself, but not 
to be consumed by end users  of the host application in any way. The UI for admin is located in the `ui/admin` folder.

The `api` service is the primary way the host application would consume this  project, using it to configure  connectors, create connections, 
and make requests to 3rd party systems where AuthProxy handles adding the necessary authentication to the requests.

The `public` service is intended to handle hosting a public facing web application, which is contained in the `ui/marketplace` folder. This application
provides the portal UI for listing connectors to the user and starting the connection flow for OAuth applications. The public service also contains
endpoints used as part of the OAuth redirect flow.

The `api` service uses JWT for authentication. `public` and `admin-api` use sessions. To get a session on `public` or `admin-api` the host service
has a redirect endpoint that will redirect back to an allowlisted location with a onetime use JWT that can be used to initiate a session.

## Build and Configuration Instructions

### Prerequisites
- Go 1.23 or later
- Docker (for running Redis and other services)

### Setting Up the Development Environment

1. **Create a Docker network for the asynq system**:
   ```bash
   docker network create authproxy
   ```

2. **Start Redis**:
   ```bash
   docker run --name redis-server -p 6379:6379 --network authproxy -d redis
   ```

3. **Start the AuthProxy backend**:
   ```bash
   go run ./cmd/server serve --config=./dev_config/default.yaml all
   ```

4. **Run the client to proxy authenticated calls to the backend**:
   ```bash
   go run ./cmd/cli raw-proxy --proxyTo=<api|admin-api>
   ```

5. **Make calls to 3rd party systems**:
   Once the client is running and communicating to the `api` you can make calls to 3rd party systems if a connection has been established:
   ```http
   ### example GET request to google calendar (if connected)
   POST http://127.0.0.1:8888/api/connections/<connection uuid>/_proxy
   Content-Type: application/json
   Accept: application/json

   {
     "url": "https://www.googleapis.com/calendar/v3/users/me/calendarList",
     "method": "GET",
     "headers": {
       "Accept": "application/json"
     }
   }
   ```

### Configuration

The application uses YAML configuration files. The development configuration is located at `./dev_config/default.yaml`.

Key configuration sections include:
- Server ports (public, API, admin API)
- Authentication settings (JWT signing keys, admin users)
- Database configuration (SQLite for development)
- Redis configuration
- OAuth connectors configuration

For client configuration, create a file at `~/.authproxy.yaml`:
```yaml
admin_username: bobdole
admin_private_key_path: /path/to/private/key
server:
  api: http://localhost:8081
```

## Testing Information

### Running Tests

To run all tests in the project:
```bash
go test -v ./...
```

To run tests in a specific package:
```bash
go test -v ./package/path
```

### Writing Tests

The project uses the standard Go testing package along with the `github.com/stretchr/testify/assert` package for assertions.

#### Test Structure
- Tests follow the standard Go test naming convention (`TestXxx`)
- Table-driven tests are commonly used for testing multiple scenarios
- Mocking is done using `gomock` and mock implementations are available in `*/mock` directories
- HTTP mocking is done using the `gock` library

#### Example Test

```go
package test

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestExample(t *testing.T) {
	// Setup
	expected := "hello world"
	actual := "hello world"
	
	// Test
	assert.Equal(t, expected, actual, "The two strings should be equal")
}
```

### Testing Utilities

The project provides testing utilities in the `internal/test_utils` package:
- `TestDataPath`: Helper for accessing test data files
- Mock implementations for database, encryption, and other components

## Development Guidelines

### Project Structure

The project is organized into multiple packages within `internal`:
- `api_common`: Common API utilities
- `apctx`: Context-related utilities
- `session`: Authentication logic
- `config`: Configuration handling
- `database`: Database access and models
- `encrypt`: Encryption utilities
- `jwt`: JWT handling
- `oauth2`: OAuth 2.0 implementation
- `proxy`: Proxy functionality
- `redis`: Redis client and utilities
- `util`: General utilities

### Code Style

The project follows standard Go coding conventions:
- Use `gofmt` or `go fmt` to format code
- Follow Go naming conventions (CamelCase for exported names, camelCase for non-exported names)
- Write comprehensive comments, especially for exported functions and types
- Use table-driven tests for testing multiple scenarios

### Continuous Integration

The project uses GitHub Actions for CI/CD:
- Builds are triggered on pushes to the main branch and pull requests
- The CI pipeline builds the project and runs all tests

## Javascript/Typescript Related Code

This codebase uses a monorepo style with multiple javasript packages contained within it. The `ui/marketplace` folder 
contains the frontend  code for the public facing application. The `ui/admin` folder contains the frontend code for 
the admin application. The `sdk/js` folder contains the SDK that is used by the other Javascript codebases.

This project uses nvm to manage node versions. When you start a terminal session, be sure to run `nvm use` to use the 
correct version of node, or inspect the `.nvmrc` file to see the version that is used and directly invoke it.

## Additional Resources

- [Go Documentation](https://golang.org/doc/)
- [Testify Documentation](https://github.com/stretchr/testify)
- [OAuth 2.0 Specification](https://oauth.net/2/)