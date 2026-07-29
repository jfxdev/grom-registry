# Registry end-to-end implementation plan

## Status and purpose

**Status: implemented on July 29, 2026. The deterministic Make target and
dedicated GitHub Actions job exist.**

This page defines the implementation plan for the real Docker journey required
by completion step 5 of [`architecture-and-mvp.md`](architecture-and-mvp.md).
The goal is to prove Grom's registry authorization, policy enforcement, and
inventory behavior through the public Grom endpoint with real Docker clients.

The harness must not:

- connect directly to the Grom database;
- expose or call the private Distribution port from the host;
- seed state through application services;
- place access-key secrets in command arguments, logs, or test failures;
- modify the operator's normal Docker credential configuration;
- depend on a remote base image or another network download during the test.

ORAS remains outside this first implementation slice. It becomes part of the
matrix before generic OCI artifact support is advertised as fully supported.

## Delivery outcome

The completed slice provides one automated command that:

1. starts Grom and Distribution in an isolated Compose project;
2. prepares all identities, projects, memberships, repositories, and policies
   through the public management API;
3. builds a deterministic local image from `scratch`;
4. exercises push and pull through the public Grom endpoint;
5. proves authorization changes on the next registry token exchange;
6. verifies repository, tag, and manifest-inventory observation;
7. removes all temporary containers, volumes, credentials, and image tags.

The intended command is:

```text
make test-registry-e2e
```

The regular `make test` command remains suitable for fast backend and frontend
checks. The registry journey compiles with the backend test suite but skips
unless explicitly enabled, so its code cannot silently drift while Docker is
unavailable.

## Implemented repository changes

### Test harness

Create `backend/tests/registrye2e/` with the following responsibilities:

| Path | Responsibility |
|---|---|
| `registry_e2e_test.go` | Ordered journey and behavior-focused subtests |
| `stack_test.go` | Compose project lifecycle, readiness, diagnostics, and cleanup |
| `api_test.go` | Cookie-authenticated management API client with Origin handling |
| `docker_test.go` | Isolated Docker credentials, image build, login, push, and pull |
| `fixtures/` | Network-independent `scratch` image inputs with two content variants |

Prefer the Go standard library and the existing backend module. Do not add a
testcontainers framework or another orchestration dependency unless the
command-based harness proves inadequate.

### Compose and commands

Update:

- `deploy/compose/docker-compose.yml` to support a configurable loopback host
  port and a configurable short registry-token TTL;
- `Makefile` with `test-registry-e2e`;
- `.env.example` only when a new local-development variable must be discoverable.

The shipped development binding should default to `127.0.0.1`. A permissive LAN
operator may explicitly choose a broader bind address, but the E2E harness must
always use loopback.

### Documentation maintained with implementation

Update:

- `docs/architecture-and-mvp.md` with acceptance evidence;
- `docs/code-map.md` with the implemented test package and command;
- `AGENTS.md` with the real command, prerequisites, isolation rule, and common
  failure modes;
- `README.md` with a concise operator-facing invocation.

`docs/domain-model.md` does not require a change unless implementation work
introduces an architectural type or ownership relationship. The planned
harness itself does neither.

## Harness architecture

### Opt-in execution

The package should compile during `go test ./...` and skip execution unless
`GROM_RUN_REGISTRY_E2E=1` is present. The Make target sets the variable and
runs the package with a bounded timeout and test-cache bypass:

```text
GROM_RUN_REGISTRY_E2E=1 go test -count=1 -timeout=10m ./tests/registrye2e
```

### Isolated stack

For each run, the harness:

1. verifies that Docker CLI, Docker daemon, and Docker Compose are available;
2. selects an unused loopback port;
3. creates a unique Compose project name;
4. supplies explicit development-profile configuration and a short registry
   JWT lifetime;
5. starts the stack detached and waits for public readiness;
6. registers cleanup immediately after startup begins;
7. executes `docker compose down --volumes --remove-orphans` during cleanup.

Named Compose volumes remain isolated by the unique project name. The harness
must never run a broad Docker prune or delete an image, container, network, or
volume it did not create.

### API client

Use `net/http` with a cookie jar. The client:

- creates the administrator session through `POST /api/v1/session`;
- sends the configured public URL as `Origin` for every authenticated mutation;
- decodes responses into generated OpenAPI transport types where they fit;
- limits response bodies included in failures;
- never logs cookies, Authorization headers, passwords, or reveal-once secrets.

All application state must be created through `/api/v1/*`.

### Docker client isolation

Create one temporary `DOCKER_CONFIG` directory per service account. Use
`docker login --password-stdin`; never pass an access key as a command argument.

Build two small image variants from `scratch`, each containing only a different
marker file. This produces distinct manifest digests without pulling a base
image. Track every local tag and remove only those exact tags during cleanup.

## Test data

Create the following state through the management API:

| Resource | Assignment | Purpose |
|---|---|---|
| Project `alpha` | Primary test boundary | Allowed push/pull and policy cases |
| Project `beta` | Secondary boundary | Cross-project denial |
| `alpha-writer` | Writer in `alpha` | First push, policies, JWT, and key revocation |
| `alpha-reader` | Reader in `alpha` | Pull success, push denial, membership removal |
| `beta-writer` | Writer in `beta` only | Existing and missing `alpha` resource denial |

Each service account receives one reveal-once access key. Keep secrets only in
memory and in the isolated Docker credential directory for that principal.

## Scenario sequence

The journey is intentionally stateful. Run the phases in order and use subtests
for diagnostics without enabling parallel execution.

### Phase 1: first push and normal access

1. Build image variant A.
2. Log in as `alpha-writer`.
3. Push `alpha/app:v1` without creating the logical repository first.
4. Confirm that first-push provisioning succeeds.
5. Pull the image as `alpha-writer`.
6. Pull the image as `alpha-reader`.

Expected results:

- the Writer receives pull and push authorization;
- the first token exchange creates an empty logical repository idempotently;
- the completed manifest push changes the repository to active;
- Reader can pull the existing content.

### Phase 2: role and project denial

1. Attempt to push another tag as `alpha-reader`.
2. Attempt to pull `alpha/app:v1` as `beta-writer`.
3. Attempt to push an `alpha` repository as `beta-writer`.
4. Compare denial behavior for an existing private repository and a missing
   repository in the same unauthorized project.

Expected results:

- Reader never receives push;
- membership in `beta` grants no `alpha` action;
- pull denial never creates a logical repository;
- public status and registry error class do not reveal whether the denied
  repository exists.

### Phase 3: JWT lifetime and mutable authorization

Configure a short registry-token TTL for this isolated stack.

1. Exchange the Writer access key for a scoped bearer token.
2. Read the pushed manifest through the public `/v2` gateway with that token.
3. Wait until its signed expiry and confirm that reusing it is rejected.
4. Exchange the still-valid access key for a fresh JWT and confirm access.
5. Revoke the Writer access key through the management API.
6. Confirm that the next token exchange fails.
7. Remove the Reader membership while leaving its access key valid.
8. Confirm that its next exchange no longer carries project access.

Do not rely on restarting Grom or Distribution between these checks.

### Phase 4: policy enforcement through Docker

Create these logical repositories and policies through the API:

| Repository | Policy | Allowed case | Denied case |
|---|---|---|---|
| `alpha/protected` | Protect `prod` from overwrite | First `prod` push | Variant B overwrites `prod` |
| `alpha/immutable` | Make `release-*` immutable | First `release-1` push | Variant B overwrites `release-1` |
| `alpha/named` | Allow only `v*` tag names | Push `v1` | Push `latest` |

The assertion is a failed real Docker push caused by the public gateway's
manifest `PUT`. Do not replace these checks with direct application-service
tests; those already exist at lower layers.

### Phase 5: repository observation

After the successful `alpha/app:v1` push, query:

- `GET /api/v1/projects/alpha/repositories`;
- `GET /api/v1/projects/alpha/repository-tags?repository=app`;
- `GET /api/v1/projects/alpha/repository-inventory?repository=app`.

Assert:

- repository name `app`;
- `creationSource=push`;
- active repository status;
- tag `v1`;
- an inventoried digest associated with `v1`;
- primary container-image classification and inferred repository profile.

Use a bounded poll only if observation is not immediately visible. Every poll
must have a concrete condition and deadline.

## Failure handling and diagnostics

Before execution, fail or skip with a specific reason when:

- Docker CLI is missing;
- Docker Compose is missing;
- the Docker daemon is inaccessible;
- the selected host port cannot be used.

On a scenario failure:

1. report the behavior step and sanitized command;
2. include the public HTTP status and stable error code where available;
3. capture bounded Grom and Distribution logs;
4. redact secrets and authorization material;
5. continue guaranteed cleanup.

Do not include a reveal-once access key in `t.Log`, command arguments, Compose
configuration, environment dumps, or failure output.

## CI integration boundary

The dedicated `.github/workflows/registry-e2e.yml` workflow invokes
`make test-registry-e2e` on an Ubuntu runner with Docker Engine and Compose.
It runs for pull requests, pushes to `main`, merge queues, and manual dispatch,
with the stable status-check name `Registry E2E (Docker)`. The Make target sets
`GROM_RUN_REGISTRY_E2E=1`; missing Docker prerequisites or a failed journey
therefore fail the job instead of producing a successful skipped test.

Repository administrators must require `Registry E2E (Docker)` in the `main`
branch ruleset. Workflow YAML alone cannot mutate repository branch protection.

## Acceptance checklist

- [x] One command starts, tests, diagnoses, and removes the isolated stack.
- [x] Docker pushes and pulls supported image content through Grom.
- [x] Reader pull succeeds and Reader push fails.
- [x] Cross-project push and pull fail.
- [x] Existing and missing private repositories have equivalent denial posture.
- [x] Revoked access keys fail on the next exchange.
- [x] Removed memberships lose access on the next exchange.
- [x] An expired JWT fails and the still-valid access key obtains a fresh JWT.
- [x] Protected, immutable, and invalid tag pushes are rejected.
- [x] Successful pushes appear in repository, tag, and inventory reads.
- [x] No test reaches the private Distribution port or database.
- [x] No operator Docker credentials or unrelated Docker resources are changed.
- [x] `make test` and `make build` remain green after implementation.

Acceptance evidence was recorded on July 29, 2026 with
`make test-registry-e2e`, which passed in 92 seconds against Docker Engine
29.6.1 and Distribution 3.1.1. Distribution's clock-skew tolerance kept the
eight-second JWT usable briefly after its signed expiry; the harness therefore
polls only the public manifest endpoint until rejection under a fixed
75-second post-expiry deadline. The still-valid access key then obtained a
fresh JWT without restarting either service.

## Known implementation prerequisites

At implementation time:

- Docker Compose is installed;
- the Docker daemon may require an environment permission grant in a sandboxed
  workspace;
- ORAS is not installed and is intentionally deferred;
- the registry E2E harness and dedicated GitHub Actions job exist.

These facts are environment observations, not product requirements. The harness
must still provide portable prerequisite checks and clear diagnostics.
