# Grom agent guide

Read these files before making architectural or cross-module changes:

1. `docs/code-map.md`
2. `docs/domain-model.md`
3. `docs/architecture-and-mvp.md`

Read `docs/visual-identity.md` and `docs/visual-implementation-plan.md` before
making broad frontend styling, branding, asset, or application-shell changes.

Read `docs/backup-and-disaster-recovery-implementation-plan.md` before making
backup, restore, persistent-volume recovery, or disaster-recovery changes.

The first executable MVP is implemented.
Keep this file aligned with the code that actually exists.

## Commands

- Generate Go and TypeScript OpenAPI models: `make generate`
- Run backend and frontend checks: `make test`
- Run the backend suite against PostgreSQL: `make test-postgres GROM_TEST_POSTGRES_URL=postgres://...`
- Generate backend and frontend coverage reports: `make test-coverage`
- Run the isolated real-Docker registry, session-revocation, and restart-preservation journeys: `make test-registry-e2e`
- Upgrade a tagged GHCR release to a locally built candidate while preserving SQLite/local-registry state: `make test-release-upgrade-e2e`
- Run the isolated browser-driven administrative first-push journey: `make test-admin-e2e`
- Run the isolated boot, migration, readiness, and API-documentation journey: `make test-boot-acceptance`
- Run the isolated destructive backup/restore journey: `make test-backup-restore-e2e`
- Run the isolated destructive PostgreSQL backup/restore journey: `make test-backup-restore-postgres-e2e`
- Build the clean-checkout production image and smoke-test its public runtime: `make test-production-image-smoke`
- Exercise the low-level offline backup compatibility tool: `make backup BACKUP_DIR=/absolute/path`
- Inspect a backup with the development tool: `make backup-inspect BACKUP_PATH=/absolute/path/to/backup`
- Exercise low-level empty-volume restore: `make restore BACKUP_PATH=/absolute/path/to/backup`
- Build both applications: `make build`
- Build and publish a local Docker image using `.env`: `make image-publish-local`
- Start backend and frontend together: `cp .env.example .env && make dev`
- Start backend and frontend with a local PostgreSQL service: `cp .env.example .env && make dev-postgres`
- Start the full local stack: `cp .env.example .env && make compose-up`
- Start the local stack with PostgreSQL: `make compose-up-postgres`
- Stop the local stack: `make compose-down`
- Fully reset the local stack and delete its data: `make reset-local`
- Backend only: `cd backend && GROM_DEPLOYMENT_PROFILE=development GROM_BOOTSTRAP_ADMIN_PASSWORD=change-this-password go run ./cmd/grom`
- Frontend only: `cd frontend && npm run dev`

Keep the exact Go patch version in `backend/go.mod` and the Dockerfile builder
image aligned. Go 1.26.5 is the current minimum because earlier 1.26 patch
releases are affected by reachable standard-library vulnerability
`GO-2026-5856`.
The runtime `grom` account is fixed at UID 100 and GID 101. Keep the Dockerfile,
restored-volume ownership, and backup-agent socket group aligned; changing these
IDs requires an explicit volume-ownership migration.

`make test-registry-e2e` and `make test-admin-e2e` require an accessible Docker
daemon and Docker Compose. Each owns only its unique Compose project, loopback
port, temporary Docker credential directories, and exact test image tags; never
replace cleanup with a broad Docker prune. The administrative journey drives
the frontend embedded in the container through Playwright; it disables browser
traces, screenshots, and video because it handles a reveal-once access key.
Keep its scenarios independent and wait for each browser-visible state caused
by a mutation before navigating or starting the next action. In particular,
wait for sign-in to reach the Projects page before invoking another protected
route, and blur/await a generated form value before replacing it in Playwright.
This prevents a navigation from cancelling an in-flight session request or a
slug field from receiving both its generated and test-entered values. Give each
scenario distinct project, account, and credential names; do not rely on state
left by a preceding scenario.
Docker daemon permission failures are reported before startup. Distribution
applies clock-skew tolerance to expired JWTs, so the expiry assertion is bounded
but can take about one minute. Authenticate E2E principals immediately before
their scenario because the shared Docker daemon may cache bearer tokens for one
registry address. The registry journey restarts only the exact isolated Grom and
Distribution services after a pushed fixture; it must prove public state and
blob preservation with a fresh Docker credential directory, and must never
recreate or inspect its volumes directly. It also streams an authenticated OCI
blob upload for longer than the short-lived registry JWT and verifies the
committed blob, so do not introduce body-read or response-write server timeouts
that would terminate long uploads.
The garbage-collection journey must delete and collect a fixture, republish the
same digest under the same tag, prove it is listed and pullable with fresh Docker
credentials, then repeat with a different digest. The maintenance process must
stop its supervised Distribution child before local-storage GC and start a fresh,
ready child afterward; never run the collector concurrently with Distribution or
replace this with Docker-socket process control.
`make test-release-upgrade-e2e` is a separate, non-ruleset release-acceptance
journey: it pulls the image in `GROM_UPGRADE_FROM_IMAGE` (defaulting to the
current published baseline) for `GROM_UPGRADE_PLATFORM` (defaulting to
`linux/amd64`), deploys it on fresh SQLite/local-storage volumes,
then upgrades those volumes to a candidate built from the checkout. It must
preserve the administrator, project, Writer access key, repository inventory,
and pushed blobs across the upgrade and a subsequent restart. Keep it outside
the network-independent registry journey and do not mark supported upgrades as
accepted until the tagged-release evidence has passed in CI.
`PostgreSQL Backup Restore E2E` runs the same destructive recovery journey
through the PostgreSQL Compose overlay. Do not advertise PostgreSQL as
supported until its first CI run passes; its status check is required by the
`main` branch ruleset.
The mandatory GitHub status checks are `Backend Tests`, `PostgreSQL Tests`,
`Frontend Tests`, `Go Lint`, `Go Vulnerability Check`, `Registry E2E (Docker)`,
`Admin Journey E2E (Docker)`, `Boot Acceptance E2E (Docker)`, and
`Backup Restore E2E`, and `Production Image Smoke (Docker)`, defined under
`.github/workflows`. Keep these job names stable and require all ten in the
`main` branch ruleset; the workflows also handle merge queues through
`merge_group`. Keep govulncheck's output in `text` mode because its JSON and
SARIF modes do not fail the job when vulnerabilities are found.
`Production Image Smoke (Docker)` builds the root Dockerfile from a clean
checkout, verifies the final image declares the non-root `grom` user, and
checks health, readiness, API documentation, and the registry bearer challenge
through an isolated public Compose stack.
`.github/workflows/release.yml` runs only for an existing stable
`vMAJOR.MINOR.PATCH` tag, publishes the exact image to GHCR,
and creates a GitHub Release with its digest reference, SPDX SBOM, Trivy report,
and checksums. It uses the repository `GITHUB_TOKEN`; do not replace it with a
long-lived registry credential or make a mutable image tag the canonical
deployment reference.
The `Backend Tests` and `Frontend Tests` jobs upload separate `backend` and
`frontend` coverage and JUnit test-result reports to Codecov. They require the
repository Actions secret `CODECOV_TOKEN`; never commit or log its plaintext
value. `codecov.yml` requires at least 70% patch coverage with 1% tolerance and
excludes only generated OpenAPI code and static frontend assets; do not lower
the target or broaden exclusions to hide untested production code.

Do not edit generated files under `backend/internal/generated/openapi` or `frontend/src/shared/api/generated`.

The installed backup interface is the installation-admin web UI. Disaster
recovery uses the `recovery` mode of the same Grom image and its local web UI.
Never make a source checkout, `make`, a host shell script, or Docker-socket
access a requirement of the product flow. The backup agent must remain
network-isolated, read source volumes only, and communicate with Grom only over
its dedicated Unix socket. Backup quiescence must drain and block management
mutations, token exchanges, and registry traffic; Distribution upload purging
must remain disabled for this profile. Recovery must remain loopback-only by
default and must refuse non-empty target volumes.
Backup listings use stable cursor pagination with exactly five recovery points
per page. Local snapshot deletion is installation-admin-only, requires UI
confirmation, resolves only a validated backup UUID, and must remain serialized
against creation and download. Deleting a local snapshot must never affect an
already downloaded bundle.

## Architecture

- Backend: Go with pragmatic DDD and vertical bounded contexts under `backend/internal`.
- Contexts: Identity, Projects, Registry, and Audit.
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
- Test every migration with a minimal pre-migration table in both engines. A
  PostgreSQL test may skip only when `GROM_TEST_POSTGRES_URL` is absent; use a
  single connection and temporary tables when the migration names fixed tables
  or indexes, so it neither collides with nor mutates shared test state.
  The `PostgreSQL Tests` CI job provisions this database and sets the URL so
  these checks cannot be skipped in CI.

## HTTP and OpenAPI

- `backend/api/openapi.yaml` is the source of truth for `/api/v1/*` and `/auth/token`.
- Change the OpenAPI contract before implementing an endpoint change.
- Generated Go transport code belongs in `backend/internal/generated/openapi`.
- Generated frontend API code belongs in `frontend/src/shared/api/generated`.
- Never edit generated files manually.
- Request schemas must reject retired and unknown properties with
  `additionalProperties: false` when their fields are closed. Keep the handler
  decoder equally strict, regenerate types, and test the rejection.
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
- For focus-managed components, test the actual focused element and its
  restoration path (for example Escape from a dropdown item returning focus to
  its trigger), not only whether markup disappears.

## Product constraints

- CNCF Distribution remains the unmodified OCI/Docker registry engine.
- Grom is the only public entry point; the Distribution port stays private.
- The first repository path segment is the immutable project authorization boundary.
- Only installation administrators create projects. When a Writer or Admin registry principal requests push scope inside an existing project, Grom idempotently creates a missing empty logical repository and grants push in the same token so the first push can succeed. Pull never creates repositories. Existing Distribution repositories are reconciled as active repositories without policies.
- Only installation administrators delete projects. Project deletion is allowed only when no logical repositories remain; never orphan Distribution content or erase repository inventory through project deletion.
- Repository archival blocks future pushes but preserves pull access and OCI content. Removing an archived logical repository must verify both empty live inventory and absence from Distribution's catalog; it never deletes OCI content and must be audited.
- Repository behavior policies are isolated through their owning repository and project. Global policy presets are read-only form recommendations and never inherited runtime rules.
- Repository policy sets are replaceable by project administrators under the repository's optimistic `policyVersion`; never silently overwrite a stale policy set.
- External registry clients never receive `delete`. Manual artifact deletion resolves and deletes a manifest by digest through the authenticated control plane; blob garbage collection remains a separate operator action.
- Retention policy configuration supports an inventory-backed dry-run and an explicit, audited manual execution. Selecting retention never deletes content by itself; scheduled autopurge is not implemented.
- Lifecycle execution must reconcile with Distribution, revalidate every candidate immediately before deletion, and skip changed content.
- OCI subject/referrer relationships are inventoried and block lifecycle deletion in the current implementation. Do not add cascade deletion without an explicit policy and roadmap decision.
- Deleting an image index may additionally delete only its untagged child manifests proven unreferenced by another live index, tag, or OCI referrer. Keep this distinct from forbidden OCI subject/referrer cascade deletion, expose the exact child set in the preview, and revalidate every digest before deletion.
- Manual artifact deletion also blocks subjects with referrers and referrer artifacts. It must persist the operation, update inventory, and audit the outcome; do not bypass the application service from an HTTP handler.
- Lifecycle manifest deletion and Distribution blob garbage collection remain separate operations.
- Repository profiles are inferred passively from tagged primary OCI manifests. Referrers such as SBOMs and signatures never change the repository profile.
- Passive profile inference must not enable policies or reject pushes. Conflicting specific primary types produce the `mixed` profile with `profileNeedsReview=true`.
- Registry clients use API tokens, never web passwords.
- Bootstrap creates the only initially defined installation administrator. Every
  later user is created as a regular user with a reveal-once registration link
  to choose an initial password and remains disabled until the link is consumed;
  only an active installation administrator may promote a user to installation
  administrator. The account and registration token must be created atomically;
  registration tokens may enable pending users, while password-reset tokens
  must not re-enable disabled users.
- User password changes require the current password. Administrator resets use
  a system-generated reveal-once magic URL whose token is hashed, expires after
  30 minutes, and is carried in the URL fragment. Completing the reset revokes
  the target user's sessions. Authenticated users cannot access or consume reset
  links; they must change the password from their profile or sign out first.
  Never log or persist the plaintext reset token.
- API tokens are normally credentials owned by service accounts. The sole exception is an installation viewer's profile-scoped, reveal-once registry token: it is revocable by that viewer and must always grant only `pull` from projects with explicit membership. It must never grant `push` or `delete`, even if the viewer has a Writer or Admin project membership. Do not add tokens for other user roles or a global user-token page.
- A service account may have at most three active access keys. Keep enforcement
  and the paginated list's active/max counts server-authoritative; the UI must
  never infer the limit from only the current history page.
- Avoid Redis, message brokers, dependency-injection frameworks, and background-job frameworks in the MVP.

## Required documentation maintenance

- Update `docs/domain-model.md` when architectural types, ownership, or relationships change.
- Update `docs/code-map.md` when paths, modules, routes, repositories, or entry points change.
- Update this `AGENTS.md` whenever future agents need a new command, constraint, generated-file rule, workflow, pitfall, or compatibility note.
- Remove stale instructions instead of appending contradictory guidance.
- Every structural change must explicitly assess whether all three documents need updates.
