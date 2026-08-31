<p align="center">
  <img src="frontend/src/assets/logos/grom-registry-logo.png" alt="Grom Registry" width="420" />
</p>

[![Release](https://img.shields.io/github/v/release/jfxdev/grom-registry?display_name=tag)](https://github.com/jfxdev/grom-registry/releases)
[![License](https://img.shields.io/github/license/jfxdev/grom-registry)](LICENSE)
[![GHCR](https://img.shields.io/badge/GHCR-ghcr.io%2Fjfxdev%2Fgrom--registry-2496ed?logo=github)](https://github.com/jfxdev/grom-registry/pkgs/container/grom-registry)
[![Go](https://img.shields.io/badge/Go-1.26.5-00ADD8?logo=go)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D?logo=vuedotjs)](https://vuejs.org/)
[![shadcn-vue](https://img.shields.io/badge/shadcn--vue-UI-000000?logo=shadcnui&logoColor=white)](https://www.shadcn-vue.com/)
[![OCI](https://img.shields.io/badge/OCI-registry-262261?logo=docker)](https://opencontainers.org/)

[![codecov](https://codecov.io/gh/jfxdev/grom-registry/graph/badge.svg?token=5NOmSFnvkT)](https://codecov.io/gh/jfxdev/grom-registry)
[![CI](https://github.com/jfxdev/grom-registry/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/ci.yml)
[![CodeQL](https://github.com/jfxdev/grom-registry/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/codeql.yml)
[![Registry E2E](https://github.com/jfxdev/grom-registry/actions/workflows/registry-e2e.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/registry-e2e.yml)
[![Admin Journey E2E](https://github.com/jfxdev/grom-registry/actions/workflows/admin-journey-e2e.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/admin-journey-e2e.yml)
[![Boot Acceptance E2E](https://github.com/jfxdev/grom-registry/actions/workflows/boot-acceptance-e2e.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/boot-acceptance-e2e.yml)
[![Backup Restore E2E](https://github.com/jfxdev/grom-registry/actions/workflows/backup-restore-e2e.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/backup-restore-e2e.yml)
[![Release Upgrade E2E](https://github.com/jfxdev/grom-registry/actions/workflows/release-upgrade-e2e.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/release-upgrade-e2e.yml)
[![Production Image Smoke](https://github.com/jfxdev/grom-registry/actions/workflows/production-image-smoke.yml/badge.svg?branch=main)](https://github.com/jfxdev/grom-registry/actions/workflows/production-image-smoke.yml)

Grom is a lightweight, self-hosted OCI registry for individuals and small
teams. It combines CNCF Distribution with a secure control plane and a simple
web interface for access, images, and recovery.

## Contents

- [Product](#product)
  - [Features](#features)
  - [How access works](#how-access-works)
  - [Safe repository operations](#safe-repository-operations)
- [Installation](#installation)
  - [Local quick start](#local-quick-start)
  - [Push an image](#push-an-image)
  - [Deployment profiles](#deployment-profiles)
- [Operations](#operations)
  - [Backup and recovery](#backup-and-recovery)
  - [Releases and upgrades](#releases-and-upgrades)
- [Compatibility policy](#compatibility-policy)
- [Development](#development)
  - [Common commands](#common-commands)
  - [Acceptance commands](#acceptance-commands)
  - [Architecture](#architecture)
- [Supported scope](#supported-scope)
- [References](#references)

## Product

### Features

- Project-based Docker image push and pull.
- Reader, Writer, and Admin roles.
- Service accounts with reveal-once, revocable access keys.
- Web management for users, projects, repositories, and policies.
- Live-versus-historical repository inventory, safe image-index deletion
  previews, and manual retention runs.
- Built-in backup and loopback-only recovery.

### How access works

Projects are the security boundary. Readers can pull, Writers can push, and
project Admins manage project settings. Administrators create
projects, users, service accounts, and recovery points.

Human passwords never work as registry credentials. New users set their own
password through a single-use registration link. Service-account keys are shown
once and can be revoked at any time.

### Safe repository operations

Writers can create a repository on first push. The UI shows repositories, tags,
and manifests without giving registry clients delete permission. Archiving stops
new pushes while keeping pulls available.

Deletion and retention are previewed first and require explicit confirmation.
Grom protects OCI subject/referrer relationships and never deletes OCI content
when only a logical repository record is removed.

## Installation

### Local quick start

```bash
cp .env.example .env
docker compose --env-file .env -f deploy/compose/docker-compose.yml up --build
```

Open `http://localhost:8080` and sign in with the bootstrap credentials in
`.env`. Change the example password before exposing Grom outside localhost.

### Push an image

Create a project, service account, membership, and access key in the web UI.
Then use the project slug as the first path segment:

```bash
docker login localhost:8080
docker tag my-image:latest localhost:8080/my-project/my-image:latest
docker push localhost:8080/my-project/my-image:latest
```

### Deployment profiles

| Profile | Use |
|---|---|
| `development` | Local HTTP on loopback only. |
| `permissive` | Trusted private networks; HTTP needs explicit opt-in and shows a warning. |
| `strict` | Default for production; requires HTTPS and secure cookies. |

For a reverse proxy, set `GROM_PUBLIC_URL`, keep `GROM_SECURE_COOKIES=true`,
and list only immediate proxy networks in `GROM_TRUSTED_PROXIES`.

## Operations

### Backup and recovery

Administrators create and download recovery points from **Backup
& recovery**. Grom pauses writes briefly, verifies the bundle, and resumes
normal work. Store downloaded bundles encrypted and off-host.

For volume loss, start the same image in the loopback-only `recovery` profile
(port `8081` by default). Recovery accepts only verified bundles and empty
target volumes. Restored web sessions and reset links are invalidated on the
first normal boot.

### Releases and upgrades

Stable tags publish the application image to `ghcr.io/jfxdev/grom-registry` and
the dedicated maintenance image to
`ghcr.io/jfxdev/grom-registry-maintenance:vMAJOR.MINOR.PATCH`. Use immutable
digests from the GitHub Release, not mutable tags. Releases include an SBOM,
vulnerability report, digest reference, and checksums for both images.

Before upgrading, create and download a verified recovery point. Set both
`GROM_IMAGE` and `GROM_REGISTRY_MAINTENANCE_IMAGE` to the matching release
image digests, start with `docker compose pull` and `docker compose up -d
--no-build`, then check `/readyz` and `/api/docs`. Do not downgrade a database
by changing only the image; restore a compatible backup.

## Compatibility policy

Starting with a `v1.0.0` release, `/api/v1` and `/auth/token` are public
contracts. Clients compatible with `/api/v1` must continue to work throughout
the `1.x` release line. The unversioned `/auth/token` follows the same
compatibility and successor-versioning mechanism as `/api/v1`; an incompatible
change must move to a new documented versioned auth endpoint alongside the new
API version, never silently alter `/auth/token`.

- Patch releases contain fixes and documentation changes only; they do not make
  incompatible contract changes.
- Minor releases may add opt-in capabilities, endpoints, and optional fields.
- Removing or renaming fields or endpoints, changing field types or semantics,
  tightening authorization or validation for an accepted request, or adding a
  value to an existing closed enum is incompatible.
- Incompatible changes require a new API version such as `/api/v2`; they must
  not silently alter `/api/v1`.

Deprecated operations and fields are marked in OpenAPI, identify their
replacement in release notes, and remain available for at least two minor
releases or 180 days, whichever is longer. Security fixes may shorten that
window and will describe the impact and migration path in the release notes.

A `v1.0.0` release establishes the initial OpenAPI contract baseline and does
not require a comparison. Each later stable release requires CI to compare its
OpenAPI contract with the previous stable contract. An incompatible change
requires an approved exception, a documented migration path, and release
notes. This policy covers the published management API and token endpoint;
Docker is the supported registry-client compatibility surface. It does not
claim generic OCI/ORAS, S3, or multi-instance support.

## Development

### Common commands

```bash
# Start the Go backend and Vue frontend
make dev

# Start local development with PostgreSQL
make dev-postgres

# Build and run the regular quality gate
make build
make test

# Build and push the image configured by GROM_IMAGE in .env
make image-publish-local
```

For a local registry, set `GROM_IMAGE` in `.env` to its full image name, for
example `localhost:5000/grom-registry:local`.

### Acceptance commands

| Command | Coverage |
|---|---|
| `make test-registry-e2e` | Public Docker authorization, push/pull, policies, inventory, GC, and tag republishing. |
| `make test-admin-e2e` | Browser-based administrator and first-push flows. |
| `make test-boot-acceptance` | Boot, migrations, readiness, and API docs. |
| `make test-backup-restore-e2e` | SQLite backup and recovery. |
| `make test-backup-restore-postgres-e2e` | PostgreSQL backup and recovery. |
| `make test-release-upgrade-e2e` | Upgrade from a tagged release. |
| `make test-production-image-smoke` | Clean production image and public runtime. |

Docker acceptance tests use isolated resources only and require Docker Engine
and Docker Compose.

### Architecture

The Go backend owns four small areas: **Identity** (users and credentials),
**Projects** (roles), **Registry** (repositories and tokens), and **Audit**.
CNCF Distribution remains private and stores OCI payloads; Grom is the only
public entry point.

The Vue 3 / TypeScript frontend uses the API contract at
`backend/api/openapi.yaml`. Run `make generate` after changing that contract;
never edit generated code directly.

## Supported scope

The supported path is one active installation with local registry storage and
Docker image push/pull. High availability, S3 storage, replication, enterprise
identity, generic OCI/ORAS support, automatic retention purging, and full audit
browsing are outside the supported platform scope.

## References

The current platform reference is organized as
[architecture](docs/architecture.md) and
[RBAC](docs/rbac.md),
[data and disaster recovery](docs/data-and-disaster-recovery.md), alongside the
[contribution guide](AGENTS.md).
