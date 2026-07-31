# dify-agent-api

Go API service for the `dify-agent` browser SPA. The service follows the boundaries in [`docs/go-backend-architecture.md`](docs/go-backend-architecture.md): contract-first HTTP, inward-only dependencies, PostgreSQL persistence, server-managed sessions, and explicit operational controls.

## Prerequisites

- Docker with Compose
- `make`, `shasum`

A host Go installation is optional; all development commands use the pinned Go container.
The supported Go language line comes from the `go` directive in `go.mod`; the authoritative patch release is `toolchain go1.26.5`, which must match the pinned Makefile, Dockerfile, and CI images.

## Local setup

1. Copy `.env.example` to `.env.local` and supply local-only PostgreSQL credentials. For the container workflow, set `DIFY_AGENT_DATABASE_URL` with `postgres` as the host (for example, `postgres://<user>:<password>@postgres:5432/<database>?sslmode=disable`).
2. Set `DIFY_AGENT_BOOTSTRAP_ADMIN_EMAIL` and a unique password of at least 12 characters. The password is read from the deployment secret only; the API creates the `super_admin` on startup if it does not exist.
3. Keep `.env.local` uncommitted; its database and administrator values are development-only.
4. Start the complete local backend from the repository root:

```sh
make dev-up
```

`make dev-up` validates the local prerequisites and configuration, starts PostgreSQL, applies migrations, builds the API image, and runs the API in the foreground. Press `Ctrl+C` to stop the API; PostgreSQL and its persistent volume remain available until `make db-down` is run. Override the host API port when needed without changing service configuration:

```sh
API_HOST_PORT=28080 make dev-up
```

The local API is available under `http://localhost:18080/api/v1` (port `8080` is already used by the standard Dify middleware stack). Authentication endpoints are `POST /auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout`, and `GET /auth/me`. Access tokens are short-lived opaque bearer tokens; rotating refresh tokens are stored only in an HttpOnly/SameSite cookie and their hashes are persisted in PostgreSQL. The cookie is Secure by default and may be non-Secure only in the explicit development/test modes needed for localhost HTTP.

Production must inject a PostgreSQL URL with a non-empty username and password and `sslmode=verify-full`; modes that encrypt without verifying both certificate trust and hostname are rejected during startup. See [`docs/adr/0004-secure-transport-defaults.md`](docs/adr/0004-secure-transport-defaults.md) for the migration consequence and localhost exception.

## Quality gates

```sh
make verify
```

`make verify` is the authoritative non-modifying repository gate. It checks governance, architecture/config contracts, formatting, module metadata, generated-code drift, static analysis, source security, tests, race behavior, and the container build while proving that workspace content is unchanged. `make check` remains a temporary compatibility alias.

Database and full-stack checks use an isolated PostgreSQL URL supplied through `DIFY_AGENT_DATABASE_URL`. The E2E target builds and starts the real `cmd/api` binary, waits for readiness, exercises the authentication flow over HTTP, and cleans up the process:

```sh
make migration-check
make integration-test
make e2e-test
```

`make container-security-check` verifies the final image contents and blocks fixable High/Critical vulnerabilities. The final `security-check` and its CI Job intentionally remain absent until the project Owner approves the license allowlist required by ADR 0003; `make source-security-check`, `make container-security-check`, and `make license-report` expose the honest sub-results meanwhile.

Use `make prepare` when a change should regenerate the OpenAPI transport, tidy modules, and format Go files. The narrower `make generate`, `make tidy`, and `make fmt` targets are also intentionally modifying. Generated files under `generated/` must never be edited manually. Production migrations are applied as a separate release step; application startup only verifies the expected migration version.

## Engineering governance

- Architecture and module boundaries: [`docs/go-backend-architecture.md`](docs/go-backend-architecture.md)
- Go engineering standards: [`engineering-standards.md`](engineering-standards.md)
- Git and review workflow: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Agent collaboration rules: [`AGENTS.md`](AGENTS.md)
- Governance modernization tasks and live progress: [`docs/engineering-governance-modernization-task-breakdown.md`](docs/engineering-governance-modernization-task-breakdown.md)
- Verified pre-change baseline: [`docs/engineering-governance-baseline.md`](docs/engineering-governance-baseline.md)
- Risk controls and data classification: [`docs/requirements-risk-matrix.md`](docs/requirements-risk-matrix.md), [`docs/data-classification.md`](docs/data-classification.md)
- Operational recovery and release: [`docs/runbook-authentication.md`](docs/runbook-authentication.md), [`docs/runbook-database.md`](docs/runbook-database.md), [`docs/runbook-migration.md`](docs/runbook-migration.md), [`docs/release-and-rollback.md`](docs/release-and-rollback.md)
- SLO measurement status: [`docs/slo.md`](docs/slo.md)

## User capability

The first business vertical slice is under `internal/modules/user/`, with application use cases, credential infrastructure, PostgreSQL repositories, and HTTP transport kept behind explicit ports. Passwords use Argon2id with per-password random salts. See [`docs/adr/0001-local-credentials-authentication.md`](docs/adr/0001-local-credentials-authentication.md) for the security decision and operational consequences.
