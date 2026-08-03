# Grom: architecture and self-hosted MVP plan

## 0. Implementation progress and completion plan

This document is both the architectural contract and the delivery plan for the
first supported self-hosted Grom release. Architecture sections describe
decisions that must remain true. Progress sections describe the repository as
implemented on July 29, 2026 and must be updated when acceptance evidence
changes.

For a user-facing inventory of behavior, permissions, and known product gaps,
also read [`product-features.md`](product-features.md).

### Product and release posture

Grom targets self-hosted installations operated by individuals and small teams
that want a compact registry with reliable authentication, authorization,
recovery, and upgrades. The first supported release is not a minimal
throwaway homelab image, but it is also not an enterprise registry platform.

The MVP optimizes for:

- one active Grom instance;
- Grom plus unmodified Distribution;
- SQLite and local blob storage as the default supported path;
- Docker image push and pull as the primary protocol journey;
- a small administration UI;
- security controls that do not require Redis, a message broker, or an
  external identity provider;
- documented backup and restoration of the default installation.

PostgreSQL, S3-compatible storage, ORAS and generic OCI artifacts remain
supported directions, but their complete compatibility matrices must not delay
the default SQLite/local-storage release unless they are advertised as fully
supported in that release.

Enterprise capabilities remain outside the MVP: high availability, multiple
active replicas, organizations and teams, SAML/OIDC/LDAP, policy admission,
cross-registry replication, enterprise compliance reporting, and integrated
scanner execution.

### Current progress summary

| Phase | Status | Current evidence | Remaining exit work |
|---|---|---|---|
| Phase 0: executable foundation | Partially accepted | Go and Vue applications, embedded build, Compose, SQLite/PostgreSQL adapters, migrations, bootstrap admin, OpenAPI models, interactive docs, contract validation, generated-code freshness, route/contract checks, mandatory CI jobs, and an accepted public boot/migration/docs journey exist | Production image build, dependency/container scanning, and PostgreSQL CI before claiming the corresponding support |
| Phase 1: authentication and project authorization | Docker acceptance complete | Sessions, projects, memberships, service accounts, reveal-once keys, registry JWTs and role mapping are covered by application/integration tests and the opt-in real-Docker journey | ORAS before claiming generic OCI support |
| Phase 2: registry browsing and core UI | Partially accepted | Project, repository, manifest detail, membership, user, service-account, policy, deletion, lifecycle, and user-disable flows exist | Audit acceptance evidence, pagination decision and complete first-push Playwright coverage |
| Phase 3: operational hardening | Partially accepted | Named deployment profiles, request-body limits, authentication rate limits, trusted-proxy enforcement, production HTTPS/cookie validation, header/idle timeouts, graceful shutdown, private Distribution, a non-root Grom image, and UI-driven quiesced backup plus same-image recovery exist | Recorded restart CI evidence, key rotation, upgrade tests, Docker smoke test and release artifacts; expanded matrices follow advertised capabilities |
| Phase 4: integrations placeholder | Accepted for MVP | Backend-driven planned catalog and disabled read-only UI exist | ADR is required before active event delivery or scan-result storage, not for the inert placeholder |

`make test` passed on July 29, 2026, including backend tests, the SQLite
integration flow, frontend lint, 23 frontend tests, and TypeScript checking.
`make build` also passed on July 29, 2026. The PostgreSQL integration test
remains conditional on `GROM_TEST_POSTGRES_URL`; a container smoke test and
release acceptance run still need to be recorded.

### Update on August 2, 2026

The following completion work is now implemented and covered by automated
checks:

- OpenAPI validation uses kin-openapi in
  `backend/internal/httpapi/contract_test.go`.
- Registered management routes and documented operations are compared in both
  directions; `/v2/*` and optional documentation routes remain intentionally
  outside the management contract comparison.
- The frontend CI job regenerates
  `frontend/src/shared/api/generated/schema.d.ts` and fails on tracked drift.
- SQLite migrations use Bun's mark-on-success behavior; failed migrations are
  not recorded as applied.
- `backend/internal/platform/database/database_test.go` covers migration
  failure handling, and `backend/tests/integration/core_test.go` covers
  SQLite state preservation across a close/reopen restart.
- Audit service serialization, store failures, SQLite persistence,
  idempotent restore-style events, and sensitive-metadata assertions are
  covered by the audit and HTTP tests.

The August 2 verification run passed `make test`, `make test-coverage`, and
`make build`. Frontend line coverage was 70.3%; the repository Codecov gate
remains 70% patch coverage with 1% tolerance.

### Execution order for the self-hosted MVP

The numbered completion steps below remain the full work-package inventory.
They are executed in this product-priority order:

| Priority | Delivery slice | Required result |
|---:|---|---|
| 1 | Documentation and acceptance alignment | Architecture, product features, code map, agent guidance, and MVP matrix agree with accepted evidence |
| 2 | Contract and CI hardening | OpenAPI validation, Go/TypeScript freshness, bidirectional route checks, and mandatory coverage evidence |
| 3 | Restart and migration acceptance | Failed migrations block readiness; restarts preserve the supported SQLite/local-storage installation |
| 4 | Essential audit acceptance | Sanitized events persist for authentication, credentials, memberships, projects, policies, backups, and destructive operations |
| 5 | Core administration UX | Complete the first-push journey, membership management, user disabling, key expiration, push guidance, and essential manifest detail |
| 6 | Release engineering | Clean install, restart, upgrade, backup/restore, image publication, and operator documentation |

PostgreSQL CI, S3 compatibility, extended ORAS/referrer coverage, full OCI
conformance, an audit browsing UI, and advanced release attestations are
second-tier gates. They become release blockers only when the corresponding
capability is advertised as supported.

### Immediate implementation adjustment: deployment profiles

**Status: implemented on July 29, 2026.**

The implementation exposes
`GROM_DEPLOYMENT_PROFILE=development|permissive|strict`, with `strict` as the
default when the variable is absent:

| Profile | Intended use | HTTP and cookies | Default data path |
|---|---|---|---|
| `development` | Local coding and tests | HTTP allowed only on loopback; secure cookies optional | SQLite and local blobs |
| `permissive` | Trusted private LAN, single instance | HTTPS recommended; private-address HTTP requires explicit opt-in and a startup warning | SQLite and local blobs |
| `strict` (default) | Internet-facing or organizational use | HTTPS and secure cookies required; invalid configuration fails startup | SQLite/local by default; PostgreSQL or S3 only when explicitly supported |

Rules common to all profiles:

1. Distribution remains private and Grom remains the only public entry point.
2. Rate limiting, CSRF protection, secret hashing, request limits, and
   sanitized logging remain enabled.
3. Forwarded client IPs are trusted only from explicit
   `GROM_TRUSTED_PROXIES` ranges.
4. `GROM_ALLOW_INSECURE_PRIVATE_HTTP=true` is valid only in the `permissive`
   profile and only for loopback, RFC 1918 IPv4, link-local IPv4, IPv6
   unique-local/link-local addresses, or explicitly recognized local namespaces
   such as `.home.arpa` and `.local`. It must never authorize an arbitrary
   public address.
5. The UI and startup logs clearly identify an insecure permissive HTTP
   deployment.
6. Changing profiles must not silently weaken an existing production
   installation.

Acceptance:

- A localhost development installation continues to start over HTTP.
- A permissive installation can explicitly use a private-LAN HTTP address and
  receives a visible warning.
- Strict and public-address configurations fail startup without HTTPS and
  secure cookies.
- An omitted `GROM_DEPLOYMENT_PROFILE` resolves to `strict`; local development
  files set `development` explicitly.
- Tests cover private, loopback, public, malformed, and proxy-fronted
  configurations.

Evidence:

- `backend/internal/platform/config/config_test.go` covers strict defaults,
  loopback development, RFC 1918 and IPv6 permissive addresses, local
  namespaces, public-address rejection, secure cookies, and invalid opt-ins.
- Startup logs include the selected profile and emit a warning for explicitly
  permitted HTTP.
- `GET /api/v1/deployment` exposes only the non-sensitive profile and warning
  state; `frontend/src/app/App.vue` displays the warning on public and
  authenticated screens.
- `.env.example` and shipped Compose set `development` explicitly.

### Completion step 1: close security-boundary gaps

**Status: implemented on July 28, 2026; release-level container and ingress
verification remains part of steps 7–9.**

Primary areas:

- `backend/internal/httpapi`
- `backend/internal/platform/config`
- `backend/cmd/grom`
- security-focused tests and deployment configuration

Work:

1. Implement bounded in-process rate limiting for failed sign-in and
   `/auth/token` authentication attempts.
2. Define rate-limit keys that do not trust arbitrary client-supplied proxy
   headers.
3. Replace unconditional proxy-header trust with explicit
   `GROM_TRUSTED_PROXIES` IP/CIDR configuration.
4. Apply real-client-IP rewriting only when the immediate peer is trusted.
5. Confirm and document the CSRF model for cookie-authenticated mutations,
   including the behavior of requests with a missing `Origin`.
6. Fail or warn clearly when production-like configuration uses insecure
   cookies or an HTTP public URL.
7. Add security tests for limiter behavior, proxy spoofing, Origin validation,
   cookie flags, and sanitized authentication failures.

Acceptance:

- Repeated failed authentication is throttled without affecting successful
  requests from unrelated clients.
- An untrusted caller cannot control the logged or rate-limited client IP using
  `X-Forwarded-For` or related headers.
- State-changing cross-origin browser requests are rejected by the documented
  CSRF model.
- Production deployment documentation requires HTTPS and secure cookies.
- Secrets and Authorization headers remain absent from application logs and
  errors.

Evidence:

- `backend/internal/httpapi/server_test.go` covers limiter isolation and
  boundedness, proxy spoofing, Origin behavior, cookie flags, and access-log
  sanitization.
- `backend/internal/platform/config/config_test.go` covers local defaults,
  production HTTPS/secure-cookie enforcement, explicit trusted proxies, and
  rejection of the legacy unbounded proxy switch.
- OpenAPI declares `429 Too Many Requests` with `Retry-After` for both
  authentication endpoints.

### Completion step 2: complete required audit coverage

**Status: event production and SQLite persistence acceptance are implemented;
public end-to-end and remaining destructive-operation failure-path evidence
remain open.**

Primary areas:

- `backend/internal/constants/audit.go`
- owning application services and HTTP handlers
- `backend/internal/audit`
- optional post-MVP OpenAPI read contract and frontend presentation

Work:

1. Record successful and failed sign-in events without recording credentials.
2. Record user creation and security-relevant user administration.
3. Record service-account creation and disabling.
4. Record access-key creation and revocation without recording the secret.
5. Record project creation and deletion.
6. Record membership creation, role replacement, and removal.
7. Preserve the existing password, repository-policy, deletion, and lifecycle
   events.
8. Test event action, actor, resource, timestamp, and sanitized metadata.
9. After the persistence gate passes, define an authorized, paginated
   audit-event listing contract.
10. Treat the installation-admin audit page as a post-MVP capability unless it
    is explicitly promoted into the release scope.

Evidence:

- `backend/internal/httpapi/server_test.go` covers authentication,
  user/service-account administration, access-key changes, memberships,
  project lifecycle, user disabling, and sanitized metadata assertions.
- `backend/internal/audit/application/service_test.go` covers structured event
  creation, metadata serialization failures, and store error propagation.
- `backend/internal/audit/infrastructure/persistence/bun/store_test.go` covers
  SQLite persistence and idempotent `RecordOnce` behavior.

Acceptance:

- Every security-sensitive management action named in the security baseline
  creates an immutable audit event.
- Audit failures cannot silently permit destructive registry operations that
  require durable evidence.
- Plaintext passwords, reset tokens, access keys, sessions, and Authorization
  values never enter audit metadata.
- Essential audit persistence does not depend on shipping an audit browsing UI.

### Completion step 3: enforce contract-first delivery

**Status: implemented for validation, freshness, and bidirectional route
coverage; breaking-change detection remains open until compatibility-guaranteed
API releases begin.**

Primary areas:

- `backend/api/openapi.yaml`
- `backend/oapi-codegen.yaml`
- `backend/internal/httpapi`
- `frontend/src/shared/api/generated`
- build scripts and CI

Current generation intentionally produces Go transport models and TypeScript
types. HTTP routing and handler conformance are implemented manually. Generated
Go server interfaces are not required for the MVP unless a later decision
changes this workflow.

Work:

1. Validate the document deterministically with kin-openapi.
2. Verify generation freshness for Go and TypeScript output.
3. Run a route-to-contract test proving every implemented `/api/v1/*` and
   `/auth/token` operation is documented.
4. Run a contract-to-route test proving required operations are registered.
5. Keep implementation/response contract tests for representative success and
   error variants.
6. Make validation, freshness, route coverage, and frontend type checks
   mandatory in CI; the remaining breaking-change check is deferred.
7. Add breaking-change detection against the main branch when Grom begins
   publishing a compatibility-guaranteed API.
8. Document the versioning and release-note process before the first
   compatibility-guaranteed API release.

Acceptance:

- MVP CI fails for invalid OpenAPI, stale generated output, an undocumented
  route, or an unregistered required operation.
- Domain entities remain independent from generated transport models.
- Frontend server-owned enums continue to come from generated OpenAPI types.
- `/api/openapi.yaml` and `/api/docs` represent the contract used to build the
  release.

### Completion step 4: establish the mandatory CI matrix

**Status: partially implemented. Mandatory backend, frontend, golangci-lint,
govulncheck, registry, administrative-browser, backup/restore, and boot
acceptance jobs are implemented; remaining matrix items below are still
open.**

Work:

1. Add CI jobs for Go formatting and tests.
2. Add frontend lint, tests, TypeScript checking, and production build.
3. Run migrations and repository integration tests against SQLite.
4. Provision PostgreSQL and run the same applicable migration and repository
   suite with `GROM_TEST_POSTGRES_URL` before PostgreSQL is advertised as fully
   supported.
5. Run the OpenAPI validation, freshness, and route checks from completion step 3.
6. Build the production container image.
7. Run dependency and container vulnerability scanning for published images;
   do not block a local development build on remote scanner availability.
8. Cache dependencies without caching generated artifacts as authoritative
   source.
9. Protect the main branch with all required checks.

Acceptance:

- SQLite is tested on every relevant change.
- When PostgreSQL is advertised as supported, its job is mandatory and a
  skipped test cannot make that database job green.
- The production image builds from a clean checkout.
- Generated-code drift and dependency vulnerabilities are visible before
  merge.

Current evidence:

- `.github/workflows/ci.yml` runs stable `Backend Tests` and `Frontend Tests`
  checks on pull requests, `main`, merge queues, and manual dispatch. Backend
  checks enforce `gofmt` and run Go tests with an atomic coverage profile;
  frontend checks use `npm ci` and run lint, Vitest with LCOV coverage,
  TypeScript checking, and the production Vite build. Both jobs upload their
  reports to Codecov with separate `backend` and `frontend` flags through the
  repository's `CODECOV_TOKEN` Actions secret. `codecov.yml` requires 70%
  patch coverage with 1% tolerance and excludes only generated OpenAPI code
  and static frontend assets.
- The same workflow runs stable `Go Lint` and `Go Vulnerability Check` jobs.
  golangci-lint 2.11.4 performs static analysis, while the official Go
  govulncheck action uses text output so reachable known vulnerabilities fail
  the check.
- Enabling the vulnerability gate identified reachable standard-library
  vulnerability `GO-2026-5856`; `backend/go.mod` and the production Docker
  builder now require Go 1.26.5, the fixed release.
- `.github/workflows/registry-e2e.yml` runs the stable
  `Registry E2E (Docker)` check on pull requests, `main`, merge queues, and
  manual dispatch using Docker Engine and Compose on `ubuntu-latest`.
- The job invokes `make test-registry-e2e`, whose explicit opt-in prevents the
  registry package from being counted as a successful skip.
- `.github/workflows/boot-acceptance-e2e.yml` runs the stable `Boot Acceptance
  E2E (Docker)` check on pull requests, `main`, merge queues, and manual
  dispatch. The command passed locally and the mandatory CI check passed on
  August 3, 2026 in [PR #16](https://github.com/jfxdev/grom-registry/pull/16).
- The active `main` branch ruleset is configured to require `Backend Tests`, `Frontend Tests`,
  `Go Lint`, `Go Vulnerability Check`, `Registry E2E (Docker)`,
  `Admin Journey E2E (Docker)`, `Boot Acceptance E2E (Docker)`, and
  `Backup Restore E2E`; branch-protection state is not stored in workflow YAML.
- Backend tests validate the OpenAPI document and compare all registered
  management routes with the contract in both directions. The frontend job
  regenerates TypeScript API types and fails on tracked drift.

### Completion step 5: prove registry behavior end to end

**Status: Docker image journey implemented and accepted on July 29, 2026.
ORAS remains deferred until generic OCI artifacts are advertised as fully
supported.**

The detailed implementation sequence, harness boundaries, scenario matrix, and
acceptance checklist are maintained in
[`registry-e2e-implementation-plan.md`](registry-e2e-implementation-plan.md).

Work:

1. Start Grom and Distribution in an isolated test environment.
2. Create projects `alpha` and `beta`.
3. Create Writer, Reader, and unauthorized service-account scenarios.
4. Use Docker to push and pull allowed content.
5. Prove cross-project pull and push denial.
6. Prove Reader pull and push denial.
7. Revoke an access key and prove the next token exchange fails.
8. Remove a membership and prove the next token exchange loses access.
9. Let a short-lived registry JWT expire and prove the long-lived access key
   can obtain a new JWT.
10. Add a representative ORAS push/pull smoke test before generic OCI artifacts
    are advertised as fully supported; keep extended referrer coverage as a
    second-tier compatibility gate.
11. Prove policy enforcement for immutable, protected, and invalid tags.
12. Prove successful pushes appear in repository, tag, and inventory reads.

Acceptance:

- Phase 1's Docker authorization exit criterion passes automatically.
- Docker Engine can push and pull the primary supported image content.
- ORAS can push and pull representative generic OCI content when that
  capability is included in the release claim.

Evidence:

- `backend/tests/registrye2e` compiles with `go test ./...` and skips unless
  `GROM_RUN_REGISTRY_E2E=1` is explicitly set.
- `make test-registry-e2e` starts a uniquely named Compose project on a random
  loopback port and cleans its containers, volumes, temporary credential
  directories, and exact image tags.
- The July 29, 2026 acceptance run passed against Docker Engine 29.6.1 and
  Distribution 3.1.1 in 92 seconds. It covered first-push provisioning, Writer
  and Reader pulls, Reader push denial, cross-project denial with equivalent
  missing/existing posture, JWT expiry and refresh, access-key revocation,
  membership removal, tag protection, immutability, tag naming, and repository,
  tag, manifest-inventory, classification, and profile observation.
- Test images are deterministic `scratch` builds; the harness uses only public
  management and `/v2` endpoints and never reaches the database or private
  Distribution port.
- Denied operations do not reveal private project or repository existence.
- Tests exercise the public Grom entry point and cannot bypass Distribution
  authorization.

### Completion step 6: finish the Phase 2 management experience

**Planning update on August 3, 2026:** the detailed acceptance plan for the
remaining browser journey and boot/contract smoke checks is maintained in
[`mvp-acceptance-implementation-plan.md`](mvp-acceptance-implementation-plan.md).
It deliberately proves the installed public Grom surface rather than relying
on mocked frontend routes or direct database setup.

Work:

1. Maintain the repository manifest-detail experience showing digest, media
   type, size, known timestamps, tags, classification, and OCI relationships.
2. Maintain membership role replacement and removal in the UI.
3. Maintain optional access-key expiration during key creation.
4. Keep general user-profile editing post-MVP unless a concrete requirement
   promotes it.
5. Maintain copyable push guidance alongside the existing pull command.
6. Remove recent-audit overview and basic settings from the default MVP unless
   a concrete first-release use case promotes them back into scope.
7. Decide where pagination is required based on current unbounded list
   endpoints, then update OpenAPI, repositories, and UI together.
8. Add request-level frontend integration tests.
9. Add Playwright coverage for sign-in, project creation, membership
   management, reveal-once key handling, first push, repository browsing, safe
   deletion, and lifecycle review.
10. Keep archival blocking push while preserving pull and OCI content. Do not
   claim a race-safe logical-removal guarantee until catalog and inventory
   preconditions are revalidated inside one serialized removal operation.

Acceptance:

- A new administrator can complete the documented first-push workflow without
  database access or configuration edits.
- Every management action promised as MVP is either exposed in the UI or
  explicitly documented as API-only.
- A project containing repositories has an implemented archival/removal
  lifecycle instead of becoming permanently undeletable through product
  operations.
- Critical browser flows pass in Playwright.

### Completion step 7: complete operational hardening

**Backup and restore status: implemented and accepted on July 29, 2026.**

Work:

1. Add request and server timeouts that protect management traffic without
   interrupting streamed layer uploads.
2. Add and test a cross-process SQLite migration lock compatible with the
   single-instance deployment profile.
3. Test PostgreSQL advisory-lock timeout and recovery.
4. Maintain and periodically validate backup and restore for SQLite, signing
   keys, Distribution metadata, and local blob storage. Follow the detailed
   [`backup and disaster recovery implementation plan`](backup-and-disaster-recovery-implementation-plan.md).
5. Write and test signing-key rotation procedures, including Distribution trust
   updates and in-flight JWT behavior.
6. Add upgrade tests from every supported previous release.
7. Test restart and upgrade preservation of users, projects, memberships,
   repository inventory, lifecycle history, signing material, and blobs.
8. Test a supported S3-compatible configuration before S3 is advertised as
   fully supported.
9. Run OCI Distribution conformance tests before making broad conformance
   claims.
10. Keep the mandatory Docker registry journey in the default release gate and
    add ORAS smoke tests only to releases that advertise generic OCI support.
11. Document operator-driven Distribution garbage collection separately from
    manifest deletion.

Acceptance:

- Backup restoration produces an installation that can authenticate, browse,
  push, and pull preserved content.
- A failed or interrupted migration does not expose readiness or corrupt
  migration history.
- Concurrent startup cannot apply the same SQLite or PostgreSQL migration twice.
- Supported upgrades preserve metadata and blobs.
- Local storage passes the default release matrix. S3 passes its documented
  matrix before it is advertised as supported.

### Completion step 8: create release engineering outputs

Work:

1. Build a versioned, non-root production image.
2. Publish immutable image digests and human-readable version tags.
3. Generate release checksums.
4. Generate and publish an SBOM for publicly distributed releases.
5. Scan the final image and dependencies for publicly distributed releases.
6. Publish minimal installation, configuration, upgrade, rollback, backup, and
   restore documentation.
7. Record database and storage compatibility for the release.
8. Smoke-test a clean installation and an upgrade before publishing.

Acceptance:

- A tagged release can be installed and upgraded with preserved metadata and
  blobs.
- Public release artifacts are checksummed and, when distributed beyond local
  development, scanned and accompanied by an SBOM.
- Operators have enough documentation to install, back up, restore, rotate
  keys, upgrade, and diagnose readiness failures.

### Completion step 9: close architecture and MVP acceptance

Work:

1. Confirm integrations remain inert; require the event-delivery and
   scan-result-storage ADR before implementing active integrations, not as a
   blocker for the read-only MVP placeholder.
2. Execute every `Default MVP` scenario in the acceptance matrix.
3. Link each default scenario to an automated test or recorded manual release
   check.
4. Resolve every `partial`, `unverified`, or `missing` default entry, and
   resolve capability-specific entries for every capability advertised by the
   release.
5. Run `make test` and `make build` from a clean checkout.
6. Build and smoke-test the container through the public Grom port.
7. Update the phase table and acceptance matrix with the release date and
   evidence.
8. Remove stale planned paths, routes, and pages instead of leaving
   contradictions in this document.
9. Confirm `docs/code-map.md`, `docs/domain-model.md`, `docs/product-features.md`,
   and `AGENTS.md` still match the release.

Acceptance:

- Every default-path phase exit criterion is supported by evidence.
- Every `Default MVP` acceptance scenario is passing.
- Every capability-specific scenario for an advertised capability is passing.
- Architecture, OpenAPI, code map, domain inventory, agent guidance, and
  product-feature documentation agree with the released implementation.
- Remaining roadmap work is explicitly outside the MVP rather than silently
  incomplete.

## 1. Product goal

Grom is a small, fast, self-hosted OCI registry centered on Docker images and
operated by individuals or small teams. It adds a simple web interface, users,
service accounts with API access keys, and project-scoped permissions without
reimplementing the registry protocol or blob storage.

The first release targets a single installation and a single Grom instance.
It must be straightforward to install on one machine, safe to expose through a
properly configured reverse proxy, and recoverable from documented backups.
High availability, organizations, policy engines, vulnerability scanning,
replication, billing, enterprise identity providers, and compliance suites are
intentionally outside the MVP.

## 2. Product principles

1. **Reuse the registry engine.** CNCF Distribution handles the OCI/Docker Registry HTTP API, manifests, blobs, uploads, storage drivers, and garbage collection.
2. **Keep authorization explicit.** Every image name starts with a project slug: `<project>/<repository>`.
3. **Keep deployment small.** The default installation uses two containers,
   SQLite, and local filesystem storage; the PostgreSQL implementation is an
   optional capability with its own release gate.
4. **Keep secrets recoverable only by rotation.** Passwords and API tokens are stored as hashes; a token secret is shown only once.
5. **Do not build roadmap features early.** The Integrations page exposes planned capabilities, but the MVP does not execute scanners or manage integration secrets.
6. **Optimize the default path first.** SQLite, local blob storage, and Docker
   push/pull define the first supported release gate.
7. **Scale validation with the claim.** PostgreSQL, S3, ORAS, and generic OCI
   behavior become mandatory gates only when the release advertises them as
   supported rather than experimental.
8. **Permit an explicit private-network tradeoff.** Private-LAN HTTP may be
   allowed only by the `permissive` profile with a visible warning; public
   addresses and `strict` deployments require HTTPS.

## 3. Recommended architecture

```mermaid
flowchart LR
    C["Docker / OCI client"] --> G["Grom gateway"]
    B["Browser"] --> G
    G -->|"UI, /api/v1, /auth/token"| A["Go control plane"]
    G -->|"/v2/* streaming proxy"| R["CNCF Distribution v3"]
    A --> D[("SQLite / PostgreSQL")]
    A -->|"short-lived signed JWT"| C
    R --> S[("Local filesystem or S3")]
    R -->|"validate JWT with public key"| A
```

### Runtime components

- **Grom:** one Go binary containing the API, token service, streaming reverse proxy, and embedded frontend assets.
- **Distribution:** an unmodified `distribution/distribution` v3 registry process on the private container network.
- **Relational database:** SQLite by default or PostgreSQL, accessed through the Bun ORM, for users, memberships, service accounts, token hashes, sessions, and audit events.
- **Blob storage:** local filesystem by default; S3-compatible storage through Distribution configuration.

With SQLite, the default deployment still has only the Grom and Distribution containers.
PostgreSQL is an optional third service or an externally managed database.
Only Grom is exposed publicly.
Keeping Distribution private prevents bypassing project authorization.
TLS may terminate in the platform ingress or in an optional reverse proxy; production use requires HTTPS.

### Why not implement the OCI API in Grom

Implementing upload sessions, content addressing, manifest negotiation, resumable transfers, range requests, and garbage collection would create a second registry engine.
Distribution already implements these concerns and supports Docker clients.
Grom should remain a control plane and authentication layer.

## 4. Repository and image naming

All repositories use this shape:

```text
<registry-host>/<project-slug>/<repository-path>:<tag>
```

Examples:

```text
registry.example.com/payments/api:v1.4.0
registry.example.com/payments/workers/settlement:main
```

The first path segment is always the project slug.
Repository paths may contain additional segments.
Project slugs are immutable in the MVP so renaming a project never silently changes image references.

Projects are created only by installation administrators. Repositories may be
registered manually before their first push, storing the project boundary,
relative repository path, description, and selected behavior policies. A
Writer or Admin registry principal may also register an empty logical repository
when requesting push scope inside an existing project. The repository is created
before the token is signed, and the same token contains push so the first upload
can succeed. The auto-created repository has no policies. Pull requests never
create repositories. Distribution creates the physical repository content on
the first successful push. Existing Distribution repositories are reconciled
into Grom without policies so upgrades preserve access to existing content.
Project administrators can later replace the complete policy set under an
optimistic version, including for repositories created by push or reconciliation.

Repositories begin with an `unknown` content profile. Tagged primary manifests
are classified from explicit artifact type, config media type, layer media types,
or index descriptors. The first reliable observation passively infers a profile
such as `container_image`, `terraform_module`, `sbom`, or `generic_oci`.
Subject/referrer artifacts never change that profile. Later incompatible primary
content produces `mixed` and requires review. Inference is informational: it
never enables policies, rejects pushes, or changes authorization.

## 5. Identity and authorization model

### Principals

- **User:** a human account that can sign in to the web interface.
- **Service account:** a non-human account used by CI, CD, or automation.
- **API token:** a revocable credential owned exclusively by a service account.

A user authenticates to the web UI with email and password.
Docker/OCI clients authenticate with the service-account username and one of its API tokens as the password.
Web passwords are never accepted as registry credentials.

Service accounts belong to the installation and can be assigned to one or more projects.
This avoids introducing organizations or nested teams.

### Project roles

| Role | Pull | Push | Manage project and members |
|---|---:|---:|---:|
| Reader | Yes | No | No |
| Writer | Yes | Yes | No |
| Admin | Yes | Yes | Yes |

External registry deletion is disabled. Project and installation administrators
can delete manifests through Grom after a digest-and-alias preview, repository
policy evaluation, OCI relationship check, and immediate revalidation. Manual
deletions are persisted and audited. Subjects with referrers and referrer
artifacts remain protected; cascade deletion is not implemented. Registry
garbage collection remains an explicit operator action.

An installation administrator can manage users and service accounts.
Only an installation administrator can create a project and becomes its first
project admin. Only an installation administrator can delete a project, and only
after every logical repository has been removed. This prevents project deletion
from orphaning Distribution content or silently erasing repository inventory.

Every user has a dedicated profile page where they can change their password
after confirming the current password. Installation administrators reset a user
through a system-generated, reveal-once magic URL. Its random token is stored
only as a hash, expires after 30 minutes, becomes invalid when a newer link is
created, and can be consumed only once. The token is carried in the URL fragment
so it is not included in navigation requests or access logs. Completing the
reset replaces the password and revokes that user's active web sessions.
Authenticated users cannot open or consume a reset link; they must use the
current-password flow from their profile or sign out first.
Password changes, reset-link creation, and completed resets are audited without
recording plaintext credentials or reset tokens.

### Token design

Tokens use a recognizable structure such as `grm_<public-id>_<secret>`.
The public ID locates the record efficiently; only a password hash of the secret is stored.
Each token has a name, service-account owner, creation time, optional expiration time, last-used time, and revocation time.

Project access is derived from the token owner's current memberships.
Removing a principal from a project immediately removes access without rotating the token.
Tokens are created, displayed, and revoked only inside their owning service account in the UI. Multiple briefly overlapping keys are supported for safe rotation.

## 6. Registry authentication flow

1. A Docker client requests `/v2/` or a repository operation.
2. Distribution responds with a Bearer challenge pointing to Grom's `/auth/token`.
3. The Docker client calls `/auth/token` using Basic authentication, with the service-account username and API token.
4. Grom parses the requested repository scope and takes the first path segment as the project.
5. Grom intersects the requested actions with the principal's project role.
6. Grom returns a signed, short-lived registry JWT containing only the allowed actions.
7. The client retries the operation and Distribution validates the JWT using Grom's public key.

Registry bearer tokens should live for approximately five minutes.
API tokens remain revocable long-lived credentials, but they are never forwarded to Distribution.

Requested actions map as follows:

- `pull`: Reader, Writer, or Admin.
- `push`: Writer or Admin.
- `delete`: denied in the MVP.
- catalog-wide access: denied to external clients.

The authorization service must return a valid token containing only the permitted subset when a client requests mixed scopes.
Authentication failures and denied scopes must not reveal whether a private project or repository exists.

Push is additionally restricted to repositories registered in Grom. Manifest
PUT requests pass through the policy-aware gateway, which enforces tag naming,
immutability, and overwrite protection without buffering layer uploads.
External registry clients never receive `delete`; manual deletion is an
authenticated project-management operation that resolves the digest and every
tag alias before calling Distribution internally.

When a project Writer or Admin requests push scope, the token service
idempotently creates the missing empty logical repository before signing the
token. Push remains in that same token, so Docker can complete its first upload
without a preparatory request. Readers and pull-only token requests never create
repositories, and no registry operation creates a project.

Successful manifest pushes are observed by the gateway and recorded as a
metadata-only inventory. Repository administrators can reconcile that inventory,
create an expiring retention dry-run, and execute it manually. Every candidate is
reconciled with Distribution and evaluated against the current policy set
immediately before its individual deletion. Every outcome is persisted and
audited. A preview is atomically claimed and can create at most one run.
The preview records both the repository policy version and lifecycle evaluator
version used for its decisions. Stale execution locks fail the interrupted run
before a later run can proceed. Scheduled autopurge is not implemented.

## 7. Backend architecture and boundaries

Use pragmatic Domain-Driven Design with vertical bounded contexts.
DDD here means explicit domain rules and dependency direction, not aggregates or abstractions for every data structure.

### Repository root

```text
grom/
├── backend/             Go control plane and gateway
├── frontend/            Vue application
├── deploy/
│   ├── compose/
│   └── distribution/
├── docs/
│   ├── architecture-and-mvp.md
│   ├── code-map.md
│   ├── domain-model.md
│   ├── product-features.md
│   ├── visual-identity.md
│   └── visual-implementation-plan.md
├── AGENTS.md
├── Makefile
├── README.md
└── LICENSE
```

Backend and frontend remain independently buildable.
The frontend consumes the backend contract through generated OpenAPI types, not through a hand-maintained shared source folder.
The `.github/workflows` CI structure exists, with mandatory general CI and an
isolated real-Docker registry acceptance workflow.

### Bounded contexts

- **Identity:** users, sessions, service accounts, API tokens, password and credential policies.
- **Projects:** projects, memberships, roles, and project authorization.
- **Registry:** registry token grants, repository scope parsing, catalog reads, and the Distribution gateway.
- **Audit:** immutable security event recording; authorized querying is planned
  in completion step 2.
- **Integrations:** read-only integration catalog in the MVP.

Identity and Projects are separate contexts because a principal may exist without belonging to a project and memberships should not own identity credentials.
Registry asks Projects for authorization decisions instead of reading membership tables directly.

### Current backend directories

```text
backend/
├── api/
│   ├── embed.go
│   └── openapi.yaml
├── cmd/
│   └── grom/
│       └── main.go
├── internal/
│   ├── identity/
│   │   ├── domain/
│   │   ├── application/
│   │   └── infrastructure/
│   │       ├── persistence/bun/
│   │       └── password/
│   ├── projects/
│   │   ├── domain/
│   │   ├── application/
│   │   └── infrastructure/persistence/bun/
│   ├── registry/
│   │   ├── domain/
│   │   ├── application/
│   │   └── infrastructure/
│   │       ├── distribution/
│   │       ├── persistence/bun/
│   │       └── signing/
│   ├── audit/
│   │   ├── domain/
│   │   ├── application/
│   │   └── infrastructure/persistence/bun/
│   ├── integrations/
│   │   └── domain/
│   ├── httpapi/
│   ├── foundation/
│   ├── constants/
│   ├── generated/openapi/
│   ├── platform/
│   │   ├── config/
│   │   └── database/
│   └── webassets/
│       └── dist/
├── migrations/
├── tests/
│   └── integration/
├── go.mod
├── go.sum
└── oapi-codegen.yaml
```

The centralized `internal/httpapi` package owns route registration and HTTP
translation for the current MVP. It may be split into context-owned transport
packages only when current behavior justifies the move. Folders that would
contain only one trivial file are not created merely to match a theoretical
layout.

The generated Go package contains OpenAPI transport models only. Application
services and domain entities remain hand-written and independent from generated
models.

### Dependency rules

```mermaid
flowchart LR
    T["Transport / HTTP"] --> A["Application use cases"]
    A --> D["Domain"]
    I["Infrastructure adapters"] --> D
    I --> A
    P["Platform composition root"] --> T
    P --> I
```

- Domain packages contain entities, value objects, policies, domain services, repository interfaces, and domain errors.
- Domain packages import only the Go standard library and, when unavoidable, a very small shared-kernel package.
- Application packages orchestrate use cases, transactions, domain repositories, and external ports.
- HTTP handlers translate requests and responses; they do not contain authorization rules or issue ORM queries.
- Bun repository implementations live only under `infrastructure/persistence/bun`.
- Bun models may be persistence-specific structs when database shape differs from domain shape; explicit mappers keep ORM tags out of domain entities.
- `main.go` and `platform` form the composition root that chooses SQLite or PostgreSQL and wires concrete adapters into use cases.
- One bounded context must not import another context's infrastructure package.
- Cross-context calls use narrow application interfaces, not direct table access.
- Keep the `foundation` package limited to stable application-wide structs; do not create a generic `utils` domain.
- Keep code-wide named constants in the `constants` package, organized into domain-named files rather than one large file.

Repository interfaces are capability-oriented, for example `FindUserByUsername`, `SaveToken`, or `ListProjectMemberships`.
Avoid a generic `Repository[T]`, because it hides domain intent and usually leaks persistence concerns.

Prefer the Go standard library where it is sufficient.
Use a small router and the [Bun SQL-first ORM](https://github.com/uptrace/bun) behind repository implementations.
Use Bun's SQLite and PostgreSQL dialects, but keep domain and application logic unaware of the selected database.
Use explicit, versioned migrations and apply every pending migration automatically during application startup.
Migration code uses Bun's schema builder for portable operations and a clearly marked dialect branch only where the databases genuinely differ.
Avoid a message broker, Redis, a dependency-injection framework, and a background-job framework in the MVP.

### Canonical fundamental structs

Fundamental structs used by more than one bounded context are centralized in `backend/internal/foundation`.
This gives maintainers and future coding agents one predictable place to discover application-wide types.

Initial canonical structs:

```text
foundation.ID
foundation.PrincipalRef
foundation.PageRequest
foundation.PageResult[T]
foundation.Timestamps
foundation.FieldError
foundation.AppError
```

The package follows these rules:

- A struct enters `foundation` only when at least two bounded contexts use the same semantics.
- Fundamental structs contain no Bun, HTTP, JSON transport, or framework-specific behavior unless serialization is their explicit application-wide responsibility.
- Domain entities such as `User`, `Project`, `Membership`, `APIToken`, and `RepositoryScope` remain in their owning bounded contexts.
- Persistence structs remain beside their Bun repository implementations.
- HTTP request and response structs remain beside their handlers or in the owning context's application DTO package.
- A type must not be copied into another package merely to avoid importing `foundation`.
- Renaming or changing the semantics of a fundamental struct requires updating `docs/domain-model.md`.

`docs/domain-model.md` is the canonical inventory of fundamental structs, domain entities, value objects, ownership, and important relationships.
`docs/code-map.md` maps bounded contexts to their entry points, repositories, routes, migrations, and frontend modules.
Both documents are updated in the same change whenever an architectural type or module is introduced, moved, or removed.

### AGENTS.md as operational memory

The root `AGENTS.md` is the mandatory operational entry point for coding agents working in the repository.
It contains concise, current instructions needed to change the code safely without rereading the entire project history.

Add or update `AGENTS.md` whenever a change introduces information that future agents need, including:

- architecture boundaries and dependency rules;
- canonical locations for fundamental structs and constants;
- build, generation, lint, test, migration, and local-run commands;
- SQLite/PostgreSQL compatibility requirements;
- OpenAPI source-of-truth and generation workflow;
- security constraints and files that must not contain secrets;
- required verification for backend, frontend, registry, and migration changes;
- generated-file ownership and files that must not be edited manually;
- known non-obvious pitfalls and temporary compatibility constraints.

`AGENTS.md` stays concise and points to deeper documents such as `docs/code-map.md`, `docs/domain-model.md`, and this plan.
It must describe the repository as it currently works, not only the intended end state.
Every structural change includes an explicit check asking whether `AGENTS.md` also needs an update.
Obsolete instructions are removed rather than accumulated.

### Central constants packages

Backend constants live in `backend/internal/constants`.
The package centralizes stable named values such as:

- project roles and principal kinds;
- registry scope actions and token claims;
- session, token, and migration defaults;
- integration keys and statuses;
- header names, context keys, and application limits.

Rules for backend constants:

- No repeated protocol, role, status, or configuration-default string literals outside the package.
- Organize constants by concern in separate files.
- Use typed constants where they prevent invalid comparisons.
- Do not put environment-specific values, secrets, database values, translated labels, or values used only once in the constants package.
- Domain validation still belongs to the domain; a constant does not replace a value object or policy.
- Tests verify critical protocol values and prevent accidental changes to persisted or externally visible strings.

Values shared with the frontend have the backend/OpenAPI schema as their source of truth.
Roles, statuses, and enum-like API values are generated into the frontend client; they are not manually maintained twice.

### Migration boot lifecycle

Grom follows this startup order:

1. Load and validate configuration.
2. Open the database and verify connectivity.
3. Acquire the migration lock.
4. Read the versioned migration history.
5. Apply every pending migration in order.
6. Release the migration lock.
7. Build the application dependencies.
8. Start the HTTP server and report readiness.

The service must not accept HTTP or registry traffic before migrations finish.
If a migration fails, startup fails with a non-zero exit code and a sanitized error; Grom must not continue against a partially compatible schema.
Orchestrators can restart the process after the operator fixes the underlying problem.

Automatic migration means executing committed, reviewed migration files.
Grom does not use ORM model inspection, `AutoMigrate`, or runtime schema diffing.

Migration safety rules:

- Keep an append-only migration history; never edit a migration already included in a release.
- Execute a migration transactionally when the selected database supports all of its statements in a transaction.
- Record the migration version only after it completes successfully.
- Serialize migration execution so two starting processes cannot apply the same version concurrently.
- Use a PostgreSQL advisory lock for PostgreSQL and a database/file lock compatible with SQLite.
- Set a configurable migration lock timeout and fail startup when it expires.
- Backward migrations exist for development where practical, but application boot only migrates forward.
- Destructive or long-running data transformations require an explicit release note and a staged expand/migrate/contract strategy.
- Log migration version, duration, and outcome without logging connection credentials.

Implementation evidence:

- `backend/internal/platform/database/database.go` configures Bun with
  `WithMarkAppliedOnSuccess(true)`, so a failed migration is not recorded as
  applied.
- `backend/internal/platform/database/database_test.go` verifies a migration
  failure propagates and leaves no applied migration record.
- `backend/tests/integration/core_test.go` verifies that a SQLite restart
  preserves the bootstrapped administrator and project state.
- `backend/tests/registrye2e` performs a public, full-stack Grom and
  Distribution restart after a fixture push, then proves persisted management
  state, Writer authorization, blob pull, and a new push. It passed locally on
  August 3, 2026; mandatory CI evidence remains required.

Current gap: PostgreSQL uses an advisory lock, but SQLite currently uses only an
in-process mutex. Completion step 7 must add cross-process SQLite coordination
before the migration-lock requirement is considered accepted.

### Database configuration and portability

Grom accepts one database URL:

```text
sqlite:///data/grom.db
postgres://grom:password@postgres:5432/grom?sslmode=require
```

The URL selects the Bun dialect and driver.
SQLite remains the default for a single-instance installation.
PostgreSQL is recommended for installations that need managed backups, stronger concurrent-write behavior, or a future path to multiple Grom replicas.

Supporting both databases requires more than changing the driver, so the following rules are part of the architecture:

- Use portable column types in shared models: strings, bytes, booleans, integers, and timestamps.
- Store identifiers as application-generated UUID strings consistently in both databases.
- Store structured metadata as JSON text unless a repository has an explicitly tested dialect-specific implementation.
- Do not put PostgreSQL arrays, enums, extensions, triggers, or `JSONB` operators in shared queries.
- Do not depend on SQLite row IDs, permissive typing, or database-specific date functions.
- Keep transactions short and use database constraints as the final integrity boundary.
- Maintain one logical migration sequence with dialect-specific statements only when unavoidable.
- Run repository and migration integration tests against SQLite in default CI.
- Make equivalent PostgreSQL integration tests mandatory before a release
  advertises PostgreSQL as supported.

Current gap: SQLite is exercised by `make test`; PostgreSQL coverage is
conditional and is not yet enforced by CI.

### Management and protocol entry points

```text
GET    /healthz
GET    /readyz

POST   /api/v1/session
DELETE /api/v1/session
GET    /api/v1/me
PUT    /api/v1/me/password
POST   /api/v1/password-resets

GET    /api/v1/users
POST   /api/v1/users
DELETE /api/v1/users/{id}
POST   /api/v1/users/{id}/password-reset-link

GET    /api/v1/service-accounts
POST   /api/v1/service-accounts
DELETE /api/v1/service-accounts/{id}
GET    /api/v1/service-accounts/{id}/tokens
POST   /api/v1/service-accounts/{id}/tokens
DELETE /api/v1/service-accounts/{id}/tokens/{tokenId}

GET    /api/v1/projects
POST   /api/v1/projects
GET    /api/v1/projects/{project}
DELETE /api/v1/projects/{project}
GET    /api/v1/projects/{project}/members
PUT    /api/v1/projects/{project}/members/{principalKind}/{principalId}
DELETE /api/v1/projects/{project}/members/{principalKind}/{principalId}

GET    /api/v1/projects/{project}/repositories
POST   /api/v1/projects/{project}/repositories
GET    /api/v1/projects/{project}/repositories/{repositoryId}/policies
PUT    /api/v1/projects/{project}/repositories/{repositoryId}/policies
GET    /api/v1/registry-policy-presets
GET    /api/v1/projects/{project}/repository-tags

POST   /api/v1/projects/{project}/artifact-deletion-previews
GET    /api/v1/projects/{project}/artifact-deletions
POST   /api/v1/projects/{project}/artifact-deletions

GET    /api/v1/projects/{project}/repository-inventory
POST   /api/v1/projects/{project}/repository-inventory-reconciliations

POST   /api/v1/projects/{project}/lifecycle-previews
GET    /api/v1/projects/{project}/lifecycle-previews/{previewId}
GET    /api/v1/projects/{project}/lifecycle-runs
POST   /api/v1/projects/{project}/lifecycle-runs
GET    /api/v1/projects/{project}/lifecycle-runs/{runId}

GET    /api/v1/integrations
GET    /api/v1/integrations/{key}

GET    /auth/token
GET    /api/openapi.yaml        (when API documentation is enabled)
GET    /api/docs                (when API documentation is enabled)
ANY    /v2/*
```

`backend/api/openapi.yaml` is authoritative for management, health, readiness,
and token endpoints. `/v2/*` is a streaming gateway governed by the external OCI
Distribution specifications and is not reproduced operation-by-operation in
OpenAPI.

Catalog and reconciliation endpoints query Distribution on demand with a
short-lived internal credential. Grom persists repository control-plane metadata,
behavior policies, tag-to-digest observations, and lifecycle history. Manifest
payloads, layers, and blobs remain owned exclusively by Distribution.

### OpenAPI contract

`backend/api/openapi.yaml` is the source of truth for Grom's management and authentication HTTP API.
OpenAPI 3.0.x remains intentional because the selected generation tools support
it reliably.
The contract documents every `/api/v1/*` endpoint, health/readiness, and
`/auth/token`.
The `/v2/*` proxy is identified and linked to the external OCI/Distribution specifications instead of duplicating the complete registry protocol.

Contract-first workflow:

1. Change and review the OpenAPI contract.
2. Lint and validate the document.
3. Generate Go transport models into `backend/internal/generated/openapi`.
4. Implement or adapt the owning application service and centralized HTTP
   handler.
5. Generate frontend API types into `frontend/src/shared/api/generated`.
6. Run route/contract tests, backend tests, and frontend type checks.
7. Fail CI if generated output is stale, a route is undocumented, or a required
   operation is not registered.

Generated files contain a clear generated-code header and are never edited manually.
Domain entities and application commands are not generated from OpenAPI schemas.
HTTP handlers map requests and domain/application results without coupling
domain entities to generated transport models. Generated Go server interfaces
are not part of the current MVP workflow.

Each operation defines:

- a stable and unique `operationId`;
- tags matching its bounded context;
- path, query, header, and body schemas;
- authentication and authorization expectations;
- success and relevant error responses;
- the shared structured error schema;
- pagination behavior where applicable;
- validation limits and representative examples;
- externally visible enums such as roles, principal kinds, actions, and integration statuses.

Serve the rendered interactive documentation at `/api/docs` and the raw contract at `/api/openapi.yaml`.
Neither endpoint includes secrets or environment-specific server URLs.
Documentation endpoints may be disabled by configuration in hardened deployments without disabling the API itself.

CI validates the OpenAPI document with kin-openapi, regenerates the Go transport
output, checks frontend generated-code freshness, and exercises bidirectional
route coverage. Breaking-change detection remains open until the API has a
compatibility-guaranteed release policy.

An intentional breaking change requires an API versioning decision and release note.

## 8. Data model

Core tables:

```text
users
  id, email, username, password_hash, is_system_admin, created_at, disabled_at

service_accounts
  id, name, username, description, created_at, disabled_at

projects
  id, slug, name, created_by, created_at

project_memberships
  project_id, principal_type, principal_id, role, created_at

api_tokens
  id, public_id, principal_type, principal_id, name, secret_hash,
  created_at, expires_at, last_used_at, revoked_at

sessions
  id, user_id, secret_hash, expires_at, created_at

audit_events
  id, actor_type, actor_id, action, resource_type, resource_id,
  metadata_json, created_at

registry_repositories
  id, project_id, name, description, status, profile, profile_source,
  profile_confidence, profile_inferred_at, profile_needs_review, creation_source,
  created_at, updated_at

repository_policies
  id, repository_id, type, configuration_json, enabled, version,
  created_at, updated_at

registry_manifests
  id, repository_id, digest, media_type, artifact_type, subject_digest,
  observed_kind, artifact_relationship, classification_source,
  classification_confidence, manifest_size, state, first_seen_at,
  last_pushed_at, last_seen_at, untagged_at, deleted_at

registry_tags
  repository_id, name, manifest_id, first_seen_at, last_moved_at,
  last_seen_at, detached_at

lifecycle_previews / lifecycle_preview_items
  reconciled, expiring retention decisions and expected digest aliases

lifecycle_runs / lifecycle_run_items
  actor, reason, status, per-digest deletion outcome, and timestamps
```

Use foreign keys, uniqueness constraints for usernames and project slugs, and a unique membership per principal/project.
The API-token application model exposes `service_account_id`; the existing `principal_type` and `principal_id`
storage columns are retained for migration compatibility, with `principal_type` always set to `service_account`.
Avoid soft deletion except where disabling or revoking is security-relevant.
All model access goes through Bun repositories.
HTTP handlers, application use cases, and domain services do not issue SQL directly.

## 9. Frontend architecture

Use Vue 3, TypeScript, Vite, Vue Router, shadcn-vue, TanStack Query for server state, and Pinia only for genuinely shared client state such as the active session.
Generate typed API contracts from the backend OpenAPI document so request and response shapes do not drift.

The current frontend is organized by product feature rather than by global
technical type:

```text
frontend/
├── src/
│   ├── app/
│   │   ├── App.vue
│   │   ├── main.ts
│   │   ├── router/
│   │   └── styles/
│   ├── assets/
│   │   ├── icons/
│   │   ├── logos/
│   │   └── raw/
│   ├── modules/
│   │   ├── auth/
│   │   ├── projects/
│   │   ├── registry/
│   │   ├── service-accounts/
│   │   ├── users/
│   │   └── integrations/
│   ├── shared/
│   │   ├── api/
│   │   │   ├── client.ts
│   │   │   └── generated/
│   │   ├── components/
│   │   │   ├── brand/
│   │   │   └── ui/
│   │   ├── constants/
│   │   └── lib/
├── public/
├── components.json
├── vite.config.ts
└── package.json
```

Audit presentation, settings, and shared test factories are not currently
implemented. Audit presentation and settings are post-MVP by default.
`frontend/e2e` contains both the mocked sign-in smoke and an isolated public
stack journey for the core administration flow; broader browser coverage
remains completion-step work.

Each module owns its API query definitions, feature-specific components, validation schemas, pages, and route declarations.
An `index.ts` file is the module's public API; other modules should not import its internal files.
Move code to `shared` only after it is reused by multiple modules and has no product-specific behavior.
Keep shadcn-vue generated primitives under `shared/components/ui` and compose them into product components inside modules.
Application-wide frontend constants live under `shared/constants` and are exported through its `index.ts`.
API types come from `shared/api/generated` and their convenient aliases live in
`shared/api/models.ts`. Frontend-only types remain beside their owning feature
until genuine cross-feature reuse justifies a shared location.

Frontend constant rules:

- Route names, local-storage keys, application-wide UI limits, and stable UI identifiers are centralized.
- API roles, statuses, actions, and other server-owned enums come from generated OpenAPI types.
- Translated text, CSS classes, dynamic configuration, and values used by only one module are not global constants.
- Feature-specific constants stay in their module until they become truly application-wide.
- Components and composables do not repeat route names, storage keys, or API enum strings as literals.

### Frontend state and data rules

- TanStack Query owns remote data, caching, retries, invalidation, and loading/error states.
- Pinia does not duplicate server responses; it is reserved for cross-route client state.
- Component-local state stays in the component or a feature composable.
- Query keys are defined by their owning module and include project/repository identifiers.
- Mutations invalidate only the affected queries rather than clearing the entire cache.
- Route guards enforce navigation experience, but the backend remains the authorization authority.
- Forms use typed schemas for client feedback; backend validation errors remain authoritative and map to fields when possible.
- Dates are transported as ISO 8601 UTC and formatted only at the presentation boundary.
- API errors use one shared normalized shape with a request ID for support and logs.

### Frontend quality baseline

The following remains the required release baseline:

- Strict TypeScript and `vue-tsc` with no routine use of `any`.
- ESLint and formatting checks in CI.
- Vitest and Vue Testing Library for composables and user-visible component behavior.
- Mock Service Worker or equivalent request-level mocks for frontend integration tests.
- Playwright for critical flows: sign-in, project creation, membership management, token reveal-once, and repository browsing.
- Accessible labels, keyboard navigation, visible focus, reduced-motion support, and contrast checks.
- Route-level lazy loading and intentional bundle splitting.
- Error and empty states at page level; skeletons only when they improve perceived loading.
- No business rules hidden in visual components.
- No wrapper components around shadcn-vue primitives unless they add a stable Grom-specific behavior.
- CI checks generated API types for drift and rejects hand-written duplicates of server-owned enums during review.

Current gaps:

- CI enforces frontend lint, tests, typechecking, and the production frontend
  build.
- Tests use component and application mocks, but not a request-level frontend
  integration layer such as Mock Service Worker.
- Playwright covers mocked invalid sign-in and the real public-stack flow for
  project creation, service-account key reveal, Writer membership, first push,
  repository browsing, inventory, and manifest detail. The local acceptance
  command and the `Admin Journey E2E (Docker)` GitHub Actions check passed on
  August 3, 2026; the recorded CI evidence is
  [PR #6](https://github.com/jfxdev/grom-registry/pull/6).
- Generated API drift is checked in CI for both Go and TypeScript outputs.
- The full responsive and accessibility acceptance matrix is tracked in
  `visual-implementation-plan.md`.

MVP page status:

| Page or experience | Status | Remaining work |
|---|---|---|
| Sign in | Implemented | Complete final responsive and accessibility acceptance |
| Password reset and user profile | Implemented | Complete final responsive and accessibility acceptance |
| Overview with projects and recent audit events | Post-MVP | Do not block the default self-hosted release |
| Projects list and create project | Implemented | Broaden browser coverage beyond the first-push journey |
| Project detail with repositories and members | Implemented | Broaden browser coverage for edit/removal and error states |
| Repository detail | Implemented | Broaden browser coverage and add config/layer/pull-history data only if promoted |
| Users for installation administrators | Implemented with gaps | Keep general profile editing post-MVP |
| Service accounts and nested access keys | Implemented | Broaden browser coverage for expiration, revocation, and error states |
| Audit log | Post-MVP | Essential event persistence remains required; browsing UI does not block MVP |
| Integrations | Implemented as read-only roadmap | ADR required only before active integration work |
| Basic settings | Post-MVP | Add only when concrete settings cannot be handled safely through deployment configuration |

Accessibility, empty states, clear permission errors, and copy-to-clipboard commands are part of the MVP, not polish for later.

## 10. Integrations page: MVP contract, roadmap implementation

The Integrations page exists in the MVP, but it is read-only.
It fetches a backend-owned catalog so the frontend does not hard-code product behavior.

Initial cards:

- Trivy.
- Docker Scout or another Docker-compatible scanner.
- Generic OCI artifact scanner.
- Generic webhook.

Each card may expose:

```json
{
  "key": "trivy",
  "name": "Trivy",
  "category": "security",
  "status": "planned",
  "capabilities": ["scan-on-push", "manual-scan"],
  "documentationUrl": null
}
```

The backend defines catalog descriptor types and the current static catalog, but
no provider interface, provider implementation, secret storage, callback
endpoint, job execution, or scan-result schema is implemented in the MVP.
Buttons display `Coming soon` and cannot save configuration.
This is enough to preserve an intentional extension point without pretending the integration exists.

## 11. Security baseline

| Requirement | Status | Completion requirement |
|---|---|---|
| Require HTTPS for strict and public addresses | Implemented for the current strict mode | Preserve strict behavior while adding the explicit private-LAN permissive exception |
| Hash passwords and access-key secrets with versioned Argon2id parameters | Implemented | Preserve compatibility and add parameter-upgrade policy when values change |
| Use secure, HTTP-only, SameSite cookies | Implemented for HTTPS deployments | Permissive private-LAN HTTP is an explicit insecure exception; strict always requires secure cookies |
| Protect state-changing browser requests against CSRF | Implemented | Foreign origins and session-cookie mutations without `Origin` are rejected |
| Sign registry JWTs asymmetrically | Implemented | Complete signing-key rotation procedure |
| Never log credentials or Authorization headers | Implemented | Access logging records selected request metadata and has a credential-regression test |
| Rate-limit sign-in and `/auth/token` failures | Implemented | Bounded per-client in-process limiter with configurable window and block duration |
| Return access-key secrets once and rotate when lost | Implemented | Add end-to-end reveal-once and rotation coverage |
| Audit sign-in, key changes, memberships, and service-account changes | Partial | Add durable persistence, sanitization, and failure-path acceptance coverage |
| Run containers as non-root where supported | Partial | Grom image is non-root; verify and document Distribution runtime behavior |
| Keep the Distribution port private | Implemented in shipped Compose | Preserve in every deployment profile |
| Trust proxy headers only from configured proxies | Implemented | Immediate peer must match an explicit IP/CIDR in `GROM_TRUSTED_PROXIES` |

No release may be described as security-baseline complete while a `Missing`
entry remains in this table.

## 12. Deployment profiles

### Development

- Docker Compose with Grom and Distribution.
- SQLite and local blob data in named volumes.
- Plain HTTP bound to loopback only.
- API documentation enabled by default.
- No forwarded proxy headers unless an explicit local proxy range is trusted.

### Permissive

- One Grom instance and one private Distribution instance.
- SQLite and local blob storage are the recommended default.
- Access is limited to a trusted private LAN or VPN.
- HTTPS is recommended through Caddy, Traefik, Nginx, or another local reverse
  proxy.
- HTTP on a private address requires an explicit insecure opt-in and produces a
  startup warning; it is never valid for a public address.
- Rate limits, CSRF protection, password/token hashing, request limits, and
  private Distribution remain mandatory.
- Backups include the SQLite database, signing material, Distribution metadata,
  and blob storage.

### Strict (default)

- Same two core application containers and one active Grom instance.
- HTTPS and secure cookies are mandatory.
- Trusted proxy ranges are configured explicitly when TLS terminates upstream.
- Persistent SQLite and local blob storage remain valid for a small
  installation.
- PostgreSQL or S3-compatible storage may be selected only with release
  documentation that states their support level.
- Automated backup and verified restoration cover the selected database,
  signing keys, Distribution metadata, and blob storage.

The shipped Compose files use the explicit `development` profile with SQLite
and an optional local PostgreSQL service. `.env.example` also sets
`development`; an omitted `GROM_DEPLOYMENT_PROFILE` resolves to `strict`.
Private-LAN HTTP requires the explicit permissive opt-in and produces both a
startup warning and a visible management-interface warning.
A production ingress, S3 profile, automated backup, restore verification, and
production secret management remain operator responsibilities until the
corresponding completion work provides supported procedures and evidence.

Multi-instance Grom, Redis, and distributed locking are deferred until real
scale requires them. PostgreSQL support does not imply that multiple active
Grom instances are supported.

## 13. Delivery phases

### Phase 0: executable foundation

**Progress: partially accepted.**

- **Implemented:** Go service, Vue application, embedded frontend build,
  configuration loader, structured logs, and health/readiness endpoints.
- **Implemented:** Compose environment with private Distribution.
- **Implemented:** Bun repositories, portable migrations for SQLite and
  PostgreSQL, and first-admin bootstrap.
- **Implemented:** OpenAPI contract validation, Go/TypeScript generation,
  frontend generated-code freshness, bidirectional route coverage, and
  interactive docs exist. Breaking-change detection remains deferred.
- **Implemented:** CI enforces Go formatting and tests, SQLite integration,
  frontend lint/tests/typechecking/build, golangci-lint, govulncheck, and the
  isolated real-Docker registry, administrative-browser, boot-acceptance, and
  backup/restore journeys. The boot journey passed locally and in CI on August
  3, 2026 in [PR #16](https://github.com/jfxdev/grom-registry/pull/16). Backend
  and frontend unit coverage reports are
  uploaded separately to Codecov, with an explicit 70% patch coverage target.
- **Pending:** production image build and scanning, broader dependency/container
  scanning, and PostgreSQL CI before PostgreSQL is advertised as fully
  supported.

Exit criterion: one command starts Grom with SQLite, applies pending migrations
automatically, and returns an authenticated `/v2/` challenge. The same boot
migration lifecycle must complete against PostgreSQL in CI before PostgreSQL is
included in the supported release matrix.

Remaining default-path evidence is owned by completion steps 4, 5, 7, and 8.
Production image build/scanning, broader dependency scanning,
restart/migration acceptance, and release artifacts are not yet accepted.
PostgreSQL CI remains a capability-specific gate.

### Phase 1: authentication and project authorization

**Progress: Docker acceptance complete.**

- **Implemented:** user sessions.
- **Implemented:** projects and memberships.
- **Implemented:** service accounts.
- **Implemented:** API token create, reveal-once, verify, revoke, expire, and
  rotate.
- **Implemented:** `/auth/token` and asymmetric JWT signing.
- **Implemented:** Reader/Writer/Admin scope mapping.
- **Accepted:** the mandatory registry E2E workflow proves Docker push/pull,
  Reader/Writer authorization, cross-project denial, key revocation, membership
  removal, JWT expiry, first-push provisioning, and policy enforcement through
  the public Grom endpoint.
- **Deferred capability-specific work:** ORAS acceptance remains required only
  before generic OCI support is advertised.

Exit criterion: automated tests prove allowed and denied Docker push/pull operations across at least two projects.

Application and database tests cover the authorization rules, and
`.github/workflows/registry-e2e.yml` proves the exit criterion through the public
registry protocol with Docker.

### Phase 2: registry browsing and core UI

**Progress: partially accepted.**

- **Implemented:** project, repository, tag, manifest-detail, memberships,
  users, service accounts, token management, copyable pull/push guidance, and
  logical-repository archival/removal screens exist.
- **Partial:** the complete security-sensitive event set is produced and has
  SQLite persistence/sanitization coverage; public end-to-end audit acceptance
  and audit presentation remain intentionally outside the default MVP.
- **Partial:** useful empty/error states exist in implemented screens;
  pagination requirements and browser-level acceptance beyond the accepted
  first-push journey remain unresolved.

Exit criterion: a new administrator can create a project, add a service account, push an image, and inspect it without using the database or editing configuration.

The complete first-push browser journey passed locally and in the separate
`Admin Journey E2E (Docker)` GitHub Actions workflow on August 3, 2026 via
`make test-admin-e2e`, exercising the public Grom and Distribution stack. The
CI evidence is [PR #6](https://github.com/jfxdev/grom-registry/pull/6). Broader
browser coverage and the settings decision remain in completion step 6.
Essential audit persistence remains in step 2; an audit browsing page is
post-MVP unless promoted explicitly.

### Phase 3: operational hardening

**Progress: partially accepted.**

- **Partial:** named deployment profiles, request-size/time limits,
  authentication rate limits, trusted-proxy enforcement, production
  HTTPS/cookie validation, and graceful shutdown exist; streaming-specific
  timeout evidence, backup/restore documentation, key rotation, and upgrade
  tests remain.
- **Implemented:** the default SQLite/local-storage recovery matrix is covered
  by the mandatory `Backup Restore E2E` volume-loss journey.
- **Implemented:** mandatory Docker protocol acceptance through the isolated
  registry E2E workflow.
- **Capability-specific:** PostgreSQL, S3, extended ORAS/referrer, and OCI
  conformance matrices.
- **Pending:** release images, checksums, SBOM, and supported deployment
  documentation.
- **New evidence:** the boot-acceptance journey proves empty-volume and
  prior-schema migration before public readiness, public OpenAPI/docs serving,
  and failed-migration non-exposure with unmarked migration history. It passed
  locally and in the mandatory CI check on August 3, 2026 in
  [PR #16](https://github.com/jfxdev/grom-registry/pull/16). The public
  Distribution/blob restart journey also passed locally; its mandatory CI
  evidence remains open.

Exit criterion: a tagged release can be installed and upgraded with preserved metadata and blobs.

Completion step 1 is implemented. Completion steps 4, 5, 7, and 8 own the
remaining default-path recovery, upgrade, Docker, and release work. Expanded
compatibility and conformance gates apply only to advertised capabilities.

### Phase 4: integrations placeholder

**Progress: accepted for the MVP.**

- **Implemented:** backend integration catalog.
- **Implemented:** read-only Integrations page with planned providers and
  capabilities.
- **Deferred guardrail:** architecture decision record before future event
  delivery or scan-result storage is implemented.

Exit criterion: the page is driven by the backend contract and makes no claim that scanning is active.

The exit criterion passes in the current UI and API. The event-delivery and
scan-result-storage ADR is required before active integration work begins, but
does not block the inert MVP placeholder.

## 14. Roadmap after the MVP

Compatibility expansion is driven by actual installation demand:

1. Complete the PostgreSQL gate when small teams require an external database.
2. Complete the S3-compatible storage gate when local storage is insufficient.
3. Complete representative ORAS and referrer coverage when generic OCI content
   becomes a supported product journey.
4. Run broader OCI conformance work before making general conformance claims.

Product expansion then follows user demand:

1. Generic outbound webhook for manifest-pushed events.
2. Trivy scan-on-push and manual scan, with OCI referrers for report artifacts
   where practical.
3. Scan summary and policy-free vulnerability display.
4. Docker-compatible scanner provider.
5. Optional scheduling for the implemented retention dry-runs and audited
   manual lifecycle execution.
6. OIDC login.
7. Multiple Grom replicas, including the required coordination and locking
   design.
8. Registry mirroring or pull-through cache.

Signature and vulnerability admission policies, Kubernetes operators, Helm
charts, teams, organizations, and replication should each require a separate
design decision rather than entering the product implicitly. Repository-local
behavior policies and safe manual deletion are part of the implemented control
plane. Inventory-backed dry-runs and audited manual execution are implemented;
periodic scheduling remains an explicit follow-up.

## 15. MVP acceptance scenarios

Status vocabulary:

- **Passing:** automated evidence exercises the acceptance boundary.
- **Partial:** the implementation or lower-level test exists, but the required
  end-to-end boundary is not proven.
- **Unverified:** the code path appears to exist, but dedicated acceptance
  evidence is absent.
- **Missing:** the required implementation or delivery mechanism does not exist.

| # | Scenario | Gate | Status on August 3, 2026 | Evidence needed to close |
|---:|---|---|---|---|
| 1 | An administrator creates projects `alpha` and `beta` | Default MVP | Passing | Registry E2E creates both projects through the public management API |
| 2 | A service account is Writer in `alpha` and has no membership in `beta` | Default MVP | Passing | Registry E2E persists real projects, principals, and memberships |
| 3 | Its access key can push and pull `alpha/api:v1` | Default MVP | Passing | Registry E2E performs Docker push and pull through Grom |
| 4 | The same key cannot pull or push `beta/api:v1` | Default MVP | Passing | Registry E2E proves equivalent denial for existing and missing private repositories |
| 5 | A Reader key can pull but cannot push | Default MVP | Passing | Registry E2E proves Reader pull success and push denial |
| 6 | Revoking a key prevents the next registry-token exchange immediately | Default MVP | Passing | Registry E2E repeats the exchange after key revocation |
| 7 | Removing a membership prevents access while the key remains valid | Default MVP | Passing | Registry E2E removes membership and verifies the next scoped token loses access |
| 8 | A short-lived registry JWT expires without invalidating the long-lived key | Default MVP | Passing | Registry E2E bounds expiry rejection and performs a successful fresh exchange |
| 9 | Docker Engine can push and pull supported image content | Default MVP | Passing | Mandatory registry E2E job exercises Docker through the public Grom endpoint |
| 10 | The UI lists the pushed repository and tag from live Distribution metadata | Default MVP | Passing | `make test-admin-e2e` passed locally and the mandatory `Admin Journey E2E (Docker)` workflow passed in [PR #6](https://github.com/jfxdev/grom-registry/pull/6) |
| 11 | Integrations shows backend-driven planned providers with no active configuration | Default MVP | Passing | Existing API/UI tests cover the read-only flow |
| 12 | Restarting both services preserves users, projects, memberships, and blobs | Default MVP | Partial | The public process-level restart test passed locally; record its `Registry E2E (Docker)` CI evidence before marking passing |
| 13 | The applicable backend suite passes against PostgreSQL | PostgreSQL support | Partial | Mandatory PostgreSQL CI job with no skip path |
| 14 | Empty or supported older databases migrate before readiness | Default MVP | Passing | `make test-boot-acceptance` and the mandatory `Boot Acceptance E2E (Docker)` workflow passed in [PR #16](https://github.com/jfxdev/grom-registry/pull/16) |
| 15 | A failed migration prevents startup and HTTP/registry exposure | Default MVP | Passing | The real failing-migration fixture passed through the mandatory `Boot Acceptance E2E (Docker)` workflow in [PR #16](https://github.com/jfxdev/grom-registry/pull/16) |
| 16 | Every management/auth endpoint is versioned in OpenAPI and rendered at `/api/docs` | Default MVP | Passing | The public OpenAPI and documentation checks passed through the mandatory `Boot Acceptance E2E (Docker)` workflow in [PR #16](https://github.com/jfxdev/grom-registry/pull/16) |
| 17 | CI rejects invalid OpenAPI, stale generated code, and undocumented routes | Default MVP | Passing | Backend contract tests and frontend generated-code freshness check run in CI |
| 18 | Development, permissive, and strict profiles enforce their documented network rules, with strict as the default | Default MVP | Passing | Configuration tests cover the implemented profile rules |
| 19 | A default SQLite/local-storage backup restores an installation that can authenticate, browse, push, and pull | Default MVP | Passing | Backup Restore E2E creates and downloads through the UI-facing API, deletes the local snapshot while preserving the downloaded bundle, destroys the original volumes, restores through the same image's recovery service, invalidates the old browser session, signs in again, browses the recovered project, pulls preserved content, pushes a new tag, and rejects corruption |
| 20 | Every essential authentication, credential, membership, project, policy, and destructive action creates a sanitized audit event | Default MVP | Partial | Durable SQLite persistence and sanitization tests exist; complete destructive failure-path coverage |
| 21 | An installation administrator disables a user and the user's active sessions stop working | Default MVP | Partial | Add the test-only public HTTP/session acceptance scenario in [`mvp-acceptance-implementation-plan.md`](mvp-acceptance-implementation-plan.md) and record its `Registry E2E (Docker)` CI evidence |
| 22 | ORAS can push and pull representative generic OCI content | Generic OCI support | Missing | ORAS smoke job before advertising generic OCI support |
| 23 | An S3-backed installation passes push, pull, restart, and restore checks | S3 support | Missing | Documented S3 compatibility job before advertising S3 support |

The default self-hosted MVP is not accepted until every `Default MVP` row is
`Passing`. Capability-specific rows must pass before that capability is
advertised as supported, but they do not delay an explicitly
SQLite/local-storage/Docker-focused release. Lower-level unit or application
tests remain valuable, but they do not replace a scenario whose stated boundary
is Docker, browser UI, process restart, backup restoration, startup failure, or
CI.

## 16. Decisions to preserve

- Distribution is a separate, unmodified process.
- Grom is the only public entry point.
- SQLite is the default supported database. A PostgreSQL implementation exists
  through Bun; a release advertises PostgreSQL support only after its
  capability-specific gate passes.
- The first supported release is optimized for one active instance with
  SQLite, local blob storage, and Docker clients.
- Development, permissive, and strict profiles make network-security tradeoffs
  explicit; `strict` is the default, and public addresses always require
  HTTPS.
- PostgreSQL, S3, ORAS, and broader OCI claims require their capability-specific
  acceptance gates before being advertised as supported.
- Database-specific behavior stays inside the store and migration packages.
- Reviewed, versioned migrations run automatically before the application becomes ready.
- ORM model changes never alter the schema implicitly.
- The first repository path segment is the immutable project boundary.
- Registry clients use API tokens, never web passwords.
- Authorization comes from current project membership.
- Push is granted only for repositories registered inside the matching project.
- Repository behavior policies never cross their owning project boundary.
- Global policy presets are form recommendations, not inherited runtime policy.
- Manifest deletion is performed by digest through Grom; garbage collection remains a separate operator action.
- Integrations are visible but inactive until the roadmap phase implements a real provider.

## 17. Primary references

- [CNCF Distribution](https://distribution.github.io/distribution/about/)
- [Distribution token authentication specification](https://distribution.github.io/distribution/spec/auth/token/)
- [Distribution registry JWT format](https://distribution.github.io/distribution/spec/auth/jwt/)
- [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md)
- [OpenAPI Specification](https://spec.openapis.org/oas/)
- [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)
