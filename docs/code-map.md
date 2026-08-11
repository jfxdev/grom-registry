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
| `.github/workflows/ci.yml` | Mandatory SQLite backend checks, separate mandatory PostgreSQL migration/repository suite, golangci-lint, govulncheck, frontend lint/tests/typecheck/build checks, and backend/frontend Codecov coverage and JUnit test-result uploads |
| `codecov.yml` | Explicit 70% patch-coverage gate and narrow generated-code/static-asset exclusions |
| `.github/workflows/registry-e2e.yml` | Mandatory real-Docker registry authorization, restart-preservation, and session-revocation acceptance check for pull requests, main, and merge queues |
| `.github/workflows/release-upgrade-e2e.yml` | Tagged-GHCR-release to checkout-candidate SQLite/local-storage upgrade acceptance check; it is intentionally separate from the required, network-independent registry journey |
| `.github/workflows/admin-journey-e2e.yml` | Mandatory browser-driven administrative first-push acceptance check for pull requests, main, and merge queues |
| `.github/workflows/boot-acceptance-e2e.yml` | Mandatory real-Docker default-installation boot, migration, readiness, and API-documentation acceptance check |
| `.github/workflows/backup-restore-e2e.yml` | Real-Docker SQLite and PostgreSQL volume-loss and recovery acceptance checks |
| `.github/workflows/production-image-smoke.yml` | Mandatory clean-checkout production-image build, non-root assertion, and public-surface smoke check |
| `.github/workflows/release.yml` | Tag-triggered GHCR image publication, SBOM and vulnerability-report generation, release checksums, and GitHub Release assets |
| `docs/` | Architecture, domain inventory, operations, decisions, and visual identity |
| `AGENTS.md` | Current operational instructions for coding agents |

## Documentation

| Path | Responsibility |
|---|---|
| `docs/architecture-and-mvp.md` | Product boundaries, runtime architecture, and MVP decisions |
| `docs/product-features.md` | Implemented product capabilities, business rules, permissions, channel coverage, and explicit gaps |
| `docs/domain-model.md` | Canonical inventory of domain types and ownership |
| `docs/code-map.md` | Repository navigation and operational entry points |
| `docs/backup-and-disaster-recovery-implementation-plan.md` | Implemented SQLite/PostgreSQL backup, empty-target restore, and recovery acceptance design |
| `docs/backup-and-disaster-recovery.md` | Operator backup, restore, encrypted retention, drill, and troubleshooting procedure |
| `docs/release-operations.md` | Operator installation by digest, upgrade, rollback, signing-key posture, and supported matrix |
| `docs/registry-e2e-implementation-plan.md` | Implemented real-Docker authorization, policy, inventory, and test-harness design and evidence |
| `docs/mvp-acceptance-implementation-plan.md` | Implemented and accepted public-browser first-push, boot/readiness/API-docs, session-revocation, and restart-preservation acceptance work |
| `docs/pagination-and-destructive-browser-acceptance-plan.md` | Planned public-browser acceptance of destructive flows |
| `docs/visual-identity.md` | Approved visual direction, responsive rules, and UI acceptance criteria |
| `docs/visual-implementation-plan.md` | Detailed frontend delivery phases, interaction behavior, and validation plan |
| `docs/assets/visual-identity/` | Visual concept references used by the identity guide |

## Backend contexts

| Context | Owns | Does not own |
|---|---|---|
| `identity` | Users (including installation viewers), sessions, service accounts, service-account tokens, and the viewer's single active profile token | Project membership decisions |
| `projects` | Projects, memberships, roles, authorization policies | Credential verification |
| `registry` | Logical repositories, behavior policies, Docker token grants, catalog reconciliation, safe deletion, `/v2` gateway | Blob persistence or OCI protocol implementation |
| `audit` | Immutable security event recording | Business transaction state or audit presentation/querying in the current MVP |

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
| `backend/internal/platform/backup/` | Backup manager and isolated agent, portable SQLite/PostgreSQL bundles, recovery web server, versioned sets, safe archives, and staged restore |
| `backend/internal/platform/maintenance/` | Drains active writes and blocks mutations and registry traffic during a quiesced snapshot |
| `backend/cmd/grom-backup/` | Image entry points for the backup agent, recovery UI, and low-level offline compatibility commands |
| `backend/internal/httpapi/security.go` | Bounded authentication failure limiter, trusted real-client-IP resolution, and rate-limit responses |
| `backend/migrations/` | Ordered migrations applied automatically during boot |
| `backend/api/openapi.yaml` | Canonical HTTP API contract |
| `backend/internal/generated/openapi/` | Generated Go transport contracts; never edit manually |
| `backend/internal/registry/infrastructure/persistence/bun/` | Logical repository, policy, inventory, deletion, lifecycle persistence, and keyset-paginated registry history and repository lists |
| `backend/internal/identity/infrastructure/persistence/bun/pagination.go` | Keyset-paginated, server-filtered administrative users, service accounts, and access keys |
| `backend/internal/projects/infrastructure/persistence/bun/pagination.go` | Keyset-paginated visible projects and principal-keyset project memberships |

## Frontend modules

| Module | Responsibility |
|---|---|
| `auth` | Sign-in, current-session experience, self-service password changes, and public magic-link reset completion |
| `projects` | Project list, detail, creation, memberships, and dedicated repository detail pages |
| `registry` | Repositories, manifest inventory, lifecycle dry-runs, manual execution, tags, and command snippets |
| `service-accounts` | Service-account lifecycle, assignments, and nested access-key management |
| `users` | User profile, password management, and installation user administration |
| `backups` | Installation-admin recovery-point creation, status, listing, and download |
| `settings` | Installation-admin operational status for the database backend and Distribution |

Audit presentation remains planned completion work and does not currently exist
under `frontend/src/modules`.

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
| `/api/v1/users`, `/api/v1/users/{id}`, `/api/v1/users/{id}/administrator`, `/api/v1/users/{id}/viewer`, `/api/v1/users/{id}/password-reset-link` | Identity administration, regular-user creation with reveal-once registration links, administrator/viewer promotion, user disabling with session revocation, and password-reset-link creation |
| `/api/v1/me/registry-tokens`, `/api/v1/me/registry-tokens/{tokenId}` | Viewer profile token: one active, reveal-once, revocable credential that grants pull only through explicit membership |
| `/api/v1/projects` | Project listing and installation-admin creation |
| `/api/v1/projects/{project}` | Project detail and installation-admin deletion of empty projects |
| `/api/v1/projects/{project}/repositories`, `/api/v1/projects/{project}/repositories/{repositoryId}`, `/api/v1/projects/{project}/repository-tags` | Registry browsing, direct repository detail lookup, and logical repository creation |
| `/api/v1/projects/{project}/repositories/{repositoryId}/archive`, `/api/v1/projects/{project}/repositories/{repositoryId}` | Project-admin archival and logical-repository removal; removal drains registry and management mutations, then revalidates the Distribution catalog and inventory before deleting the logical record |
| `/api/v1/projects/{project}/repositories/{repositoryId}/policies` | Optimistically versioned repository-policy reads and replacement |
| `/api/v1/registry-policy-presets` | Global read-only repository-policy recommendations |
| `/api/v1/projects/{project}/artifact-deletion-previews`, `/api/v1/projects/{project}/artifact-deletions` | Safe manifest deletion through the control plane |
| `/api/v1/projects/{project}/repository-inventory`, `/api/v1/projects/{project}/repository-inventory-reconciliations` | Metadata inventory and reconciliation with Distribution |
| `/api/v1/projects/{project}/lifecycle-previews`, `/api/v1/projects/{project}/lifecycle-runs` | Persisted retention dry-runs and audited manual execution |
| `/api/v1/deployment` | Public non-sensitive deployment posture used for operator warnings |
| `/api/v1/settings/status` | Installation-administrator view of the selected application database and Distribution availability |
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
| `backend/internal/registry/infrastructure/distribution/client.go` | Internal Distribution metadata client; encapsulates native catalog and tag pagination behind Grom cursors |
| `backend/internal/registry/infrastructure/persistence/bun/lifecycle.go` | Inventory, preview, run, and execution-lock persistence |
| `backend/internal/audit/` | Immutable lifecycle audit event recording |
| `backend/tests/registrye2e/` | Opt-in public-endpoint registry authorization, slow streaming blob upload, user-session revocation, and full-stack restart-preservation journeys; isolated Compose lifecycle, API and Docker clients, and network-independent fixtures |
| `backend/tests/bootacceptance/` | Opt-in public boot, migration, readiness, and API-documentation acceptance journey through an isolated Compose stack; includes the reviewed prior-schema fixture and a separate real-migration failure fixture |
| `backend/tests/backuprestoree2e/` | Opt-in destructive SQLite or PostgreSQL volume-loss recovery journey through public Grom and Docker endpoints |
| `frontend/e2e/` | Playwright mocked sign-in smoke plus isolated public-stack first-push and destructive-administration journeys |
| `frontend/src/modules/registry/` | Inventory and lifecycle API integration |

## Operational entry points

| Command | Purpose |
|---|---|
| `make generate` | Regenerate Go and TypeScript models from OpenAPI |
| `make test` | Run backend tests, frontend lint, tests, and type checks |
| `make test-postgres GROM_TEST_POSTGRES_URL=postgres://...` | Run the backend migration and repository suite against an available PostgreSQL database |
| `make test-coverage` | Generate Go and frontend LCOV coverage reports used by Codecov |
| `make test-registry-e2e` | Run the isolated real-Docker authorization, policy, JWT, inventory, session-revocation, and restart-preservation journeys |
| `make test-release-upgrade-e2e` | Pull the configured tagged GHCR baseline for `GROM_UPGRADE_PLATFORM` (default `linux/amd64`), upgrade its preserved SQLite/local-registry volumes to a locally built candidate, and verify metadata, credentials, blobs, and restart preservation |
| `make test-admin-e2e` | Run isolated real-browser first-push plus destructive access, credential, artifact, lifecycle, repository, project, user, and backup journeys |
| `make test-boot-acceptance` | Run the isolated default-installation boot, migration, readiness, and API-documentation journey |
| `make test-backup-restore-e2e` | Destroy and restore an isolated default installation and verify old and new registry activity |
| `make test-backup-restore-postgres-e2e` | Destroy and restore an isolated PostgreSQL installation and verify old and new registry activity |
| `make test-production-image-smoke` | Build the production image from a clean checkout and verify its non-root public runtime surface |
| `make build` | Build frontend and backend |
| `make dev` | Start the Go backend and Vite frontend together |
| `make dev-postgres` | Start a local PostgreSQL service, then the Go backend and Vite frontend using it |
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
