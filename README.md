# dify-agent-api

Go API service for the `dify-agent` browser SPA. The service follows the boundaries in [`docs/go-backend-architecture.md`](docs/go-backend-architecture.md): contract-first HTTP, inward-only dependencies, PostgreSQL persistence, server-managed sessions, and explicit operational controls.

## Prerequisites

- Docker with Compose
- `make`, `shasum`

A host Go installation is optional; all development commands use the pinned Go container.

## Local setup

1. Copy `.env.example` to `.env.local` and supply local-only PostgreSQL credentials. For the container workflow, set `DIFY_AGENT_DATABASE_URL` with `postgres` as the host (for example, `postgres://<user>:<password>@postgres:5432/<database>?sslmode=disable`).
2. Set `DIFY_AGENT_BOOTSTRAP_ADMIN_EMAIL` and a unique password of at least 12 characters. The password is read from the deployment secret only; the API creates the `super_admin` on startup if it does not exist.
3. Keep `.env.local` uncommitted; its database and administrator values are development-only.
4. Start PostgreSQL and apply migrations:

```sh
make db-up
set -a; source .env.local; set +a
make migrate-up
```

5. Build and run the API:

```sh
make build
docker run --rm --env-file .env.local --network dify-agent-api_default -p 127.0.0.1:18080:8080 dify-agent-api:dev
```

The local API is available under `http://localhost:18080/api/v1` (port `8080` is already used by the standard Dify middleware stack). Authentication endpoints are `POST /auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout`, and `GET /auth/me`. Access tokens are short-lived opaque bearer tokens; rotating refresh tokens are stored only in a Secure/HttpOnly/SameSite cookie and their hashes are persisted in PostgreSQL.

## Quality gates

```sh
make check
```

`make generate` regenerates the committed OpenAPI transport and updates the SHA-256 artifact. Generated files under `generated/` must never be edited manually. Production migrations are applied as a separate release step; application startup only verifies the expected migration version.

## User capability

The first business vertical slice is under `internal/modules/user/`, with application use cases, credential infrastructure, PostgreSQL repositories, and HTTP transport kept behind explicit ports. Passwords use Argon2id with per-password random salts. See [`docs/adr/0001-local-credentials-authentication.md`](docs/adr/0001-local-credentials-authentication.md) for the security decision and operational consequences.
