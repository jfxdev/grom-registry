# Grom product features and business rules

This document describes the behavior that exists in the executable product.
It is the functional reference for product, support, QA, and engineering work.
Architecture and ownership details remain in
[`architecture-and-mvp.md`](architecture-and-mvp.md) and
[`domain-model.md`](domain-model.md).

## 1. Scope and status vocabulary

Grom is a self-hosted control plane and public gateway for an unmodified CNCF
Distribution registry. It adds human access, automation credentials,
project-scoped authorization, repository behavior policies, inventory, and safe
manifest lifecycle operations.

Feature status in this document means:

- **Web and API:** an end user can complete the flow in the management UI, and
  the corresponding backend capability exists.
- **API:** the backend capability and OpenAPI contract exist, but the current UI
  does not expose the complete flow.
- **Internal:** the behavior runs as part of another operation and has no direct
  management screen.
- **Roadmap:** the product intentionally displays or documents the capability,
  but does not execute it.

The management UI is currently available in English.

## 2. Product boundary

Grom owns:

- human users and web sessions;
- service accounts and their registry access keys;
- projects, memberships, and project roles;
- short-lived registry authorization tokens;
- logical repository metadata and behavior policies;
- manifest inventory and passive content classification;
- controlled manifest deletion and retention execution;
- security-sensitive audit event persistence;
- the public HTTP gateway to Distribution.

CNCF Distribution remains the source of truth for manifests, tags, layers,
blobs, uploads, and storage drivers. Grom never stores OCI payloads and does not
reimplement the Docker Registry protocol.

Only Grom is intended to be publicly reachable. Direct public access to
Distribution would bypass Grom's project authorization and repository policies.

## 3. Actors and permissions

### 3.1 Principals

| Principal | Purpose | Authentication |
|---|---|---|
| User | Human access to the management UI | Email and web password |
| Service account | CI, CD, and other automation | Username and a service-account access key |
| Access key | Revocable credential owned by exactly one service account | Used as the Docker/OCI client password |

Human web passwords are never accepted by the registry token endpoint. Access
keys are not web-login credentials.

### 3.2 Installation administrator

An installation administrator can:

- create regular users and promote active users to installation administrators or viewers;
- disable users, revoking their active web sessions;
- generate a password-reset link for a user;
- create and disable service accounts;
- create, list, and revoke service-account access keys;
- create projects;
- view and manage every project;
- delete an empty project;
- perform every repository management operation available to a project admin.

The first user is bootstrapped at startup from configuration and is an
installation administrator. Bootstrap is idempotent after a user already
exists.

### 3.3 Installation viewer

An installation administrator may promote an active regular user to installation
viewer. Viewers cannot administer users and see projects only through explicit
membership. A viewer may create one active, named registry token in their
profile. Its secret is shown once, may be revoked by that viewer, and always
grants only `pull` for projects with membership; it never grants push or delete,
even when the membership role is Writer or Admin.

User creation produces a disabled regular account and a single-use registration
link; consuming it activates the account. A password-reset link changes a
password and revokes sessions, but does not activate a disabled user.

### 3.4 Project roles

| Role | View project and repositories | Pull | Push | Manage members, repositories, policies, and lifecycle |
|---|---:|---:|---:|---:|
| Reader | Yes | Yes | No | No |
| Writer | Yes | Yes | Yes | No |
| Admin | Yes | Yes | Yes | Yes |

Installation administrators can manage all projects regardless of membership.
Project access is calculated from current membership, so changing or removing a
membership affects the next registry-token exchange without rotating an access
key.

External registry clients are never granted the `delete` action.

## 4. Identity and web sessions

**Availability: Web and API**

### Sign in and sign out

- A user signs in with email and password.
- Email matching is normalized to lowercase.
- A successful sign-in creates an opaque server-side session and an HTTP-only,
  SameSite cookie.
- The default session lifetime is 24 hours and is configurable.
- Sign-out removes the current session and clears the cookie.
- Protected UI routes restore the current session before navigation.
- An unauthenticated route request is redirected to sign-in and preserves the
  intended destination.

### User profile and self-service password change

- Every signed-in user can view their username, email, account type, and
  creation date.
- Changing a password requires the current password.
- A new password must contain at least 12 characters.
- Successful password changes are audited.

### Security behavior

- Passwords, session secrets, access-key secrets, and reset-token secrets are
  stored as Argon2id hashes.
- Request bodies are limited to 1 MiB and reject unknown JSON fields.
- State-changing browser requests with a foreign `Origin`, or with a session
  cookie and no `Origin`, are rejected.
- Failed web sign-in and registry-token authentication attempts are limited per
  resolved client address by a bounded in-process limiter. The response is
  `429 Too Many Requests` with `Retry-After` while blocked.
- Forwarded client addresses are used only when the immediate peer belongs to
  an explicitly configured trusted-proxy IP or CIDR range.
- Deployment profiles are explicit: absent configuration defaults to `strict`;
  `development` limits HTTP to loopback; and `permissive` private-network HTTP
  requires an explicit opt-in that produces startup and web-interface warnings.
- API errors include a request ID.

## 5. Installation user administration

**Availability: Web and API; installation administrator only**

An administrator can:

- list human users;
- create a regular user with email and username, then copy a reveal-once
  registration link for them to choose their initial password; the account
  remains disabled until the link is used;
- promote an active regular user to installation administrator;
- generate a reveal-once password-reset URL for an existing user.

Administrators cannot disable their own account or the last active installation
administrator. Disabling a user blocks future sign-in and revokes all active
sessions immediately.

The registration link is single-use, expires after 30 minutes, and carries its
secret in the URL fragment. The user must choose a password of at least 12
characters when registering, changing, or resetting it.

Deletion, email editing, username editing, and administrator demotion are not
currently exposed.

### Administrator password reset

1. An administrator generates a new reset URL.
2. The plaintext token appears only in that response and is placed in the URL
   fragment.
3. The URL expires after 30 minutes.
4. Creating a newer URL invalidates every older unused reset for that user.
5. The signed-out user opens the URL and chooses a password of at least 12
   characters.
6. Successful completion consumes the reset and revokes all of the user's
   existing web sessions.

A reset URL is single-use. A currently authenticated user cannot consume one;
they must change their password from the profile page or sign out first.
Reset-link creation and successful completion are audited without persisting or
logging the plaintext token.

## 6. Service accounts and access keys

### Service accounts

**Availability: Web and API**

- Every signed-in user can view the service-account catalog.
- Only an installation administrator can create or disable a service account.
- A service account has a display name, unique registry username, optional
  description, creation time, and optional disabled time.
- Disabling is security-relevant state; it is not a physical deletion.

The current UI labels listed accounts as active and removes a disabled account
from normal lists after the operation completes.

### Access keys

**Availability: Web and API; installation administrator only**

- Access keys are managed inside their owning service account.
- A key has a name, public identifier, creation time, optional expiration,
  last-used time, and optional revocation time.
- The secret uses the recognizable `grm_<public-id>_<secret>` shape.
- The complete secret is returned only at creation and cannot be retrieved
  again.
- Multiple active keys support credential rotation.
- Revocation blocks the next registry authentication immediately.
- Registry authentication updates the key's last-used time.
- A key cannot be listed or revoked through a different service account.

The API and current UI support an optional key expiration timestamp; key lists
show the configured expiry and distinguish expired keys from revoked keys.

## 7. Projects and memberships

### Project lifecycle

**Availability: Web and API**

- Only an installation administrator can create a project.
- The creator becomes the first project Admin.
- A project has a display name and an immutable lowercase slug.
- The slug is the first path segment of every repository:
  `<registry>/<project-slug>/<repository>:<tag>`.
- Signed-in users see only projects where they are members; installation
  administrators see all projects.
- Only an installation administrator can delete a project.
- Deletion is rejected while any logical repository remains and removes only
  the empty project and its memberships.

Project administrators can archive a logical repository, which blocks new
pushes while retaining pull access and existing OCI content. The current removal
route checks that the archived record has no live inventory and no Distribution
catalog entry; its checks are not yet serialized against concurrent registry
changes, so it is not described as race-safe. The project becomes eligible for
installation-administrator deletion when no logical repositories remain.

### Memberships

**Availability: Web and API**

- Project Admins and installation administrators can list memberships.
- They can assign a user or service account as Reader, Writer, or Admin.
- Saving an existing principal again replaces its role.
- The backend validates that the principal exists.
- The API can remove a membership.

The current UI can add a membership, replace its role, and remove it with an
explicit access-change confirmation. Non-installation project Admins can select
service accounts in the UI; selecting human users still depends on the
installation-admin-only user catalog.

## 8. Registry authentication and gateway

**Availability: Internal protocol behavior**

### Docker/OCI authentication

1. A client reaches `/v2/` through Grom and receives Distribution's Bearer
   challenge.
2. The client calls `/auth/token` with the service-account username and access
   key using Basic authentication.
3. Grom validates the key, its owner, expiration, and revocation state.
4. Grom parses every requested repository scope.
5. The first repository path segment identifies the project.
6. Requested actions are intersected with current project membership.
7. Grom signs a short-lived JWT containing only the allowed actions.
8. Distribution validates that JWT and serves the OCI operation.

The default registry JWT lifetime is five minutes and is configurable.
Unsupported scopes are ignored. Mixed requests receive only the authorized
subset. Registry-wide catalog permission and client-side deletion are not
granted.

### Streaming gateway

- `/v2/*` is streamed to the private Distribution service.
- Layer and blob uploads are not buffered by Grom.
- Manifest PUTs with tag references pass through repository-policy checks.
- Successful manifest PUTs trigger best-effort metadata observation for the
  inventory.
- Digest-addressed manifest PUTs bypass tag mutation policies because they do
  not mutate a tag.

## 9. Logical repositories

### Repository registration

**Availability: Web and API**

A logical repository belongs to one project and stores:

- a relative, lowercase path, including optional nested segments;
- an optional description;
- status (`empty` or `active`);
- creation source (`manual`, `push`, or `reconciled`);
- inferred content profile and confidence;
- an optimistically versioned policy set.

Repository names may use lowercase letters, numbers, `.`, `_`, and `-` inside
path segments. A path is immutable after creation.

Project Admins and installation administrators can manually create a repository
and optionally select policies. Manual creation does not create content in
Distribution; the repository remains empty until the first push.

### First-push provisioning

**Availability: Internal protocol behavior**

When a Writer or Admin requests push scope for a missing repository inside an
existing project:

- Grom idempotently creates the empty logical repository;
- it records `push` as the creation source;
- it creates no policies;
- the same short-lived JWT includes push permission, so no preparatory API call
  is necessary.

A Reader, a pull-only request, or a request for a nonexistent project never
creates a repository or project.

### Reconciliation with existing Distribution content

**Availability: Internal and API**

- Repository listing asks Distribution for repositories under the project
  prefix.
- Existing Distribution repositories missing from Grom are imported as active,
  policy-free logical repositories with `reconciled` creation source.
- Registered repositories not present in an available Distribution catalog are
  marked empty.
- If the Distribution catalog is unavailable, Grom returns stored logical
  repositories without rewriting their status.
- A legacy Distribution repository discovered during a push authorization
  check is imported and remains pushable.

## 10. Repository behavior policies

**Availability: Web and API; project Admin or installation administrator**

Policies belong to exactly one repository. At most 16 policies may be stored in
one set. Replacing a set requires the caller's expected policy version; a stale
version is rejected instead of silently overwriting concurrent changes.

| Policy | Business effect |
|---|---|
| Tag protection | Can prevent overwrite, prevent manual deletion, and exclude matching tags from lifecycle |
| Immutability | Prevents overwriting matching existing tags |
| Retention | Selects content by tag pattern, age, latest-count, and untagged grace period for a lifecycle dry-run |
| Tag naming | Rejects pushed tags that do not match an allowed pattern |
| Manual deletion | Can require an operator-supplied deletion reason |

Patterns use shell-style matching such as `v*`, `prod`, or `pr-*`. Policies can
be enabled or disabled without removing their configuration.

Retention configuration alone never deletes content. Manual deletion and
lifecycle execution are separate, explicit operations.

### Read-only presets

The repository creation form loads five global recommendations:

- protect production tags;
- immutable releases;
- clean temporary builds;
- release tag convention;
- confirm manual deletion.

A preset only copies editable values into the form. It is not inherited,
globally enforced, or linked to the repository after creation.

## 11. Repository browsing

**Availability: Web and API**

- Project members can list logical repositories.
- Repository rows show status, creation source, policy count, inferred profile,
  and whether profile review is needed.
- Opening a repository lists its current Distribution tags.
- A user can copy a `docker pull` command for a selected tag.
- Project Admins can open policy, manual deletion, lifecycle, repository
  archival/removal, and recent deletion-history controls.

Selecting an inventory entry opens a manifest-detail dialog with its digest,
media type, size, timestamps, tags, classification, and OCI relationship.
Config, layer, and pull-history detail are not implemented.

## 12. Manifest inventory and passive classification

### Inventory

**Availability: Web, API, and internal**

Grom stores metadata, not content, for observed manifests:

- digest, media type, artifact type, subject digest, and size;
- known tags and tag movement/detachment timestamps;
- first-seen, last-seen, last-pushed, untagged, and deleted timestamps;
- active, untagged, missing, or deleted state;
- classification kind, relationship, evidence source, and confidence.

Successful tagged manifest pushes are observed automatically. A reconciliation
reads live tags, resolves their manifests, discovers OCI referrers, updates
inventory, and marks disappeared aliases or manifests without discarding
history.

Project members can inspect stored inventory from the selected repository in
the project page. Project Admins can request reconciliation. Manual deletion
previews and lifecycle previews reconcile automatically before making
decisions.

### Repository profile inference

**Availability: Internal, visible in the repository UI**

Grom passively identifies container images, image indexes, Terraform/OpenTofu
modules, SPDX and CycloneDX SBOMs, signatures, Helm charts, generic OCI
artifacts, and unknown OCI content.

Profiles exposed on repositories are `unknown`, `container_image`,
`terraform_module`, `sbom`, `generic_oci`, or `mixed`.

Only tagged primary manifests can influence the profile. OCI referrers such as
signatures and SBOM attachments remain inventoried but never change it.
Conflicting specific primary content can produce `mixed` and
`profileNeedsReview=true`. Inference is informational: it never enables a
policy, changes authorization, or rejects a push.

## 13. Manual artifact deletion

**Availability: Web and API; project Admin or installation administrator**

Manual deletion is a two-step, digest-safe flow:

1. The operator selects a tag or digest.
2. Grom reconciles inventory and resolves the reference to a digest.
3. The preview lists every tag currently pointing to that digest.
4. Repository deletion policies are evaluated.
5. OCI subject/referrer relationships are checked.
6. The operator supplies a reason when policy requires it.
7. Execution repeats the preview and requires the expected digest and tag set
   to match.
8. Grom deletes the manifest by digest through its internal Distribution
   credential.
9. The result is persisted, inventory is updated, and the operation is audited.

Deletion is blocked when a protected tag points to the digest, when the artifact
is an OCI referrer, or when it has OCI referrers. Cascade deletion is not
implemented.

Deleting a manifest does not immediately reclaim blob storage. Distribution
garbage collection is a separate operator action.

## 14. Retention lifecycle

**Availability: Web and API; project Admin or installation administrator**

Lifecycle is always manual and starts with a persisted dry-run:

1. Grom reconciles the repository with Distribution.
2. Enabled retention policies evaluate tagged content and untagged grace
   periods.
3. Tag protection and lifecycle exclusions are applied.
4. OCI referrer artifacts and subjects with referrers are blocked.
5. The preview records eligible, retained, and blocked items, the policy
   version, evaluator version, inventory time, and a policy snapshot.
6. The preview expires after 15 minutes.
7. The operator must provide an execution reason.
8. Each eligible digest is reconciled and re-evaluated immediately before its
   deletion.
9. Changed, missing, or no-longer-eligible artifacts are skipped.
10. Per-item results and the completed, partially completed, or failed run are
    persisted and audited.

A preview can create at most one run. Repository-level execution locking
prevents concurrent lifecycle runs. A stale interrupted run is failed before a
later run proceeds.

There is no scheduler, automatic purge, background lifecycle worker, or cascade
deletion. Blob garbage collection remains separate.

## 15. Audit trail

**Availability: Internal persistence only**

The current implementation records:

- user password changes;
- administrator password-reset-link creation;
- password-reset completion;
- repositories provisioned by first push;
- repository policy replacements;
- manual artifact deletion start, completion, and failure;
- lifecycle preview creation;
- lifecycle run start, per-item outcomes, completion, and failure.
- successful and failed sign-in attempts;
- user and service-account creation and disabling;
- access-key creation and revocation;
- project creation and deletion;
- membership creation, replacement, and removal.

Audit events are immutable database records with actor, action, resource, JSON
metadata, and timestamp.

There is currently no audit-event listing endpoint or audit page. Events remain
available only through durable backend persistence in the MVP.

## 16. Operational capabilities

**Availability: Web, API, and runtime**

- One Go process serves the management API, registry-token service, streaming
  gateway, and embedded Vue application.
- SQLite is the default database; PostgreSQL is also supported through Bun.
- Reviewed versioned migrations run automatically before HTTP readiness.
- A migration failure prevents startup.
- Distribution runs as a separate private process and can use its supported
  filesystem or object-storage configuration.
- `/healthz` reports process health and `/readyz` reports readiness.
- OpenAPI is the source of truth for management and token endpoints.
- When enabled, raw OpenAPI is served at `/api/openapi.yaml` and interactive
  documentation at `/api/docs`.
- Local Compose profiles support the default SQLite stack and an optional
  PostgreSQL stack.
- Installation administrators create, monitor, list five at a time, download,
  and delete complete SQLite or PostgreSQL recovery points with local
  Distribution storage from **Backup & recovery**. Local deletion requires
  typed confirmation and does not affect previously downloaded bundles.
- Backup briefly drains and blocks mutations and registry traffic while an
  isolated, networkless agent snapshots the recovery-critical volumes.
- The same Grom image provides a standalone, loopback-only recovery UI that
  validates an uploaded bundle and restores empty volumes after total loss.
- The installed backup and recovery workflow does not require a source
  checkout, `make`, a Docker socket, or a shell inside the application.

Production installations require HTTPS and secure cookies. Startup rejects a
non-local public URL that does not satisfy both requirements. The default
SQLite configuration is designed for a small single-instance
installation; active-active Grom replicas are not supported.

## 17. Current channel coverage

| Capability | Web UI | API | Internal behavior |
|---|---:|---:|---:|
| Sign-in, sign-out, current profile, password change | Yes | Yes | — |
| User creation, disabling, and reset-link generation | Yes | Yes | Session revocation on disable |
| User delete/edit | No | No | No |
| Service-account create/list/disable | Yes | Yes | — |
| Access-key create/list/revoke | Yes | Yes | — |
| Access-key expiration selection | Yes | Yes | Enforced |
| Project create/list/detail/delete-empty | Yes | Yes | — |
| Membership list/add | Yes | Yes | — |
| Membership role replacement/removal | Yes | Yes | Enforced |
| Repository manual creation and policy selection | Yes | Yes | — |
| First-push repository provisioning | Indicated | — | Yes |
| Existing repository reconciliation | Reflected | Yes | Yes |
| Logical repository archival/removal | Yes | Yes | Enforced |
| Tag browsing and pull-command copy | Yes | Yes | — |
| Inventory read/reconciliation | Yes | Yes | Yes |
| Passive profile inference | Displayed | Returned | Yes |
| Manual deletion preview/execution/history | Yes | Yes | Yes |
| Lifecycle preview/execution/history | Yes | Yes | Yes |
| Backup creation/paginated list/download/confirmed deletion | Yes | Yes | Isolated agent |
| Empty-volume disaster recovery | Local recovery UI | Local recovery service | Staged restore |
| Audit event recording | No | No | Yes |

## 18. Explicitly not implemented

The executable product does not currently provide:

- organizations, teams, or nested authorization groups;
- user deletion/editing;
- a full manifest-detail page with config, layer, or pull-history data, or an
  audit-log screen;
- vulnerability scanning or admission policies;
- automatic retention scheduling or autopurge;
- cascade deletion of OCI subjects and referrers;
- Distribution blob garbage collection from the management plane;
- registry mirroring or pull-through cache;
- OIDC or enterprise identity providers;
- multi-instance active-active operation;
- scheduled backups, remote-backup credentials, or S3 recovery;
- billing, quotas, replication, or a Kubernetes operator.

These omissions are product boundaries, not implicit partial features.

## 20. Primary end-to-end workflow

For a new automation client:

1. An installation administrator creates a project.
2. The administrator creates a service account.
3. A project Admin assigns it a Writer or Admin membership.
4. The installation administrator creates an access key and copies the secret.
5. The automation uses the service-account username and access key with
   `docker login`.
6. Its first push can provision the missing logical repository inside the
   existing project.
7. A project Admin can later configure repository policies.
8. Successful pushes populate inventory and may infer a repository profile.
9. Project members browse repositories and tags in the UI.
10. Project Admins review and explicitly execute safe manual deletion or
    retention operations when needed.
