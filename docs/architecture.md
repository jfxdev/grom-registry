# Grom platform architecture

## Overview

Grom is a self-hosted OCI registry platform. It provides the control plane, web
interface, and public gateway. CNCF Distribution runs unchanged behind it,
privately, and remains the source of truth for manifests, tags, blobs, uploads,
and OCI storage. Grom does not store OCI payloads or reimplement the Docker
Registry protocol.

The supported deployment is one active Grom instance, using SQLite by default or
PostgreSQL, local Distribution storage, and Docker image push and pull. Grom is
the only public entry point.

~~~mermaid
flowchart LR
  Browser["Browser"] --> Grom["Grom: UI, API, authentication, and gateway"]
  Client["Docker/OCI client"] --> Grom
  Grom --> DB[("SQLite or PostgreSQL")]
  Grom --> Distribution["Private CNCF Distribution"]
  Distribution --> Storage[("Local OCI storage")]
  Grom --> Backup["Backup agent over a Unix socket"]
  Backup --> Recovery["Recovery bundles"]
~~~

## Runtime components

| Component | Responsibility |
|---|---|
| Grom | Serves the embedded Vue interface, management API, sessions, /auth/token, and the streaming /v2/* gateway. |
| CNCF Distribution | Implements the OCI/Docker protocol and persists OCI content. |
| SQLite or PostgreSQL | Stores control-plane data and audit history; migrations run before readiness. |
| Backup agent | Creates and provides recovery points with no network, public ports, or Docker socket. |
| Recovery UI | A separate mode of the same image, limited to loopback, which restores only empty volumes. |

/healthz reports process health and /readyz reports readiness. A failed migration
prevents startup.

## Domain boundaries

The Go backend uses pragmatic bounded contexts. A context never reads another
context's tables directly; cross-context work goes through narrow application
interfaces.

| Context | Owns | Does not own |
|---|---|---|
| Identity | Users, registration and reset links, sessions, service accounts, and credentials. | Project authorization. |
| Projects | Projects, memberships, and Reader, Writer, and Admin roles. | Credential verification. |
| Registry | Logical repositories, policies, JWT grants, inventory, safe deletion, retention, and the /v2 gateway. | Blobs, uploads, and OCI protocol implementation. |
| Audit | Immutable records of sensitive actions. | Business state and an audit UI. |

Shared foundation types live in backend/internal/foundation/: ID, PrincipalRef,
PageRequest, PageResult, PageCursor, Timestamps, FieldError, and AppError.
Stable backend constants live in backend/internal/constants/.

Domain entities do not depend on Bun, HTTP, generated OpenAPI types, or another
context's infrastructure. Repository interfaces represent domain capabilities;
Bun implementations live in the owning context.

## Functional model

| Area | Current entities and behaviour |
|---|---|
| Identity | User, Session, PasswordReset, ServiceAccount, APIToken, and ViewerRegistryToken. Secrets are shown only when created and stored as hashes. |
| Projects | Project, Membership, ProjectSlug, ProjectRole, and AccessDecision. The immutable slug is the OCI authorization boundary. |
| Registry | Repository, Policy, ManifestInventory, ArtifactDeletion, LifecyclePreview, LifecycleRun, and logical-usage snapshots. A repository belongs to one project. |
| Audit | AuditEvent, AuditAction, and AuditResource. Events never contain secrets. |

## Registry flow

1. A client reaches /v2/ through Grom and receives Distribution's Bearer
   challenge.
2. It calls /auth/token with a service-account username and access key. Web
   passwords never authenticate registry clients.
3. Grom validates the key, owner, expiry, and revocation. It maps the first
   segment of each requested scope to a project and intersects requested actions
   with the principal's current membership.
4. Grom signs a short-lived JWT containing only allowed actions. Distribution
   validates it and performs the OCI operation.

The gateway streams uploads without buffering. Tagged manifest PUT requests pass
through repository policies, and successful pushes attempt an inventory
observation. Digest-addressed PUT requests do not mutate tags, so they do not
pass through tag-mutation rules.

A Writer or Admin can create a missing logical repository idempotently on the
first push when the project already exists. Pulls, Readers, and nonexistent
projects never create projects or repositories. External clients never receive
the OCI delete action.

## Repositories, inventory, and safe operations

A logical repository has an immutable relative path, optional description,
status, creation source, inferred content profile, and an optimistically
versioned policy set. Manual creation does not create content in Distribution.
Archiving blocks pushes and retains pulls; project administrators can unarchive
the repository to restore pushes. Removing the logical record requires an
archived repository, no Distribution catalog entry, and no live inventory. It
never deletes OCI content.

Grom stores manifest, tag, OCI-relationship, and platform metadata, not
payloads. Reconciliation imports legacy content, updates active and untagged
records, and keeps disappeared items as missing or deleted history. Profiles are
inferred passively from tagged primary manifests; referrers such as signatures
and SBOMs do not change profiles, policies, or authorization.

Accounted usage is the logical sum of live, unique OCI descriptors in a scope.
It is neither the physical volume size nor the space a garbage collection must
reclaim. A shared digest is counted in every project that references it.

Repository policies are replaced under policyVersion. They cover tag protection,
immutability, retention, tag naming, and deletion reasons. Global presets are
form recommendations only; they have no runtime effect.

Each retention policy has independent expiry-by-age, newest-artifacts, and
untagged-grace criteria. Operators enable only the criteria needed for that
policy and may preserve a disabled criterion's value for a later re-enable;
older policies infer enabled criteria from their configured limits.

- Manual deletion creates a preview, reconciles, resolves a digest, and
  revalidates tags, policies, and OCI relationships before deletion.
- Subjects with referrers and referrer artifacts are protected. An image index
  may remove only untagged children that have no tags, other indexes, or
  referrers.
- Retention starts with a stored preview, expires after 15 minutes, and runs
  manually with a reason and per-digest revalidation.
- Blob garbage collection is a separate platform operation. Grom blocks
  mutations and registry traffic, stops Distribution, runs exclusive collection,
  and starts a fresh process.

## Persistence, contract, and frontend

SQLite and PostgreSQL are supported through Bun, and shared queries must remain
portable. Ordered migrations under backend/migrations/ are the only supported
schema-change mechanism; AutoMigrate and runtime schema diffs are not used.

backend/api/openapi.yaml is the source of truth for /api/v1/*, health,
readiness, and /auth/token. Change the contract before implementation, run make
generate, and never edit backend/internal/generated/openapi/ or
frontend/src/shared/api/generated/. The /v2/* gateway follows OCI/Distribution
rather than duplicating every protocol operation in OpenAPI. Lists use stable,
opaque cursors rather than totals or numbered pages.

The frontend uses Vue 3, TypeScript, Vite, Vue Router, shadcn-vue, and TanStack
Query for server state. Pinia is reserved for genuinely shared client state.
Product modules own their queries, pages, and components; code moves to shared
only after real reuse.

Project-membership listings are enriched at the HTTP boundary through the
Identity application service: user memberships expose username and email, while
service-account memberships expose display name and registry username. A
membership-name search first resolves matching principals through Identity, then
passes only those principal references to the Projects pagination repository;
Projects never reads Identity tables directly.

## Security and deployment

| Profile | Use | HTTP rules |
|---|---|---|
| development | Local development | HTTP only on loopback. |
| permissive | Trusted private networks | Private HTTP requires explicit opt-in and produces warnings. |
| strict | Default and exposed installations | HTTPS and secure cookies are required. |

A non-local GROM_PUBLIC_URL requires HTTPS and GROM_SECURE_COOKIES=true.
Forwarded addresses are trusted only when the immediate peer is in
GROM_TRUSTED_PROXIES. Passwords, access keys, sessions, and reset tokens use
Argon2id. Request bodies are limited to 1 MiB and closed schemas reject unknown
properties. Cookie-authenticated mutations require an allowed Origin, and
login/token failures are rate-limited by resolved client address.

## Backup and recovery

Administrators create, inspect, download, and delete recovery
points in the UI. Grom drains writes and blocks mutations, token exchange, and
registry traffic while the isolated agent captures application data, the
database, signing material, OCI storage, and Distribution configuration. Local
deletion requires confirmation and does not affect downloaded bundles. Bundles
contain private data and must be retained off-host with authenticated encryption.

Recovery mode validates the bundle, refuses incompatible versions and non-empty
targets, restores in stages, and never starts normal services. On the first
normal boot, Grom applies compatible migrations, invalidates restored web
sessions and reset links, records the recovery, and consumes its marker. See
[Data, storage, and disaster recovery](data-and-disaster-recovery.md) for the
complete recovery flow and the distinction between accounted and physical usage.

## Repository layout

| Path | Responsibility |
|---|---|
| backend/cmd/grom/ | Main process. |
| backend/cmd/grom-backup/ | Backup agent, recovery, and compatibility tools. |
| backend/internal/{identity,projects,registry,audit}/ | Domain, application, and infrastructure contexts. |
| backend/internal/httpapi/ | Central HTTP routing and adaptation. |
| backend/internal/platform/ | Configuration, database, lifecycle, backup, maintenance, and Distribution supervision. |
| frontend/src/modules/ | Auth, users, service accounts, projects, registry, backups, and settings. |
| frontend/src/shared/ | API client, generated types, reusable UI, constants, and utilities. |
| deploy/compose/, deploy/docker/, deploy/distribution/ | Deployment, image entrypoint, and private Distribution configuration. |
| deploy/backup/ | Development and compatibility tools, not a product interface. |

## Current boundaries

Grom does not provide high availability, multiple active instances, S3,
replication, pull-through cache, organizations or teams, OIDC/SAML/LDAP,
scanning or admission policies, subject/referrer cascade deletion, automated
retention purging, scheduled or remote backups, audit browsing, billing, quotas,
or a Kubernetes operator.

## Keeping this document current

Update this document when topology, contexts, data ownership, paths, platform
flows, or supported boundaries change. Update docs/rbac.md when identities,
roles, credentials, or permissions change, and update
docs/data-and-disaster-recovery.md when backup, recovery, storage, or garbage
collection changes.
