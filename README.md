# Grom

[![codecov](https://codecov.io/gh/jfxdev/grom-registry/graph/badge.svg?token=5NOmSFnvkT)](https://codecov.io/gh/jfxdev/grom-registry)

Lightweight OCI registry with project-scoped access control.

## What is included

- CNCF Distribution v3 as the OCI/Docker data plane.
- Go control plane and streaming registry gateway.
- SQLite by default, with PostgreSQL support through Bun.
- Automatic versioned migrations during boot.
- Project-scoped Reader, Writer, and Admin roles.
- Human users, service accounts with reveal-once access keys, and short-lived registry JWTs.
- Vue 3 management interface using the shadcn-vue component system.
- Contract-first OpenAPI documentation at `/api/docs`.
- OCI manifest inventory with retention dry-runs and audited manual lifecycle execution.
- Passive repository-profile inference for container images, Terraform modules, SBOMs, and generic OCI content.
- Installation-admin backup UI and same-image, loopback-only disaster-recovery UI.
- Read-only integrations roadmap.

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
```

This opt-in check requires Docker Engine and Docker Compose. It uses an
isolated Compose project on a random loopback port and removes its temporary
containers, volumes, credentials, and image tags when it finishes.

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
