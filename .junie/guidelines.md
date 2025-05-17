# AuthProxy Development Guidelines

This document provides guidelines and instructions for developing and maintaining the AuthProxy project. AuthProxy is a tool to be embedded in other 
applications to manage the authentication to other 3rd party systems. It is an embedded iPaaS without the focus on moving data. 

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
   go run ./cli/server serve --config=./dev_config/default.yaml all
   ```

4. **Run the client to proxy authenticated calls to the backend**:
   ```bash
   go run ./cli/client raw-proxy --proxyTo=<api|admin-api>
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

The project provides testing utilities in the `test_utils` package:
- `TestDataPath`: Helper for accessing test data files
- Mock implementations for database, encryption, and other components

## Development Guidelines

### Project Structure

The project is organized into multiple packages:
- `api_common`: Common API utilities
- `apctx`: Context-related utilities
- `auth`: Authentication logic
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

## Additional Resources

- [Go Documentation](https://golang.org/doc/)
- [Testify Documentation](https://github.com/stretchr/testify)
- [GORM Documentation](https://gorm.io/docs/)
- [OAuth 2.0 Specification](https://oauth.net/2/)