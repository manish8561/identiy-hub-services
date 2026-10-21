# Backend Architecture Diagram

This document defines the complete backend flow for the application, including the client, API gateway, auth service, user service, notification service, MongoDB storage, and Kafka event sync.

## High-level architecture

```mermaid
flowchart LR
    subgraph Client[Client Layer]
        FE[Angular Frontend\nlocalhost:8085]
        BROWSER[Browser Cookies\naccess_token JWT]
    end

    subgraph Backend[Backend Services]
        GW[API Gateway\nlocalhost:8080\nRoutes + CORS]
        AS[Auth Service\nlocalhost:8081\nLogin / Register / Validate / Refresh]
        US[User Service\nlocalhost:8082\nProfile CRUD + Auth Middleware]
        NS[Notification Service\nlocalhost:8083\nEmail + Preference/History APIs]
        KAFKA[Kafka\nUser lifecycle events]
    end

    subgraph Data[Data Stores]
        AUTHDB[(MongoDB\nauth_db)]
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

    AS -->|Create / validate / refresh user session| AUTHDB
    AS -->|Publish user.created.v1| KAFKA
    AS -->|Publish user.updated.v1| KAFKA
    AS -->|Publish user.deleted.v1| KAFKA

    KAFKA -->|Consume user lifecycle events| US
    US -->|Read / write user profiles| USERDB
    KAFKA -->|user.created.v1| NS
    NS -->|Persist preferences and delivery history| NOTIFYDB
    NS -->|SMTP welcome email| EMAIL[Email provider]

    US -->|Return profile data| FE
    AS -->|Return auth result / cookie| FE
    NS -->|Return preference and history| FE
```

## Authentication flow

```mermaid
sequenceDiagram
    participant User
    participant Frontend
    participant Gateway
    participant AuthService
    participant MongoDB

    User->>Frontend: Enter credentials
    Frontend->>Gateway: POST /api/auth/login
    Gateway->>AuthService: Forward login request
    AuthService->>MongoDB: Validate email/password
    MongoDB-->>AuthService: User record
    AuthService-->>Gateway: JWT token + success response
    AuthService-->>Frontend: Set HttpOnly access_token cookie
    Frontend-->>User: Authenticated session

    User->>Frontend: Request protected page
    Frontend->>Gateway: GET /api/users/me (cookie included)
    Gateway->>AuthService: Validate token
    AuthService-->>Gateway: Valid user session
    Gateway->>UserService: Forward request with auth context
    UserService-->>Frontend: User profile data
```

## User profile synchronization flow

```mermaid
flowchart TD
    AUTH[Auth Service]
    KAFKA[Kafka Topic: user.events]
    USER[User Service]
    MONGO[(MongoDB user_db.user_profiles)]

    AUTH -->|user.created.v1| KAFKA
    AUTH -->|user.updated.v1| KAFKA
    AUTH -->|user.deleted.v1| KAFKA

    KAFKA -->|Consume event| USER
    USER -->|Upsert / Delete profile| MONGO

    USER -->|Read profile| MONGO
```

## Notification flow

```mermaid
flowchart LR
    AUTH[Auth Service]
    KAFKA[Kafka Topic: user.created.v1]
    NOTIFY[Notification Service]
    PREF[(notification_db.notification_preferences)]
    HIST[(notification_db.notification_records)]
    SMTP[SMTP provider]

    AUTH -->|User created| KAFKA
    KAFKA -->|Consume event| NOTIFY
    NOTIFY -->|Load user preference| PREF
    NOTIFY -->|Store delivery result| HIST
    NOTIFY -->|Send welcome email| SMTP
```

## Request lifecycle summary

1. The frontend sends requests to the API gateway.
2. The gateway routes the request to the relevant microservice.
3. Auth service handles login, registration, token validation, refresh, and logout.
4. User service handles profile retrieval, updates, listing, and deletion.
5. JWT is stored in an HttpOnly cookie named `access_token`.
6. Events are published to Kafka when user lifecycle changes occur.
7. User service consumes those events to keep the profile database synchronized.
8. Notification service consumes `user.created.v1`, checks the user's email preference, and records the welcome-email outcome. Browser clients can only read or update their own preference and history through the gateway.

## Notes

- `auth-service` owns `auth_db` authentication data.
- `user-service` owns `user_db` profile data.
- [x] Documented the relationship: each `user_profiles` document corresponds to an `auth_users` document by the same user ID (`_id`). They are separate collections with no database-level foreign key; Kafka lifecycle events asynchronously create, update, and delete the profile.
- `notification-service` owns `notification_db` preferences and delivery history.
- The API gateway is the single entry point for frontend traffic.
- Cookie-based auth is used for browser sessions, and token validation also supports bearer-token fallback in some flows.
