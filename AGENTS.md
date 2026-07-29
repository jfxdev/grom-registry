# Grom agent guide

Read these files before making architectural or cross-module changes:

1. `docs/code-map.md`
2. `docs/domain-model.md`
3. `docs/architecture-and-mvp.md`

Read `docs/visual-identity.md` and `docs/visual-implementation-plan.md` before
making broad frontend styling, branding, asset, or application-shell changes.

The first executable MVP is implemented.
Keep this file aligned with the code that actually exists.

## Commands

- Generate Go and TypeScript OpenAPI models: `make generate`
- Run backend and frontend checks: `make test`
- Run the isolated real-Docker registry journey: `make test-registry-e2e`
- Build both applications: `make build`
- Start backend and frontend together: `cp .env.example .env && make dev`
- Start the full local stack: `cp .env.example .env && make compose-up`
- Start the local stack with PostgreSQL: `make compose-up-postgres`
- Stop the local stack: `make compose-down`
- Backend only: `cd backend && GROM_DEPLOYMENT_PROFILE=development GROM_BOOTSTRAP_ADMIN_PASSWORD=change-this-password go run ./cmd/grom`
- Frontend only: `cd frontend && npm run dev`

Keep the exact Go patch version in `backend/go.mod` and the Dockerfile builder
image aligned. Go 1.26.5 is the current minimum because earlier 1.26 patch
releases are affected by reachable standard-library vulnerability
`GO-2026-5856`.

`make test-registry-e2e` requires an accessible Docker daemon and Docker
Compose. It owns only its unique Compose project, loopback port, temporary
Docker credential directories, and exact test image tags; never replace its
cleanup with a broad Docker prune. Docker daemon permission failures are
reported before startup. Distribution applies clock-skew tolerance to expired
JWTs, so the expiry assertion is bounded but can take about one minute.
Authenticate E2E principals immediately before their scenario because the
shared Docker daemon may cache bearer tokens for one registry address.
The mandatory GitHub status checks are `Backend Tests`, `Frontend Tests`,
`Go Lint`, `Go Vulnerability Check`, and `Registry E2E (Docker)`, defined under
`.github/workflows`. Keep these job names stable and require all five in the
`main` branch ruleset; the workflows also handle merge queues through
`merge_group`. Keep govulncheck's output in `text` mode because its JSON and
SARIF modes do not fail the job when vulnerabilities are found.

Do not edit generated files under `backend/internal/generated/openapi` or `frontend/src/shared/api/generated`.

## Architecture

- Backend: Go with pragmatic DDD and vertical bounded contexts under `backend/internal`.
- Contexts: Identity, Projects, Registry, Audit, and Integrations.
- Domain packages must not depend on Bun, HTTP frameworks, generated transport types, or another context's infrastructure.
- Repository interfaces express domain capabilities; Bun implementations live under the owning context's `infrastructure/persistence/bun`.
- Cross-context behavior uses narrow application interfaces, never direct access to another context's tables.
- Fundamental cross-context structs belong in `backend/internal/foundation`.
- Stable named backend constants belong in `backend/internal/constants`, split into concern-specific files.
- Do not create a generic repository, generic `utils` package, or abstraction without current behavior requiring it.

## Database and migrations

- SQLite and PostgreSQL are supported through Bun.
- Shared models and queries must remain portable between both databases.
- Pending, reviewed migrations run automatically during boot before HTTP readiness.
- Do not use ORM `AutoMigrate` or model-driven runtime schema diffing.
- A migration failure must fail startup.
- Test repository and migration changes against both SQLite and PostgreSQL.

## HTTP and OpenAPI

- `backend/api/openapi.yaml` is the source of truth for `/api/v1/*` and `/auth/token`.
- Change the OpenAPI contract before implementing an endpoint change.
- Generated Go transport code belongs in `backend/internal/generated/openapi`.
- Generated frontend API code belongs in `frontend/src/shared/api/generated`.
- Never edit generated files manually.
- Domain entities must not be generated from or coupled to OpenAPI transport schemas.
- CI must validate the contract, generated-code freshness, implementation compatibility, and frontend types.
- OpenAPI 3.0.x is intentional while `oapi-codegen` stable generation targets that version.
- Authentication failure limiting is bounded and in-process; do not add Redis
  for this single-instance MVP.
- Never restore unconditional real-IP middleware. Forwarded client addresses
  are trusted only when the immediate peer matches `GROM_TRUSTED_PROXIES`.
- Cookie-authenticated state changes require an allowed `Origin`; requests with
  a session cookie and no `Origin` are rejected.
- Non-local `GROM_PUBLIC_URL` values require HTTPS and
  `GROM_SECURE_COOKIES=true`.
- `GROM_DEPLOYMENT_PROFILE` defaults to `strict`; local development must set
  `development` explicitly. Private-LAN HTTP is accepted only in `permissive`
  with `GROM_ALLOW_INSECURE_PRIVATE_HTTP=true` and always produces operator and
  UI warnings.

## Frontend

- Use Vue 3, TypeScript, Vite, Vue Router, shadcn-vue, and feature-oriented modules.
- TanStack Query owns server state; Pinia is only for genuinely shared client state.
- shadcn-vue primitives belong in `frontend/src/shared/components/ui`.
- Product-specific icons and logos belong in `frontend/src/assets/icons` and `frontend/src/assets/logos`; prefer Lucide for generic interface icons.
- Files under `frontend/src/assets/raw` are source material and must not be imported by production UI code.
- Buttons use relief and press travel through transforms; decorative crystals are never placed inside buttons.
- Frontend-wide constants belong in `frontend/src/shared/constants`.
- Server-owned roles, statuses, actions, and enum values come from generated OpenAPI types; do not duplicate them manually.
- Keep strict TypeScript and cover critical flows with Vitest/Vue Testing Library and Playwright.

## Product constraints

- CNCF Distribution remains the unmodified OCI/Docker registry engine.
- Grom is the only public entry point; the Distribution port stays private.
- The first repository path segment is the immutable project authorization boundary.
- Only installation administrators create projects. When a Writer or Admin registry principal requests push scope inside an existing project, Grom idempotently creates a missing empty logical repository and grants push in the same token so the first push can succeed. Pull never creates repositories. Existing Distribution repositories are reconciled as active repositories without policies.
- Only installation administrators delete projects. Project deletion is allowed only when no logical repositories remain; never orphan Distribution content or erase repository inventory through project deletion.
- Repository behavior policies are isolated through their owning repository and project. Global policy presets are read-only form recommendations and never inherited runtime rules.
- Repository policy sets are replaceable by project administrators under the repository's optimistic `policyVersion`; never silently overwrite a stale policy set.
- External registry clients never receive `delete`. Manual artifact deletion resolves and deletes a manifest by digest through the authenticated control plane; blob garbage collection remains a separate operator action.
- Retention policy configuration supports an inventory-backed dry-run and an explicit, audited manual execution. Selecting retention never deletes content by itself; scheduled autopurge is not implemented.
- Lifecycle execution must reconcile with Distribution, revalidate every candidate immediately before deletion, and skip changed content.
- OCI subject/referrer relationships are inventoried and block lifecycle deletion in the current implementation. Do not add cascade deletion without an explicit policy and roadmap decision.
- Manual artifact deletion also blocks subjects with referrers and referrer artifacts. It must persist the operation, update inventory, and audit the outcome; do not bypass the application service from an HTTP handler.
- Lifecycle manifest deletion and Distribution blob garbage collection remain separate operations.
- Repository profiles are inferred passively from tagged primary OCI manifests. Referrers such as SBOMs and signatures never change the repository profile.
- Passive profile inference must not enable policies or reject pushes. Conflicting specific primary types produce the `mixed` profile with `profileNeedsReview=true`.
- Registry clients use API tokens, never web passwords.
- User password changes require the current password. Administrator resets use
  a system-generated reveal-once magic URL whose token is hashed, expires after
  30 minutes, and is carried in the URL fragment. Completing the reset revokes
  the target user's sessions. Authenticated users cannot access or consume reset
  links; they must change the password from their profile or sign out first.
  Never log or persist the plaintext reset token.
- API tokens are credentials owned exclusively by service accounts. Management endpoints and UI flows stay nested under the owning service account; do not reintroduce a global token page or user-owned registry tokens.
- Integrations are read-only planned catalog entries in the MVP; do not implement scanners, jobs, or secret storage without a roadmap decision.
- Avoid Redis, message brokers, dependency-injection frameworks, and background-job frameworks in the MVP.

## Required documentation maintenance

- Update `docs/domain-model.md` when architectural types, ownership, or relationships change.
- Update `docs/code-map.md` when paths, modules, routes, repositories, or entry points change.
- Update this `AGENTS.md` whenever future agents need a new command, constraint, generated-file rule, workflow, pitfall, or compatibility note.
- Remove stale instructions instead of appending contradictory guidance.
- Every structural change must explicitly assess whether all three documents need updates.
