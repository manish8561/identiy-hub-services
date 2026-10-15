# Identity Hub Services

A full-stack application with Angular frontend and Go microservices backend.

## Project Structure

```
.
├── backend/           # Go microservices backend
│   ├── auth-service/  # Authentication service
│   ├── user-service/  # User management service
│   ├── api-gateway/   # API Gateway service
│   └── README.md      # Backend documentation
├── frontend/          # Angular frontend
├── deploy/                    # Deployment configurations
│   ├── docker-compose.yml       # Local / development
│   ├── docker-compose.prod.yml  # Production
│   └── docker-compose.test.yml  # Testing
└── deploy.sh                  # Production deploy helper (uses .env + prod compose)
```

## Prerequisites

- Docker and Docker Compose
- Node.js (for frontend development)
- Go 1.26+ (for backend development)

### Git hooks (optional)

Install the pre-commit and pre-push hooks once after cloning. The pre-commit hook checks staged Go files in each backend service with `gofmt` and runs `go vet ./...` and `go test ./...` for affected services. Format a file if needed with `gofmt -w <file>`, then stage it and commit again. The pre-push hook scans all backend services for vulnerabilities when backend files change, builds each changed backend service, and builds the frontend when frontend files change. The backend vulnerability scan requires `govulncheck`, `jq`, and a running Docker daemon. Trivy runs from the `aquasec/trivy` Docker image, so no host Trivy installation is needed:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
./.githooks/install.sh
```

Run the same formatting, lint, and test checks locally for all backend services at any time:

```bash
make -C backend lint-format
```

Run the vulnerability scan manually at any time, including before pushing. It runs `govulncheck`, scans Go dependencies and freshly built service images with Trivy, and prints results for all backend services:

```bash
make -C backend security-check
```

Skip checks for a single commit or push: `SKIP_GIT_HOOKS=1 git commit ...` or `SKIP_GIT_HOOKS=1 git push ...`

Run the scanners separately for local testing. Run `govulncheck` across all Go services:

```bash
for service in auth-service user-service api-gateway; do
  (cd "backend/$service" && govulncheck ./...) || exit 1
done
```

Run Trivy filesystem and container image scans without `govulncheck`:

```bash
make -C backend trivy-security-check
```

## Quick start

### Local environment (development)

Runs the full stack with ports published for debugging (Mongo, Kafka, auth, user, gateway, frontend).

1. **Start all services (backend + frontend)**:

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

Access points:

- Frontend: http://localhost:8085
- API Gateway: http://localhost:8080
- Swagger UI: http://localhost:8080/swagger/
- Auth Service: http://localhost:8081
- User Service: http://localhost:8082
- Notification Service: http://localhost:8083
- MongoDB: localhost:27017
- Kafka: localhost:9092

Stop everything:

```bash
docker compose -f deploy/docker-compose.yml down
```

Notes:

- The frontend proxies API calls to the gateway via `/api` (configured in `frontend/nginx.conf`).
- Angular environments use `apiUrl: '/api'` for containerized runs.

### Production environment

Uses `deploy/docker-compose.prod.yml` with secrets from a root `.env` file. Only the API gateway and frontend are published; MongoDB, Kafka, auth, and user stay on the Docker network.

1. **Deploy (recommended)**:

```bash
./deploy.sh
```

   On first run this creates `.env` with random Mongo password and `JWT_SECRET`.

2. **Or run Compose directly**:

   ```bash
   # Create .env first if it does not exist (or run ./deploy.sh once)
   docker compose -f deploy/docker-compose.prod.yml --env-file .env up -d --build
   ```

3. **Access**:
   - Frontend: http://localhost:8085
   - API Gateway: http://localhost:8080
   - Swagger UI: http://localhost:8080/swagger/

4. **Stop**:

   ```bash
   ./deploy.sh stop
   # or
   docker compose -f deploy/docker-compose.prod.yml --env-file .env down
   ```

### Testing environment

1. **Start test environment**:

   ```bash
   docker compose -f deploy/docker-compose.test.yml up --build
   ```

2. **Access test services**:
   - API Gateway: http://localhost:8080
   - Auth Service: http://localhost:8081
   - User Service: http://localhost:8082
   - MongoDB: localhost:27017

3. **Stop**:

   ```bash
   docker compose -f deploy/docker-compose.test.yml down
   ```

## API Documentation

### Swagger Documentation

The API Gateway provides centralized Swagger documentation for all microservices:

- **Swagger UI**: http://localhost:8080/swagger/index.html
- **API Documentation**: http://localhost:8080/docs
- **OpenAPI JSON**: http://localhost:8080/swagger/doc.json
- **OpenAPI YAML**: http://localhost:8080/swagger/doc.yaml

### Documentation Features

✅ **Centralized Documentation**: Single entry point for all API documentation
✅ **Interactive Testing**: Test APIs directly from the Swagger UI
✅ **Authentication Support**: HttpOnly cookie session (`access_token`) with Bearer fallback for API clients
✅ **Request/Response Examples**: Detailed examples for all endpoints
✅ **Error Codes**: Comprehensive error response documentation
✅ **API Versioning**: Version control for API changes

### Documentation Structure

The API documentation is organized by service:

#### Authentication Service (`/api/auth`)

- `POST /api/auth/login` - User login (sets session cookie)
- `POST /api/auth/register` - User registration (sets session cookie)
- `GET /api/auth/me` - Current session user ID
- `POST /api/auth/logout` - Clear session cookie
- `POST /api/auth/validate` - Validate session (cookie, body, or Bearer)
- `POST /api/auth/refresh` - Refresh session cookie

#### User Service (`/api/users`)

- `GET /api/users/me` - Get current user's profile
- `GET /api/users/profile/:id` - Get user profile (must match session user)
- `GET /api/users/list` - List users (paginated)
- `PUT /api/users/profile/:id` - Update user profile
- `DELETE /api/users/profile/:id` - Delete user profile

### Why Centralized Documentation?

1. **Single Source of Truth**: All API documentation in one place
2. **Consistent Experience**: Uniform documentation across all services
3. **Easier Maintenance**: One documentation to update
4. **Better Developer Experience**: No need to navigate multiple Swagger UIs
5. **API Gateway Integration**: Documentation matches the actual API Gateway routes

## Microservices Architecture

### Services Overview

1. **Auth Service** (Port 8081)
   - User authentication and registration
   - HttpOnly cookie session management (JWT stored in `access_token` cookie)
   - Password validation

2. **User Service** (Port 8082)
   - User profile management
   - User data CRUD operations
   - User search and listing
   - Cookie-based auth middleware on all `/api/users/*` routes

3. **API Gateway** (Port 8080)
   - Single entry point for all requests
   - Request routing to appropriate services
   - Forwards cookies between client and backend services
   - CORS handling
   - **Centralized API Documentation**

4. **Frontend** (Port 8085 in Docker)
   - Angular application
   - User interface for all operations
   - Cookie sessions via `withCredentials: true` (no token in localStorage)
   - Authentication and protected routes

### Database

- **MongoDB**: Database-per-service model
- **Auth data**: `auth_db.auth_users` (owned by auth-service)
- **Profile data**: `user_db.user_profiles` (owned by user-service)

### Message Queue

Kafka is enabled in Docker Compose for user lifecycle synchronization.

## Logging (Zap)

All backend services use **Uber's Zap** for high-performance structured logging.

### Features

- **Structured JSON Output**: All logs in JSON format for easy parsing
- **Environment-based Log Levels**: Configure via `LOG_LEVEL` environment variable
- **Service Identification**: Each log entry includes service name
- **Performance Optimized**: Minimal overhead logging

### Log Levels

Set the `LOG_LEVEL` environment variable:

- `debug`: Most verbose, includes debug information
- `info`: General information (default)
- `warn`: Warning messages
- `error`: Error messages only

### Example Log Output

```json
{
  "level": "info",
  "timestamp": "2024-01-15T10:30:45.123Z",
  "caller": "main.go:25",
  "message": "Server starting",
  "service": "auth-service",
  "port": "8081"
}
```

### Usage in Code

```go
import "your-service/internal/logger"

// Initialize logger (done in main.go)
logger.InitLogger()

// Use logger
logger.GetLogger().Info("User authenticated",
    zap.String("user_id", userID),
    zap.String("email", email),
)

logger.GetLogger().Error("Database connection failed",
    zap.Error(err),
    zap.String("database", "mongodb"),
)
```

## Kafka Integration

### Architecture

- **Event-Driven Communication**: Services communicate asynchronously via Kafka topics
- **Producer**: Auth Service publishes user lifecycle events
- **Consumer**: User Service processes events for data consistency
- **Topics**: Versioned event schemas for backward compatibility
- **Docker Image**: `apache/kafka:4.1.2` (pinned for reproducible local/test runs)

### Event Flow

1. **User Registration**: Auth Service creates user → publishes `user.created.v1`
2. **User Profile Sync**: User Service consumes `user.updated.v1` and upserts profile data
3. **User Profile Cleanup**: User Service consumes `user.deleted.v1` and deletes profile data

### Configuration

```bash
# Kafka Configuration
KAFKA_BROKERS=kafka:9092
KAFKA_CLIENT_ID=auth-service
KAFKA_TOPIC_USER_CREATED=user.created.v1
KAFKA_TOPIC_USER_UPDATED=user.updated.v1
KAFKA_TOPIC_USER_DELETED=user.deleted.v1
```

### Event Schema Example

```json
{
  "event_id": "evt_123456789",
  "event_type": "user.created.v1",
  "timestamp": "2024-01-15T10:30:45.123Z",
  "user_id": "user_123",
  "email": "user@example.com",
  "metadata": {
    "source": "auth-service",
    "trace_id": "trace_abc123"
  }
}
```

## Development Setup

### Backend Development

1. **Individual Service Development**:

   ```bash
   # Auth Service
   cd backend/auth-service
   go mod tidy
   go run main.go

   # User Service
   cd backend/user-service
   go mod tidy
   go run main.go

   # API Gateway
   cd backend/api-gateway
   go mod tidy
   go run main.go
   ```

2. **Environment Variables**:

   ```bash
   # Auth Service
   PORT=8081
   MONGO_URI=mongodb://localhost:27017
   MONGO_DB=auth_db
   JWT_SECRET=your-secret-key
   COOKIE_SECURE=false
   COOKIE_SAME_SITE=Lax
   ALLOWED_ORIGINS=http://localhost:4200,http://localhost:8085

   # User Service
   PORT=8082
   MONGO_URI=mongodb://localhost:27017
   MONGO_DB=user_db
   JWT_SECRET=your-secret-key
   ALLOWED_ORIGINS=http://localhost:4200,http://localhost:8085

   # API Gateway
   PORT=8080
   AUTH_SERVICE_URL=http://localhost:8081
   USER_SERVICE_URL=http://localhost:8082

   # Kafka (for local development)
   KAFKA_BROKERS=localhost:9092
   KAFKA_CLIENT_ID=auth-service
   KAFKA_GROUP_ID=user-service-group
   KAFKA_TOPIC_USER_CREATED=user.created.v1
   KAFKA_TOPIC_USER_UPDATED=user.updated.v1
   KAFKA_TOPIC_USER_DELETED=user.deleted.v1
   ```

### Frontend Development

```bash
cd frontend
npm install
ng serve
```

## API Endpoints

### Authentication (via API Gateway)

Login and register set an `access_token` HttpOnly cookie. Use `-c` / `-b` with curl to persist and send cookies.

```bash
# Login (saves cookie to cookies.txt)
curl -c cookies.txt -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}'

# Register
curl -c cookies.txt -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"John Doe","email":"user@example.com","password":"password","confirmPassword":"password"}'

# Current session
curl -b cookies.txt http://localhost:8080/api/auth/me

# Delete the account (also removes the user profile)
curl -b cookies.txt -X DELETE http://localhost:8080/api/auth/account

# Logout
curl -b cookies.txt -X POST http://localhost:8080/api/auth/logout
```

### User Management (via API Gateway)

```bash
# Get current user profile (requires session cookie)
curl -b cookies.txt http://localhost:8080/api/users/me

# List users (requires authentication)
curl -b cookies.txt http://localhost:8080/api/users/list
```

## Production Deployment Script

A comprehensive production deployment script is available to automate the deployment process.

### Quick Deployment

```bash
# Deploy the application (default command)
./deploy.sh

# Or explicitly specify deploy
./deploy.sh deploy
```

### Available Commands

```bash
# Deploy the application
./deploy.sh deploy

# Stop all services
./deploy.sh stop

# Restart all services
./deploy.sh restart

# View and follow logs
./deploy.sh logs

# Check service status
./deploy.sh status

# Run health checks
./deploy.sh health

# Show help
./deploy.sh help
```

### Features

- **Environment Setup**: Automatically generates secure secrets and creates `.env` file
- **Compose file**: Deploys with `deploy/docker-compose.prod.yml`
- **Dependency Checks**: Validates Docker and Docker Compose installation
- **Backup System**: Creates backups of existing data before deployment
- **Health Monitoring**: Waits for services to become healthy with timeout
- **Health Checks**: Verifies published gateway and frontend endpoints after deployment
- **Graceful Error Handling**: Proper error handling and cleanup

### Deployment Information

After successful deployment, the script will display:

```bash
=== Production Deployment Information ===
API Gateway:     http://localhost:8080
Swagger UI:      http://localhost:8080/swagger/
Frontend:        http://localhost:8085
Auth/User/Mongo/Kafka: internal Docker network only

=== Useful Commands ===
View logs:       docker compose -f deploy/docker-compose.prod.yml --env-file .env logs -f
Check status:    docker compose -f deploy/docker-compose.prod.yml --env-file .env ps
Stop services:   docker compose -f deploy/docker-compose.prod.yml --env-file .env down
Restart service: docker compose -f deploy/docker-compose.prod.yml --env-file .env restart [service-name]

=== Monitoring ===
Monitor health:  watch docker compose -f deploy/docker-compose.prod.yml --env-file .env ps
View resources:  docker stats
```

## Docker Commands

### Local

```bash
# Start all services (detached)
docker compose -f deploy/docker-compose.yml up -d --build

# Stop services
docker compose -f deploy/docker-compose.yml down

# View logs
docker compose -f deploy/docker-compose.yml logs

# View specific service logs
docker compose -f deploy/docker-compose.yml logs auth-service
docker compose -f deploy/docker-compose.yml logs user-service
docker compose -f deploy/docker-compose.yml logs api-gateway
docker compose -f deploy/docker-compose.yml logs frontend
```

### Production

```bash
# Start (requires .env at repo root — created by ./deploy.sh)
docker compose -f deploy/docker-compose.prod.yml --env-file .env up -d --build

# Stop services
docker compose -f deploy/docker-compose.prod.yml --env-file .env down

# View logs
docker compose -f deploy/docker-compose.prod.yml --env-file .env logs

# View specific service logs
docker compose -f deploy/docker-compose.prod.yml --env-file .env logs auth-service
docker compose -f deploy/docker-compose.prod.yml --env-file .env logs user-service
docker compose -f deploy/docker-compose.prod.yml --env-file .env logs api-gateway
docker compose -f deploy/docker-compose.prod.yml --env-file .env logs frontend
```

### Testing

```bash
# Start test environment
docker compose -f deploy/docker-compose.test.yml up --build

# Stop test services
docker compose -f deploy/docker-compose.test.yml down

# View test logs
docker compose -f deploy/docker-compose.test.yml logs
```

### Individual Services

```bash
# Build individual services
docker build -t auth-service backend/auth-service/
docker build -t user-service backend/user-service/
docker build -t api-gateway backend/api-gateway/

# Run individual services
docker run -p 8081:8081 auth-service
docker run -p 8082:8082 user-service
docker run -p 8080:8080 api-gateway
```

## Health Checks

```bash
# Local / test (services published to host)
curl http://localhost:8080/health  # API Gateway
curl http://localhost:8081/health  # Auth Service
curl http://localhost:8082/health  # User Service

# Production (only gateway + frontend are published)
curl http://localhost:8080/health
curl http://localhost:8085
./deploy.sh health
```

## Troubleshooting

### Common Issues

1. **Port conflicts (local/test)**: Ensure ports 8080, 8081, 8082, 8085, 27017, and 9092 are available
2. **Port conflicts (prod)**: Ensure ports 8080 and 8085 are available (Mongo/Kafka/auth/user are not host-published)
3. **MongoDB connection**: Check if MongoDB is running and accessible; for prod, confirm `.env` credentials match the volume
4. **Service communication**: Verify service URLs in API Gateway configuration
5. **Authentication errors**: Check `JWT_SECRET` matches across auth-service and user-service; verify cookies are sent (`withCredentials: true` in browser, `-b` in curl)
6. **Missing `.env` (prod)**: Run `./deploy.sh` once, or create `.env` from the Production Environment section below

7. **Image pull/DNS errors**: If you see errors like `server misbehaving` when pulling base images, try:
   ```bash
   sudo systemctl restart systemd-resolved
   docker system prune -f
   docker compose -f deploy/docker-compose.yml build --pull
   ```

### Logs and Debugging

```bash
# Local
docker compose -f deploy/docker-compose.yml logs -f

# Production
docker compose -f deploy/docker-compose.prod.yml --env-file .env logs -f
# or
./deploy.sh logs

# Testing
docker compose -f deploy/docker-compose.test.yml logs -f
```

## Environment Variables

### Production (`.env` at repo root)

Used by `deploy/docker-compose.prod.yml` and `./deploy.sh`. Generated automatically on first deploy:

```bash
MONGO_INITDB_ROOT_USERNAME=admin
MONGO_INITDB_ROOT_PASSWORD=<random-hex>
MONGO_INITDB_DATABASE=auth_db
JWT_SECRET=<random-hex>
LOG_LEVEL=0
API_GATEWAY_PORT=8080
FRONTEND_PORT=8085
# Set COOKIE_SECURE=true and COOKIE_SAME_SITE=None when serving over HTTPS
COOKIE_SECURE=false
COOKIE_SAME_SITE=Lax
ALLOWED_ORIGINS=http://localhost:8085
```

Inside the compose stack, services also use:

```bash
# Wired from .env by docker-compose.prod.yml
MONGO_URI=mongodb://${MONGO_INITDB_ROOT_USERNAME}:${MONGO_INITDB_ROOT_PASSWORD}@mongodb:27017
MONGO_DB=auth_db   # auth-service
MONGO_DB=user_db   # user-service
GIN_MODE=release
KAFKA_BROKERS=kafka:9092
```

### Test Environment

```bash
# MongoDB
MONGO_INITDB_ROOT_USERNAME=admin
MONGO_INITDB_ROOT_PASSWORD=password123
MONGO_INITDB_DATABASE=auth_db

# Auth Service
PORT=8081
MONGO_URI=mongodb://admin:password123@mongodb:27017
MONGO_DB=auth_db
JWT_SECRET=test-jwt-secret-key-for-testing

# User Service
PORT=8082
MONGO_URI=mongodb://admin:password123@mongodb:27017
MONGO_DB=user_db
KAFKA_CLIENT_ID=user-service-test
KAFKA_GROUP_ID=user-service-group-test

# API Gateway
PORT=8080
AUTH_SERVICE_URL=http://auth-service:8081
USER_SERVICE_URL=http://user-service:8082

# Kafka
KAFKA_BROKERS=kafka:9092
KAFKA_CLIENT_ID=auth-service-test
KAFKA_TOPIC_USER_CREATED=user.created.v1
KAFKA_TOPIC_USER_UPDATED=user.updated.v1
KAFKA_TOPIC_USER_DELETED=user.deleted.v1
```

## Features

- **Microservices Architecture**: Scalable and maintainable service decomposition
- **User Authentication**: HttpOnly cookie session (JWT in `access_token` cookie)
- **User Management**: Complete CRUD operations for user profiles
- **API Gateway**: Single entry point with routing and middleware
- **MongoDB Integration**: Service-owned databases with event-driven sync
- **Event-Driven Communication**: Kafka-based async messaging between services
- **Structured Logging**: Zap-based JSON logging across all services
- **Docker Support**: Complete containerization for all services
- **Local / Prod / Test Environments**: Separate Docker Compose files for each environment
- **Health Checks**: Service health monitoring
- **CORS Support**: Cross-origin resource sharing
- **Protected Routes**: Authentication-based route protection
- **Centralized API Documentation**: Swagger/OpenAPI documentation

### Verified Achievements

- **Backend Build Validation**: Auth Service, User Service, and API Gateway compile successfully.
- **Frontend Production Build**: Angular production bundle builds successfully.
- **Deployment Configuration Validation**: Local and test Docker Compose configurations are valid.
- **Production Configuration Validation**: Production Docker Compose configuration is valid when supplied with the required environment variables.
- **Kafka Integration**: User lifecycle events are configured for asynchronous service communication.
- **MongoDB Integration**: Auth and user data use service-owned MongoDB databases.
- **Docker Containerization**: Backend services and the frontend have dedicated container images.
- **Cookie-Based Authentication**: Authentication uses an HttpOnly `access_token` cookie with protected routes.
- **API Gateway**: Centralized routing connects clients to the backend services.
- **Auth and User Services**: Authentication and user profile management are implemented as separate services.
- **Message Queues**: Kafka integration for async communication
- **Structured Logging**: Zap-based JSON logging
- **API Documentation**: Swagger/OpenAPI documentation
- **Backend Hardening**: Implement rate limiting in the API Gateway and service-level middleware.
- **Backend Hardening**: Refactor route definitions into dedicated router modules in Go services.

## Documentation

- [Backend README](backend/README.md) - Detailed backend documentation
- [Frontend README](frontend/README.md) - Frontend-specific instructions

## Future Implementations and Enhancements

- **Upgrade versions for backend, db, frontend**
- **Observability**
  - Add `trace_id` propagation across API Gateway, Auth Service, and User Service.
  - Integrate OpenTelemetry for distributed tracing and metrics collection.
  -  **Service Discovery**: Implement service discovery (Consul, etcd)
- **Infrastructure**
  - Add Kubernetes manifests/Helm charts for frontend and backend deployments.
  - Introduce environment-specific deployment overlays (dev, test, prod).
- **Quality and reliability**
  - Expand automated test coverage (unit, integration, and API contract tests).
  - Add CI checks for build, lint, test, and image validation.
  - Add comprehensive test suites for each service
- **Notification Service**
  - For sending email, sms, push notifications, whatsapp, etc. as per need
  - Separate service with no link api gateway

1. **Load Balancing**: Add load balancers for each service
3. **Circuit Breakers**: Implement circuit breakers for service communication
4. **Distributed Tracing**: Add tracing (Jaeger, Zipkin)
5. **Monitoring**: Implement metrics and monitoring (Prometheus, Grafana) 

## Proposed Implementation Roadmap

The items below are proposed follow-up work and are not represented as implemented features.

### 1. Strengthen service reliability

- Add bounded retries, backoff, and dead-letter handling for Kafka event processing.
- Implement circuit breakers and timeouts for synchronous service-to-service calls.
- Add integration and contract tests for authentication, profile synchronization, and notification delivery.

### 2. Expand observability

- Propagate a correlation/trace ID across the API Gateway and backend services.
- Add OpenTelemetry instrumentation for distributed traces and service metrics.
- Provide Prometheus metrics and Grafana dashboards for request latency, error rates, Kafka consumer lag, and notification outcomes.

### 3. Extend notifications

- Add channel adapters for additional delivery methods such as SMS and push notifications.
- Keep channel-specific provider configuration isolated from the notification API.
- Add tests for provider failures, retries, and delivery-history consistency.

### 4. Improve deployment and delivery

- Add CI workflows to run formatting, lint, tests, builds, and container image validation.
- Add Kubernetes deployment manifests or Helm charts with environment-specific configuration.
- Evaluate service discovery and load balancing when scaling beyond the current Compose deployment.
