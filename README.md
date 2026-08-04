# Grom

[![codecov](https://codecov.io/gh/jfxdev/grom-registry/graph/badge.svg?token=5NOmSFnvkT)](https://codecov.io/gh/jfxdev/grom-registry)
[![CI](https://github.com/jfxdev/grom-registry/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/ci.yml)
[![Registry E2E](https://github.com/jfxdev/grom-registry/actions/workflows/registry-e2e.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/registry-e2e.yml)
[![Admin Journey E2E](https://github.com/jfxdev/grom-registry/actions/workflows/admin-journey-e2e.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/admin-journey-e2e.yml)
[![Boot Acceptance E2E](https://github.com/jfxdev/grom-registry/actions/workflows/boot-acceptance-e2e.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/boot-acceptance-e2e.yml)
[![Backup Restore E2E](https://github.com/jfxdev/grom-registry/actions/workflows/backup-restore-e2e.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/backup-restore-e2e.yml)

Grom is a lightweight, self-hosted OCI registry for individuals and small
teams. It pairs CNCF Distribution with a secure control plane and a simple web
interface for managing projects, access, images, and recovery.

## Features

- Project-scoped Docker and OCI registry access with Reader, Writer, and Admin roles.
- Human users, service accounts, reveal-once access keys, and short-lived JWTs.
- Streaming gateway in front of unmodified CNCF Distribution v3.
- Web management interface for projects, memberships, repositories, manifests, and policies.
- Automatic versioned database migrations with SQLite as the default storage.
- OpenAPI contract and interactive API documentation at `/api/docs`.
- OCI manifest inventory, passive artifact classification, retention previews, and audited manual lifecycle actions.
- Built-in backup creation, download, and loopback-only disaster recovery for the default installation.
- Explicit development, permissive, and strict deployment profiles.
- Read-only integrations catalog for planned future capabilities.

## Quick start

```bash
cp .env.example .env
docker compose --env-file .env -f deploy/compose/docker-compose.yml up --build
```

Open `http://localhost:8080` and sign in with the bootstrap credentials from `.env`.
Change the example password before exposing Grom outside localhost.

The Compose bundle builds and tags one self-contained image by default. A
release deployment can set `GROM_IMAGE` to the desired published Grom image;
the normal server, isolated backup agent, and disaster-recovery UI all use that
same image.

## Releases

Pushing a semantic-version Git tag such as `v0.1.0` publishes the image to
`ghcr.io/jfxdev/grom-registry` under both `v0.1.0` and `0.1.0`. Use the image
digest recorded in the GitHub Release asset for an immutable deployment
reference. Each release also contains an SPDX SBOM, a Trivy vulnerability
report, and `checksums.sha256` for its assets. Tags with a prerelease suffix,
such as `v0.1.0-rc.1`, are published as GitHub prereleases.

To use PostgreSQL instead of SQLite:

```bash
make compose-up-postgres
```

After creating a project, service account, membership, and access key:

```bash
docker login localhost:8080
docker tag my-image:latest localhost:8080/my-project/my-image:latest
docker push localhost:8080/my-project/my-image:latest
```

Grom has three explicit deployment profiles:

- `development` allows HTTP only on loopback addresses and is set by the local
  `.env.example` and Compose configuration.
- `permissive` recommends HTTPS but can allow private-LAN HTTP only when
  `GROM_ALLOW_INSECURE_PRIVATE_HTTP=true`. Startup logs and the web interface
  display a persistent warning in that mode.
- `strict` requires HTTPS and `GROM_SECURE_COOKIES=true`. It is the default when
  `GROM_DEPLOYMENT_PROFILE` is absent.

The permissive HTTP exception accepts only loopback, RFC 1918, link-local, IPv6
private/link-local, `.home.arpa`, and `.local` addresses. It never permits an
arbitrary public address.

When Grom runs behind a reverse proxy, set `GROM_TRUSTED_PROXIES` to the
comma-separated IP addresses or CIDR ranges of the immediate trusted proxy
network. Forwarded client-IP headers are ignored for every other peer. Do not
use a broad range that also contains untrusted application clients.
Set `GROM_PUBLIC_URL` to the external HTTPS URL and keep
`GROM_SECURE_COOKIES=true` when TLS terminates at that trusted proxy.

Failed web sign-in and registry-token authentication attempts are limited in
process. The defaults allow five failures in five minutes and block that client
for fifteen minutes. They can be changed with
`GROM_AUTH_FAILURE_LIMIT`, `GROM_AUTH_FAILURE_WINDOW`, and
`GROM_AUTH_BLOCK_DURATION`.

For local development:

```bash
cp .env.example .env
make dev
```

This starts the Go backend at `http://localhost:8080` and the Vite frontend at
`http://localhost:5173` in the same terminal. Press `Ctrl+C` to stop both.

To load another environment file:

```bash
make dev DEV_ENV_FILE=.env.development
```

To fully reset the local installation, first stop any running `make dev`
process and then run:

```bash
make reset-local
```

This removes the Grom, Distribution, signing-certificate, and optional
PostgreSQL Compose volumes, orphan containers from the local stack, and the
default `backend/data` and `data` development directories. It preserves
`.env`, installed frontend dependencies, Docker images, and unrelated Docker
resources.

Run all checks with:

```bash
make test
```

Run the real Docker registry acceptance journey with:

```bash
make test-registry-e2e

# Run the browser-driven administrative first-push journey (requires Docker and Playwright Chromium)
make test-admin-e2e

# Run public boot, migration, readiness, and API-documentation acceptance checks
make test-boot-acceptance

# Run the destructive backup and restore recovery journey
make test-backup-restore-e2e
```

This opt-in check requires Docker Engine and Docker Compose. It uses an
isolated Compose project on a random loopback port and removes its temporary
containers, volumes, credentials, and image tags when it finishes.

### Test coverage summary

- `make test` runs the backend unit and integration suite plus frontend lint,
  component tests, and type checking.
- `make test-registry-e2e` proves Docker push/pull authorization, scoped access,
  key revocation, JWT renewal, policy enforcement, and inventory observation
  through Grom's public endpoint.
- `make test-admin-e2e` proves the browser workflow from administrator sign-in
  through project setup, reveal-once access-key handling, first push, and
  repository/manifest browsing.
- `make test-boot-acceptance` proves an empty installation boots, supported
  SQLite state migrates before readiness, failed migrations expose no public
  endpoint, and the packaged OpenAPI documentation is available.
- `make test-backup-restore-e2e` proves a UI-created backup can restore a lost
  default installation, preserve registry content, invalidate old sessions, and
  accept new registry activity.

Installation administrators create and download recovery points from
**Backup & recovery** in the web interface. If the installation volumes are
lost, start the deployment's `recovery` profile and open the loopback recovery
port configured by `GROM_RECOVERY_PORT`. No source checkout or `make` command
is part of the installed backup or recovery workflow. See the
[backup and disaster recovery guide](docs/backup-and-disaster-recovery.md).

## Documentation

- [Product features and business rules](docs/product-features.md)
- [Architecture and MVP plan](docs/architecture-and-mvp.md)
- [Registry E2E implementation plan](docs/registry-e2e-implementation-plan.md)
- [Backup and disaster recovery](docs/backup-and-disaster-recovery.md)
- [Code map](docs/code-map.md)
- [Domain model inventory](docs/domain-model.md)
- [Agent guide](AGENTS.md)
