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
| `deploy/backup/` | Low-level development and compatibility helpers for offline backup inspection and restore |
| `deploy/docker/` | Multi-mode image entrypoint for normal, backup-agent, and recovery execution |
| `deploy/distribution/` | CNCF Distribution configuration |
| `.github/workflows/ci.yml` | Mandatory Go formatting/tests, golangci-lint, govulncheck, frontend lint/tests/typecheck/build checks, and separate backend/frontend Codecov coverage and JUnit test-result uploads |
| `.github/workflows/registry-e2e.yml` | Mandatory real-Docker registry acceptance check for pull requests, main, and merge queues |
| `.github/workflows/backup-restore-e2e.yml` | Real-Docker volume-loss and recovery acceptance check |
| `docs/` | Architecture, domain inventory, operations, decisions, and visual identity |
| `AGENTS.md` | Current operational instructions for coding agents |

## Documentation

| Path | Responsibility |
|---|---|
| `docs/architecture-and-mvp.md` | Product boundaries, runtime architecture, and MVP decisions |
| `docs/product-features.md` | Implemented product capabilities, business rules, permissions, channel coverage, and explicit gaps |
| `docs/domain-model.md` | Canonical inventory of domain types and ownership |
| `docs/code-map.md` | Repository navigation and operational entry points |
| `docs/backup-and-disaster-recovery-implementation-plan.md` | Implemented default SQLite/local-storage backup, empty-volume restore, and recovery acceptance design |
| `docs/backup-and-disaster-recovery.md` | Operator backup, restore, encrypted retention, drill, and troubleshooting procedure |
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
| `backend/internal/platform/backup/` | Backup manager and agent, portable bundles, recovery web server, versioned sets, safe archives, and staged restore |
| `backend/internal/platform/maintenance/` | Drains active writes and blocks mutations and registry traffic during a quiesced snapshot |
| `backend/cmd/grom-backup/` | Image entry points for the backup agent, recovery UI, and low-level offline compatibility commands |
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
| `backups` | Installation-admin recovery-point creation, status, listing, and download |

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
| `/api/v1/backups`, `/api/v1/backups/{backupId}`, `/api/v1/backups/{backupId}/download` | Installation-admin paginated backup operations, confirmed local deletion, and portable recovery bundles |
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
| `backend/tests/backuprestoree2e/` | Opt-in destructive volume-loss recovery journey through public Grom and Docker endpoints |
| `frontend/src/modules/registry/` | Inventory and lifecycle API integration |

## Operational entry points

| Command | Purpose |
|---|---|
| `make generate` | Regenerate Go and TypeScript models from OpenAPI |
| `make test` | Run backend tests, frontend lint, tests, and type checks |
| `make test-coverage` | Generate Go and frontend LCOV coverage reports used by Codecov |
| `make test-registry-e2e` | Run the isolated real-Docker authorization, policy, JWT, and inventory journey |
| `make test-backup-restore-e2e` | Destroy and restore an isolated default installation and verify old and new registry activity |
| `make build` | Build frontend and backend |
| `make dev` | Start the Go backend and Vite frontend together |
| `make compose-up` | Build and start Grom plus Distribution |
| `make compose-up-postgres` | Build and start Grom, Distribution, and PostgreSQL |
| `make compose-down` | Stop the local stack |
| `make backup BACKUP_DIR=/absolute/path` | Exercise the low-level offline compatibility path during development |
| `make backup-inspect BACKUP_PATH=/absolute/path` | Inspect a transported set with the development tool |
| `make restore BACKUP_PATH=/absolute/path` | Exercise low-level empty-volume restore during development |
| `make reset-local` | Stop the local stack, remove its Compose volumes and orphan containers, and delete default local development data |

## Navigation rules

1. Start with `AGENTS.md`, this file, `docs/domain-model.md`, and
   `docs/architecture-and-mvp.md`.
2. Find the owning bounded context before editing a struct or rule.
3. Check `foundation` before creating a cross-context struct.
4. Check the constants package before introducing a repeated protocol, role, status, route, or storage-key literal.
5. Do not access another context's database tables directly.
6. Change `backend/api/openapi.yaml` before implementing an HTTP contract change.
7. Never edit generated OpenAPI files manually.
8. Update this map and assess `AGENTS.md` in the same change when ownership, workflows, or paths change.
