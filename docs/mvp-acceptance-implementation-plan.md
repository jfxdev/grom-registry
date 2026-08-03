# MVP acceptance implementation plan

## Purpose and scope

**Status: Work package A implemented and passed locally on August 3, 2026; CI
acceptance evidence remains to be recorded. Work package B remains planned.**

This plan closes the next two default-MVP acceptance gaps from
[`architecture-and-mvp.md`](architecture-and-mvp.md):

1. a real-browser first-push journey that proves the UI refreshes from live
   Distribution metadata; and
2. public boot, readiness, and API-documentation smoke checks for the default
   SQLite/local-storage installation.

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
  the MVP acceptance matrix identify the command and workflow. Its first CI
  run exposed a test race and must pass after the correction before acceptance
  evidence is recorded.

### Acceptance and evidence

The harness is implemented as `make test-admin-e2e` and its CI workflow exposes
the stable check `Admin Journey E2E (Docker)`. The command passed locally on
August 3, 2026. Record the first successful CI run before changing MVP scenario
10 from **Partial** to **Passing**.

## Work package B: default-installation boot and contract smoke checks

### Current baseline

Database-level tests prove that a failed SQLite migration is not marked as
applied, and an integration test proves metadata survives a close/reopen. The
registry E2E waits on public `/readyz`. These tests do not yet prove the full
startup boundary: migration states, process readiness exposure, and `/api/docs`
on an installed stack.

### Scenarios

The acceptance suite must run against the public Grom port and test:

| Scenario | Required assertion |
|---|---|
| Empty SQLite volume | Migrations complete, `/readyz` becomes successful, and a bootstrap administrator can sign in. |
| Supported previous SQLite state | Migrations complete before readiness and preserved users, projects, and memberships remain readable after boot. |
| Failed migration fixture | The process exits or remains unready; `/readyz`, management API, and `/v2/` must not become available. Migration history must not record the failed version. |
| Documentation surface | `/api/openapi.yaml` returns the versioned contract, the existing bidirectional registered-route/OpenAPI validation proves every management and authentication operation is included, and `/api/docs` returns the interactive documentation page. |

### Implementation design

- Add an opt-in boot-acceptance package or harness next to the existing
  public-stack E2E suites. It must use a unique Compose project and temporary
  volumes, with bounded waits and diagnostics on failure. The existing Go
  `registrye2e` stack helper is the preferred starting point because it already
  implements these Compose safety boundaries.
- Seed a supported previous SQLite fixture only through a reviewed migration
  fixture or an archived, versioned database artifact. Do not mutate live
  schemas ad hoc in the test.
- Introduce a narrowly scoped test-only failing-migration fixture through the
  database bootstrap seam. It must be impossible to activate from a production
  deployment configuration.
- Probe public HTTP only after process launch. Do not replace this suite with
  direct calls to the migration runner or an in-process HTTP handler.
- Add a `make test-boot-acceptance` target and a separate CI workflow/job once
  stable. Keep the tests targeted enough to finish within the existing Docker
  acceptance budget.

### Acceptance and evidence

When these scenarios pass in the dedicated command and CI, mark MVP scenarios
14, 15, and 16 **Passing**. Scenario 12 remains separate: it requires a
deliberate full Compose restart proving Distribution/blob preservation.

## Execution order

1. Completed: implement the public-stack Playwright configuration and isolated
   harness.
2. Completed: add and stabilize the browser administration/first-push scenario.
3. Next: add the empty-volume and `/api/docs` boot smoke checks.
4. Add previous-version and failing-migration fixtures, then their public
   readiness assertions.
5. Wire the boot-acceptance command into CI, then refresh the architecture
   acceptance table with dated evidence.
