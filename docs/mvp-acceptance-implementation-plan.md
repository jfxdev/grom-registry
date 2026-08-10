# MVP acceptance implementation plan

## Purpose and scope

**Status: Work packages A and B are implemented and accepted locally and in CI
on August 3, 2026. Their recorded CI evidence is
[PR #6](https://github.com/jfxdev/grom-registry/pull/6) for A and
[PR #16](https://github.com/jfxdev/grom-registry/pull/16) for B. Work packages
C and D are also accepted locally and in CI on August 3, 2026; their recorded
evidence is [PR #17](https://github.com/jfxdev/grom-registry/pull/17). The
expanded destructive administrative journey is implemented and passed locally
on August 9, 2026; its mandatory CI evidence is pending.**

This plan closes the next two default-MVP acceptance gaps from
[`architecture-and-mvp.md`](architecture-and-mvp.md):

1. a real-browser first-push journey that proves the UI refreshes from live
   Distribution metadata; and
2. public boot, readiness, and API-documentation smoke checks for the default
   SQLite/local-storage installation.

It also plans two remaining public acceptance checks: immediate user-session
revocation after administrative disablement, and full Grom plus Distribution
restart preservation. Both work packages are test-only: existing product
implementation and lower-level tests already cover their behavior, and
production code changes are out of scope unless public acceptance reveals a
defect.

It does not expand the product scope. PostgreSQL, S3, ORAS, audit presentation,
and release publication remain separate capability or release-engineering work.

## Work package A: real-browser administration and first push

### Current baseline

`frontend/e2e/sign-in.spec.ts` tests a mocked invalid-sign-in response through
Vite. It is useful as a UI smoke test but cannot establish the required
acceptance boundary: it does not start Grom or Distribution, create data in the
browser, or exercise a Docker push.

The existing `backend/tests/registrye2e` harness already provides the intended
isolation model: a unique Compose project, loopback-only public endpoint,
temporary Docker credential directories, exact-resource cleanup, and no direct
database access. Reuse its safety rules; do not add a broad Docker cleanup.

### Target journey

Run an isolated installed stack and drive the public UI with Chromium:

1. Sign in with the configured bootstrap installation administrator.
2. Create project `alpha` from the UI.
3. Create service account `alpha-writer` and a reveal-once access key from the
   UI; retain the secret only in test memory or its temporary Docker credential
   directory.
4. Assign `alpha-writer` the Writer role in `alpha` from the UI.
5. Log in with Docker and push a deterministic local `scratch` image to
   `alpha/app:v1`, without first creating a logical repository.
6. Return to the project UI and assert that the repository is active, `v1` is
   visible, and the observed manifest inventory is shown. Open the manifest
   detail dialog and assert its digest and tag.

This proves the administrator's primary workflow without database access,
configuration edits, a private Distribution port, or mocked API responses.

### Implementation design

- Implemented: a Playwright global setup/teardown pair under `frontend/e2e/` for the
  public-stack mode. It will reserve a loopback port, start a uniquely named
  Compose project from `deploy/compose/docker-compose.yml`, wait for public
  readiness, write the public URL to a per-run runtime file read by the tests,
  and tear down that exact project. This keeps browser actions in Playwright
  while avoiding a second Go test harness that would have to launch Node.
- Implemented: Playwright configuration recognizes `GROM_RUN_ADMIN_E2E=1`. That mode
  disables Vite's `webServer`, installs the global setup/teardown, and requires
  the public URL supplied by setup. The ordinary mocked suite continues to
  start Vite and keeps its current base URL.
- Implemented: the dedicated opt-in command `make test-admin-e2e` invokes the
  public-stack mode. Compose builds the root `Dockerfile`, whose build embeds
  the frontend distribution in Grom; the browser therefore validates the same
  frontend that the container serves.
- Keep ordinary `npm run test:e2e` fast and mocked. The new journey is a
  Docker-required release gate, not a replacement for component tests.
- Use `docker login --password-stdin`; never include the reveal-once key in an
  argument, URL, log, screenshot, or failure message.
- Run Docker build/login/push from a narrowly scoped Node helper in the same
  test support folder. It must build only the existing deterministic `scratch`
  fixture, use a per-test temporary `DOCKER_CONFIG`, redact the key from
  diagnostics, and remove only the exact image tags it created.
- Make selectors accessible and stable by using roles, labels, and visible
  names. Add `data-testid` only where semantics cannot provide a stable
  selector.
- Implemented: the `Admin Journey E2E (Docker)` workflow runs the command as a
  separately named mandatory check, and `AGENTS.md`, `docs/code-map.md`, and
  the MVP acceptance matrix identify the command and workflow. After the test
  race was corrected, the workflow passed in
  [PR #6](https://github.com/jfxdev/grom-registry/pull/6).

### Acceptance and evidence

The harness is implemented as `make test-admin-e2e` and its CI workflow exposes
the stable check `Admin Journey E2E (Docker)`. The command passed locally and
the mandatory CI check passed on August 3, 2026 in
[PR #6](https://github.com/jfxdev/grom-registry/pull/6). MVP scenario 10 is
therefore **Passing**.

## Work package B: default-installation boot and contract smoke checks

### Current baseline

Before this work package, database-level tests proved that a failed SQLite
migration was not marked as applied, an integration test proved metadata
survived a close/reopen, and registry E2E waited on public `/readyz`. The boot
acceptance suite now proves the full startup boundary locally; CI evidence is
the remaining acceptance gap.

### Scenarios

The acceptance suite must run against the public Grom port and test:

| Scenario | Required assertion |
|---|---|
| Empty SQLite volume | Migrations complete, `/readyz` becomes successful, and a bootstrap administrator can sign in. |
| Supported previous SQLite state | Migrations complete before readiness and preserved users, projects, and memberships remain readable after boot. |
| Failed migration fixture | The process exits or remains unready; `/readyz`, management API, and `/v2/` must not become available. Migration history must not record the failed version. |
| Documentation surface | `/api/openapi.yaml` returns the versioned contract, the existing bidirectional registered-route/OpenAPI validation proves every management and authentication operation is included, and `/api/docs` returns the interactive documentation page. |

### Implementation design

- Implemented: `backend/tests/bootacceptance` starts isolated Compose projects
  with bounded waits, diagnostics, and exact-project cleanup. It verifies empty
  SQLite boot, a supported prior schema, a failed real migration that never
  exposes the public surface, and the installed documentation endpoints.
- The supported-upgrade fixture is the reviewed, versioned
  `fixtures/sqlite-supported-baseline-202607260001.sql` schema. The test
  creates it only in a temporary directory, seeds the historical
  user/project/membership state, and copies it into the isolated named volume
  before normal Grom startup.
- The separate failing-migration fixture starts from the current schema, marks
  migration `202607270006` pending while retaining its added column, and
  therefore makes the real production migration fail deterministically. No
  production configuration activates this fixture or changes migration logic.
- Probe public HTTP only after process launch. Do not replace this suite with
  direct calls to the migration runner or an in-process HTTP handler.
- Implemented: `make test-boot-acceptance` and the separate `Boot Acceptance
  E2E (Docker)` workflow run the suite. Keep the tests targeted enough to finish
  within the existing Docker acceptance budget.

### Acceptance and evidence

`make test-boot-acceptance` passed locally and the mandatory `Boot Acceptance
E2E (Docker)` CI check passed on August 3, 2026 in
[PR #16](https://github.com/jfxdev/grom-registry/pull/16). MVP scenarios 14,
15, and 16 are therefore **Passing**. Scenario 12 remains separate: it
requires a deliberate full Compose restart proving Distribution/blob
preservation.

## Work package C: administrative user disablement revokes a live session

**Status: implemented and accepted locally and in CI on August 3, 2026; the
mandatory `Registry E2E (Docker)` check passed in
[PR #17](https://github.com/jfxdev/grom-registry/pull/17).**

### Current baseline

`backend/internal/httpapi/server_test.go` proves that `DELETE /api/v1/users/{id}`
invalidates the target user's active session. The missing MVP evidence is the
same behavior through an isolated installed stack and a real public session
cookie; it is not a missing product capability.

### Target journey

Run the following test-only scenario through the public Grom endpoint:

1. Start the existing isolated registry E2E Compose stack.
2. Sign in as the bootstrap installation administrator and create a non-admin
   human user.
3. Use an independent HTTP cookie jar to sign in as that user and confirm
   `GET /api/v1/me` returns `200 OK`.
4. Use the administrator session to call `DELETE /api/v1/users/{id}` and
   require `204 No Content`.
5. Reuse the target user's original cookie against `GET /api/v1/me` and require
   `401 Unauthorized`.
6. Attempt a fresh sign-in with the disabled user's credentials and require
   `401 Unauthorized`.

### Implementation design

- Add `TestUserDisableRevokesActiveSession` to `backend/tests/registrye2e` so
  it runs under the existing mandatory `Registry E2E (Docker)` check; do not
  add a new workflow or required status check for this short public HTTP case.
- Extend the existing management-test client only with helpers to create a
  human user, create an independent authenticated client, and disable a user.
- Keep the target non-administrative so the test does not depend on the
  self-disable and last-installation-administrator protections.
- Do not use Playwright: independent HTTP cookie jars directly prove the
  session-revocation security boundary and avoid duplicating browser coverage.
- Do not change OpenAPI or production code unless the test exposes a defect;
  any such correction becomes a separate implementation response with the
  normal contract-first review.

### Acceptance and evidence

The mandatory `Registry E2E (Docker)` check passed in
[PR #17](https://github.com/jfxdev/grom-registry/pull/17) with both the
stale-cookie rejection and the disabled-user sign-in rejection. MVP scenario 21
is therefore **Passing**. The test does not log the user's password or session
cookie in diagnostics.

## Work package D: full-stack restart preserves registry state and blobs

**Status: implemented and accepted locally and in CI on August 3, 2026; the
mandatory `Registry E2E (Docker)` check passed in
[PR #17](https://github.com/jfxdev/grom-registry/pull/17).**

### Current baseline

The SQLite integration test verifies a close/reopen of metadata state, but it
cannot prove persistence through the installed Grom and Distribution processes
or preservation of Distribution blob storage. This work package adds that
public, Docker-backed boundary without changing product behavior.

### Target journey

1. Start the existing isolated registry E2E Compose stack.
2. Create project `restart-alpha`, a Writer service account, and its access
   key through the public management API.
3. Push the deterministic local `scratch` fixture as `restart-alpha/app:v1`,
   then wait until repository, tag, and manifest inventory reads observe it.
4. Run `docker compose restart grom distribution` for the exact isolated
   project; do not recreate containers or volumes.
5. Wait for `/readyz` and `/v2/` through the public Grom endpoint.
6. Use fresh administrative and Docker clients to prove the administrator can
   sign in, the Writer still has push authority, and repository/tag/inventory
   metadata still includes `v1`.
7. Remove the local `v1` tag, pull it from the registry to prove preserved
   manifests and blobs, then push `restart-alpha/app:v2` and observe both tags
   in the public inventory.

### Implementation design

- `TestRestartPreservesPublicState` lives in `backend/tests/registrye2e` and
  runs in the existing mandatory `Registry E2E (Docker)` check. A separate
  stack per test retains isolated volumes and bounded failure diagnostics while
  avoiding a duplicate Compose harness and an additional required workflow.
- The test performs an explicit Compose `restart` only after the initial push;
  the test cleanup remains the sole owner of `down --volumes` for that exact
  project.
- A fresh Docker credential directory is used after restart. This avoids
  treating a cached bearer credential as evidence that the persisted Writer
  membership still authorizes a new registry exchange.
- No database, named volume, or private Distribution endpoint is read directly.

### Acceptance and evidence

The mandatory `Registry E2E (Docker)` check passed in
[PR #17](https://github.com/jfxdev/grom-registry/pull/17) with the
restarted-stack pull of `v1` and push of `v2`. MVP scenario 12 is therefore
**Passing**.

## Execution order

1. Completed: implement the public-stack Playwright configuration and isolated
   harness.
2. Completed: add and stabilize the browser administration/first-push scenario.
3. Completed: add the empty-volume and `/api/docs` boot smoke checks.
4. Completed: add previous-version and failing-migration fixtures with public
   readiness assertions.
5. Completed: wire the boot-acceptance command into CI, accept its successful
   run, and refresh the architecture acceptance table with dated evidence.
6. Completed: add and accept the public, test-only user-disable/session-
   revocation scenario in the mandatory CI run recorded by
   [PR #17](https://github.com/jfxdev/grom-registry/pull/17).
7. Completed: add and accept the full-stack restart-preservation scenario in
   the mandatory CI run recorded by
   [PR #17](https://github.com/jfxdev/grom-registry/pull/17).
