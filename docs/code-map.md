# Grom code map

This file is the fast navigation entry point for maintainers and coding agents.
Update it whenever a bounded context, route group, repository, migration area, or frontend module is introduced, moved, or removed.

## Current state

The first executable MVP is implemented.
Use this map together with the root `AGENTS.md`; update both when paths or ownership change.

## Root ownership

| Path | Responsibility |
|---|---|
| `backend/` | Go API, authentication service, gateway, and application logic |
| `frontend/` | Vue web application |
| `deploy/compose/` | Local and small-installation deployment |
| `deploy/distribution/` | CNCF Distribution configuration |
| `.github/workflows/ci.yml` | Mandatory Go formatting/tests, golangci-lint, govulncheck, and frontend lint/tests/typecheck/build checks |
| `.github/workflows/registry-e2e.yml` | Mandatory real-Docker registry acceptance check for pull requests, main, and merge queues |
| `docs/` | Architecture, domain inventory, operations, decisions, and visual identity |
| `AGENTS.md` | Current operational instructions for coding agents |

## Documentation

| Path | Responsibility |
|---|---|
| `docs/architecture-and-mvp.md` | Product boundaries, runtime architecture, and MVP decisions |
| `docs/product-features.md` | Implemented product capabilities, business rules, permissions, channel coverage, and explicit gaps |
| `docs/domain-model.md` | Canonical inventory of domain types and ownership |
| `docs/code-map.md` | Repository navigation and operational entry points |
| `docs/registry-e2e-implementation-plan.md` | Implemented real-Docker authorization, policy, inventory, and test-harness design and evidence |
| `docs/visual-identity.md` | Approved visual direction, responsive rules, and UI acceptance criteria |
| `docs/visual-implementation-plan.md` | Detailed frontend delivery phases, interaction behavior, and validation plan |
| `docs/assets/visual-identity/` | Visual concept references used by the identity guide |

## Backend contexts

| Context | Owns | Does not own |
|---|---|---|
| `identity` | Users, sessions, service accounts, API token credentials | Project membership decisions |
| `projects` | Projects, memberships, roles, authorization policies | Credential verification |
| `registry` | Logical repositories, behavior policies, Docker token grants, catalog reconciliation, safe deletion, `/v2` gateway | Blob persistence or OCI protocol implementation |
| `audit` | Immutable security event recording | Business transaction state or audit presentation/querying in the current MVP |
| `integrations` | Integration catalog and future provider contracts | Scanner execution in the MVP |

Each context may contain:

```text
domain/                  entities, values, policies, repository interfaces
application/             commands, queries, DTOs, external ports
infrastructure/          Bun repositories and external adapters
```

HTTP route registration and translation currently live in the centralized
`backend/internal/httpapi/` package. Introduce context-owned transport folders
only if an actual split is implemented.

## Cross-cutting backend packages

| Path | Responsibility |
|---|---|
| `backend/internal/foundation/` | Canonical application-wide fundamental structs |
| `backend/internal/constants/` | Stable named constants organized by concern |
| `backend/internal/platform/` | Configuration, database setup, server lifecycle, logging, and composition |
| `backend/internal/platform/config/` | Deployment-profile policy, private-address validation, trusted-proxy ranges, authentication-limit settings, and public URL/cookie validation |
| `backend/internal/httpapi/security.go` | Bounded authentication failure limiter, trusted real-client-IP resolution, and rate-limit responses |
| `backend/migrations/` | Ordered migrations applied automatically during boot |
| `backend/api/openapi.yaml` | Canonical HTTP API contract |
| `backend/internal/generated/openapi/` | Generated Go transport contracts; never edit manually |
| `backend/internal/registry/infrastructure/persistence/bun/` | Logical repository and repository-policy persistence |

## Frontend modules

| Module | Responsibility |
|---|---|
| `auth` | Sign-in, current-session experience, self-service password changes, and public magic-link reset completion |
| `projects` | Project list, detail, creation, and memberships |
| `registry` | Repositories, manifest inventory, lifecycle dry-runs, manual execution, tags, and command snippets |
| `service-accounts` | Service-account lifecycle, assignments, and nested access-key management |
| `users` | User profile, password management, and installation user administration |
| `integrations` | Backend-driven integration catalog |

Audit presentation and settings modules are planned completion work and do not
currently exist under `frontend/src/modules`.

Cross-cutting frontend code lives under `frontend/src/shared`.
Static product artwork lives under `frontend/src/assets`, separated into `icons` and `logos`.
Shared Grom brand primitives live under `frontend/src/shared/components/brand`.
Source-only identity material remains under `frontend/src/assets/raw` and must not be imported by production UI code.
Server-owned API types and enum values come from `shared/api/generated`.
Application-wide UI constants live in `shared/constants`.
The frontend request client is hand-written in `shared/api/client.ts`; generated
OpenAPI types live under `shared/api/generated` and are never edited manually.

## HTTP entry points

| Route prefix | Owner |
|---|---|
| `/api/v1/session`, `/api/v1/me`, `/api/v1/me/password`, `/api/v1/password-resets` | Identity session, self-service password management, and public reset completion |
| `/api/v1/service-accounts`, `/api/v1/service-accounts/{id}/tokens` | Identity |
| `/api/v1/users`, `/api/v1/users/{id}/password-reset-link` | Identity administration and reveal-once magic reset-link creation |
| `/api/v1/projects` | Project listing and installation-admin creation |
| `/api/v1/projects/{project}` | Project detail and installation-admin deletion of empty projects |
| `/api/v1/projects/{project}/repositories`, `/api/v1/projects/{project}/repository-tags` | Registry browsing and logical repository creation |
| `/api/v1/projects/{project}/repositories/{repositoryId}/policies` | Optimistically versioned repository-policy reads and replacement |
| `/api/v1/registry-policy-presets` | Global read-only repository-policy recommendations |
| `/api/v1/projects/{project}/artifact-deletion-previews`, `/api/v1/projects/{project}/artifact-deletions` | Safe manifest deletion through the control plane |
| `/api/v1/projects/{project}/repository-inventory`, `/api/v1/projects/{project}/repository-inventory-reconciliations` | Metadata inventory and reconciliation with Distribution |
| `/api/v1/projects/{project}/lifecycle-previews`, `/api/v1/projects/{project}/lifecycle-runs` | Persisted retention dry-runs and audited manual execution |
| `/api/v1/integrations` | Integrations |
| `/api/v1/deployment` | Public non-sensitive deployment posture used for operator warnings |
| `/auth/token` | Registry authentication |
| `/v2/` | Streaming gateway to Distribution |

## Registry lifecycle implementation

| Path | Responsibility |
|---|---|
| `backend/internal/registry/application/repository_service.go` | Manual repository creation, discovered-repository import, and idempotent push provisioning |
| `backend/internal/registry/application/token_service.go` | OCI grants and Writer/Admin first-push repository provisioning |
| `backend/internal/registry/application/inventory_service.go` | Push observation and Distribution reconciliation |
| `backend/internal/registry/application/classifier.go` | Passive OCI artifact classification and repository-profile evidence |
| `backend/internal/registry/application/artifact_deletion_service.go` | Manual deletion preview, OCI relationship protection, revalidation, persistence, and audit |
| `backend/internal/registry/application/lifecycle_service.go` | Retention planning, live revalidation, and manual execution |
| `backend/internal/registry/infrastructure/distribution/gateway.go` | Public `/v2` reverse proxy, forwarding headers, manifest policy enforcement, and push observation |
| `backend/internal/registry/infrastructure/persistence/bun/lifecycle.go` | Inventory, preview, run, and execution-lock persistence |
| `backend/internal/audit/` | Immutable lifecycle audit event recording |
| `backend/tests/registrye2e/` | Opt-in public-endpoint journey, isolated Compose lifecycle, API and Docker clients, and network-independent fixtures |
| `frontend/src/modules/registry/` | Inventory and lifecycle API integration |

## Operational entry points

| Command | Purpose |
|---|---|
| `make generate` | Regenerate Go and TypeScript models from OpenAPI |
| `make test` | Run backend tests, frontend lint, tests, and type checks |
| `make test-registry-e2e` | Run the isolated real-Docker authorization, policy, JWT, and inventory journey |
| `make build` | Build frontend and backend |
| `make dev` | Start the Go backend and Vite frontend together |
| `make compose-up` | Build and start Grom plus Distribution |
| `make compose-up-postgres` | Build and start Grom, Distribution, and PostgreSQL |
| `make compose-down` | Stop the local stack |

## Navigation rules

1. Start with `AGENTS.md`, this file, and `docs/domain-model.md`.
2. Find the owning bounded context before editing a struct or rule.
3. Check `foundation` before creating a cross-context struct.
4. Check the constants package before introducing a repeated protocol, role, status, route, or storage-key literal.
5. Do not access another context's database tables directly.
6. Change `backend/api/openapi.yaml` before implementing an HTTP contract change.
7. Never edit generated OpenAPI files manually.
8. Update this map and assess `AGENTS.md` in the same change when ownership, workflows, or paths change.
