# Project Architecture

This document describes the current high-level architecture and runtime flow for the application.

## High-level architecture

```mermaid
flowchart LR
    subgraph Client[Client Layer]
        FE[Angular Frontend\nlocalhost:8085]
        BROWSER[Browser Cookies\naccess_token JWT]
    end

    subgraph Backend[Backend Services]
        GW[API Gateway\nlocalhost:8080\nRoutes + CORS + Swagger]
        AS[Auth Service\nlocalhost:8081\nAccounts / Sessions / Lifecycle]
        US[User Service\nlocalhost:8082\nProfile CRUD]
        NS[Notification Service\nlocalhost:8083\nPreferences + Email Delivery]
        KAFKA[Kafka\nUser lifecycle events]
    end

    subgraph Data[Data Stores]
        AUTHDB[(MongoDB\nauth_db\n auth_users + user_sessions + auth_action_tokens)]
        USERDB[(MongoDB\nuser_db)]
        NOTIFYDB[(MongoDB\nnotification_db)]
    end

    FE -->|HTTP + credentials| GW
    FE -->|Session cookie| BROWSER
    BROWSER -->|JWT access_token| GW
    BROWSER -->|JWT access_token| AS
    BROWSER -->|JWT access_token| US

    GW -->|/api/auth/*| AS
    GW -->|/api/users/*| US
    GW -->|/api/notifications/*| NS

    AS -->|Create / validate / revoke user_sessions| AUTHDB
    AS -->|Publish user.created.v1| KAFKA
    AS -->|Publish user.updated.v1| KAFKA
    AS -->|Publish user.deleted.v1| KAFKA
    AS -->|Verification / reset email| SMTP[SMTP Provider]

    KAFKA -->|Consume user lifecycle events| US
    US -->|Read / write user profiles| USERDB
    KAFKA -->|user.created.v1| NS
    NS -->|Persist preferences and delivery history| NOTIFYDB
    NS -->|SMTP welcome email| SMTP

    US -->|Return profile data| FE
    AS -->|Return auth result / cookie| FE
    NS -->|Return preference and history| FE
```

## Request and authentication flow

```mermaid
sequenceDiagram
    participant User
    participant Frontend
    participant Gateway
    participant AuthService
    participant UserService
    participant MongoDB

    User->>Frontend: Enter credentials
    Frontend->>Gateway: POST /api/auth/login
    Gateway->>AuthService: Forward login request
    AuthService->>MongoDB: Validate email and password
    MongoDB-->>AuthService: User record
    AuthService->>MongoDB: Insert user_sessions record for JWT jti
    AuthService-->>Gateway: Success + JWT
    AuthService-->>Frontend: Set HttpOnly access_token cookie
    Frontend-->>User: Authenticated session

    User->>Frontend: Request protected page
    Frontend->>Gateway: GET /api/users/me (cookie included)
    Gateway->>AuthService: Validate token
    AuthService-->>Gateway: Valid session
    Gateway->>UserService: Forward request with auth context
    UserService-->>Frontend: User profile data
```

## Event-driven synchronization flow

```mermaid
flowchart TD
    AUTH[Auth Service]
    KAFKA[Kafka Topic: user.events]
    USER[User Service]
    NOTIFY[Notification Service]
    MONGO[(MongoDB user_db.user_profiles)]
    NOTE[(MongoDB notification_db)]

    AUTH -->|user.created.v1| KAFKA
    AUTH -->|user.updated.v1| KAFKA
    AUTH -->|user.deleted.v1| KAFKA

    KAFKA -->|Consume event| USER
    USER -->|Upsert / Delete profile| MONGO

    KAFKA -->|user.created.v1| NOTIFY
    NOTIFY -->|Persist preference + delivery history| NOTE
```

## Components

### API Gateway

- Routes requests to the auth, user, and notification services
- Centralizes Swagger and OpenAPI docs
- Handles CORS and credentials-aware browser traffic
- Forwards cookie-based authentication context to downstream services
- Uses a shared HTTP client with a 10-second timeout for upstream health and proxy calls, propagating request cancellation and deadlines

### Auth Service

- Handles registration, login, logout, and refresh
- Creates new accounts as unverified, sends verification links, and rejects login until email verification
- Issues single-use email-verification and password-reset tokens; stores only token hashes and expires them using MongoDB TTL
- Supports password reset/change, email re-verification, self-service account deletion, and admin account controls
- Validates credentials and account status against MongoDB
- Creates JWTs backed by revocable `user_sessions` records
- Validates session activity and revokes sessions on logout, refresh, password reset, email or role change, suspension, and account deletion
- Emits user lifecycle events to Kafka
- Sends verification and password-reset mail through the configured SMTP provider

### User Service

- Owns user profile state in `user_db`
- Consumes Kafka events to keep profiles synchronized
- Checks the synchronized verification/account status and active auth session on protected requests
- Enforces that users only access their own profile data

### Notification Service

- Stores notification preferences and delivery history
- Consumes `user.created.v1` and sends a welcome email when enabled
- Records failed or successful delivery attempts
- Uses optional SMTP configuration and degrades gracefully when email is not configured

## Data model

- `auth_db.auth_users`: authentication records
- `auth_db.user_sessions`: one revocable session record per issued cookie; stores session metadata, not the JWT
- `auth_db.auth_action_tokens`: hashed, single-use verification and password-reset tokens with expiration
- `user_db.user_profiles`: synced profile data
- `notification_db.notification_preferences`: per-user email preference
- `notification_db.notification_records`: delivery history and outcomes

## Operational notes

- Browser sessions use HttpOnly cookies; no token is stored in localStorage. Each cookie maps to an active `auth_db.user_sessions` record until logout, refresh, a security-sensitive account change, or expiration.
- Configure `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, and `EMAIL_FROM` for account verification and password-reset emails; `FRONTEND_URL` defines the public origin used in email links.
- Admin role/status changes and account deletions are applied in Auth Service and propagated to user profiles through Kafka. User Service also checks active account/session state on protected requests.
- Kafka keeps the services loosely coupled and resilient to async propagation
- The gateway acts as the single public interface for client traffic
- The backend supports both local containerized runs and production deployment via Docker Compose and the `deploy.sh` helper

## Future changes

Proposed follow-up work. Items are prioritized; higher priority should land before infra/product expansions unless a release needs them sooner.

### P0 — Security and correctness

1. ~~**Auth-side account delete**~~ — ✅ Done: self-service deletion requires the current password, revokes sessions, deletes the auth account, and publishes `user.deleted.v1` to remove the synchronized profile. Admins can also delete customer accounts.
2. ~~**Production cookie / TLS settings**~~ — ✅ Done: `COOKIE_SECURE=true` and `COOKIE_SAME_SITE=None` are now the production defaults (compose, `.env`, `deploy.sh`). `NewCookieConfig` enforces `Secure=true` whenever `SameSite=None` (RFC 6265bis).
3. **Secrets management** — Remove remaining static credentials and signing keys from Compose files and deploy helpers. Add a secret-provider integration (for example Docker secrets or an external vault), secret rotation, startup validation, and clear failure behavior for missing or weak production secrets.
4. **Identity hardening** — Add MFA/TOTP and recovery codes, with WebAuthn/passkeys as the preferred passwordless follow-up. Establish a reusable auth policy for step-up authentication on password, email, role, and account lifecycle changes.
5. **Security abuse controls** — Retain service-level rate limits, but add login/registration/forgot-password abuse protection that combines IP and account limits, account lockout or escalating delays, device/session risk signals, and privacy-safe responses. Add CAPTCHA only at external-facing boundaries where bot activity is measurable.
6. **Security audit trail** — Record append-only security events for login, logout, MFA changes, password and email changes, session revocation, role/status changes, and account deletion. Include actor, subject, source IP, user-agent, trace ID, timestamp, and outcome without storing secrets or sensitive token values.
7. **Authorization model** — Replace isolated role checks with a centralized policy boundary such as RBAC with scoped permissions, and prepare for per-resource and organization/tenant ownership rules. Include policy tests and prevent privilege escalation by self-service profile updates.
8. **End-to-end security validation** — Add automated tests for cookie flags, CSRF exposure, CORS origin handling, session fixation/replay, privilege boundaries, password reset/enrollment links, and admin restrictions.

### P1 — Reliability and quality

1. ~~**Kafka DLQ + bounded retries / backoff**~~ — ✅ Done: consumers use bounded retries and backoff, publish exhausted events to service-specific DLQs, and registration uses a durable outbox-style event.
2. **Integration and contract tests** — Unit tests exist for core packages; add end-to-end coverage for auth → Kafka → user/notification sync and API contracts.
3. **CI pipelines** — Makefile and git hooks exist locally; add CI (e.g. GitHub Actions) for format, lint, test, build, and image validation.
4. ~~**Wire frontend to user APIs**~~ — ✅ Done: the admin dashboard lists paginated users with edit/delete actions, profile details can be updated, and placeholder dashboard metrics have been removed.
5. ~~**Service-level rate limiting**~~ — ✅ Done: Auth, User, and Notification services enforce per-IP token-bucket limits (10 requests/second, burst of 20) on API routes for defense in depth.
6. **Event contract registry** — Define versioned Avro/JSON Schemas or protobuf contracts for user lifecycle events, schema compatibility checks in CI, and explicit backward/forward compatibility rules.
7. **Idempotency and reconciliation** — Add idempotency keys to mutating public APIs and a recurring reconciliation job that detects auth/profile/notification drift and audits Kafka DLQs. Provide operator-safe replay tools.
8. **Data lifecycle and backup** — Document MongoDB retention, add scheduled backups and restore tests, verify TTL behavior in production-like environments, and define RPO/RTO objectives.
9. **API compatibility policy** — Establish public endpoint versioning, deprecation headers, pagination conventions, and stable error shapes. Generate or validate the gateway OpenAPI artifact in CI instead of maintaining documentation independently.
10. **Dependency and upgrade automation** — Pin service and base-image versions, add automated dependency update and vulnerability scans, and maintain a staged upgrade process for MongoDB, Kafka, Angular, Go, and container images.

### P1 — Observability

1. ~~**Correlation ID propagation**~~ — ✅ Done: `X-Correlation-ID` is validated or generated as UUIDv4 at the gateway and each service, returned to callers, propagated on proxied requests, and included in service request logs. Browser CORS allows and exposes the header.
2. **OpenTelemetry** instrumentation for distributed traces and metrics.
3. **Prometheus + Grafana** for request latency, error rates, Kafka consumer lag, and notification delivery outcomes.
4. **Structured audit and operational dashboards** — Add dashboards for auth success/failure, session revocations, password resets, account state changes, DLQ depth, replay activity, MongoDB health, and SMTP failure rates.
5. **Health and readiness semantics** — Separate process liveness from dependency readiness. Report MongoDB and Kafka connectivity, and avoid exposing internal dependency details to unauthenticated clients.
6. **Alerting and runbooks** — Add alert thresholds and actionable runbooks for broker loss, MongoDB degradation, consumer lag, DLQ growth, failed mail delivery, elevated login failures, and certificate or cookie misconfiguration.

### P2 — Product and infrastructure

1. **Additional notification channels** — Extend beyond email (SMS, push, WhatsApp) with isolated provider adapters and failure/retry tests.
2. **Kubernetes / Helm** — Manifests or charts with environment-specific overlays (dev, test, prod) when scaling beyond Compose.
3. **Load balancing and service discovery** — Evaluate when running multiple replicas of each service.
4. **Circuit breakers** for synchronous gateway → service calls (alongside timeouts).
5. **External identity federation** — Add OIDC/SAML integration for enterprise sign-in, account linking, just-in-time provisioning, and explicit local/enterprise account ownership rules.
6. **Multi-tenancy** — If organizations become a product requirement, introduce organization/workspace identifiers at the API, data, event, authorization, and audit layers. Keep tenant boundaries testable and avoid a later data migration.
7. **Privacy and consent controls** — Add account-data export, configurable retention/deletion, notification consent records, privacy preferences, and a documented deletion job for all service databases and email provider state.
8. **Delivery isolation** — Move notification provider and retry workers behind dedicated queues or jobs. Add per-channel templates, preview/test tooling, unsubscribe management, and provider reputation monitoring.
9. **Edge and network hardening** — Add TLS termination and trusted reverse-proxy configuration, security headers, CSP/CSRF policy review, request-size limits, and separate public/private service networks.
10. **Deployment environments** — Keep dev, test, staging, and production configurations isolated with immutable images, promotion workflows, smoke tests, database migration gates, and rollback criteria.

### P2 — Cleanup and documentation

1. ~~**Remove Kratos greeter / helloworld scaffold** from the API Gateway~~ — ✅ Done: removed the unused HTTP/gRPC greeter, generated proto code, and scaffold biz/data/service packages; the gateway now exposes only its HTTP health, Swagger, and proxy routes.
2. ~~**Refresh docs**~~ — Keep `README.md` / `backend/README.md` aligned with reality (notification service is implemented; gateway and service-level rate limiting and router modules are done).
3. ~~**Auth / user product gaps**~~ — ✅ Done: password change/reset, email verification, profile email re-verification, password-confirmed self-service account deletion, and admin role/status/deletion controls are available through the UI and backend.
4. ~~**User sessions collection**~~ — ✅ Done: `auth_db.user_sessions` records each issued cookie by JWT session ID, validates active sessions, revokes sessions on logout/refresh/account deletion, and expires old records through a MongoDB TTL index.
5. **Operational ownership** — Define service ownership, escalation paths, local run commands, required environment variables, release notes, and recovery procedures for MongoDB, Kafka, DLQs, mail, and sessions.

### Already in place (do not re-plan as greenfield)

- Cookie-based JWT sessions (HttpOnly `access_token`)
- Bcrypt password hashing on register/login (with lazy upgrade of legacy plaintext rows)
- Auth, User, Notification, and API Gateway microservices
- MongoDB per-service databases and Kafka user lifecycle sync
- Zap structured logging, Swagger/OpenAPI on the gateway
- Gateway rate limiting and dedicated router modules
- Per-IP token-bucket rate limiting in Auth, User, and Notification services
- Notification preferences + delivery history (email path)
- Local / prod / test Docker Compose and `deploy.sh`
- Local `make` lint / test / security tooling and unit tests for core packages
- Toast notifications for success and error messages in the frontend
- Correlation ID propagation through the API Gateway, Auth, User, and Notification services (`X-Correlation-ID` request/response header and service request logs; generated IDs are UUIDv4)
