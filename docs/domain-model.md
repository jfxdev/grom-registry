# Grom domain model inventory

This file is the canonical inventory of fundamental structs, domain entities, value objects, and their ownership.
It is intended to make architectural types easy to locate and prevent duplicate definitions.

## Fundamental structs

Target package: `backend/internal/foundation`.

| Struct | Purpose | Key fields |
|---|---|---|
| `ID` | Application-generated portable identifier | String UUID value |
| `PrincipalRef` | Reference to a user or service account across contexts | Principal kind and ID |
| `PageRequest` | Validated pagination input | Cursor and limit |
| `PageResult[T]` | Generic paginated result | Items and next cursor |
| `Timestamps` | Common creation/update metadata where both are meaningful | CreatedAt and UpdatedAt |
| `FieldError` | Stable field-level validation failure | Field, code, message |
| `AppError` | Application-wide classified error | Code, message, fields, cause |

These structs must remain free of Bun persistence tags and HTTP framework dependencies.

## Identity context

| Type | Kind | Responsibility |
|---|---|---|
| `User` | Entity | Human identity, password credential, and account status |
| `PasswordReset` | Entity | Expiring, single-use password setup or reset capability stored only as a public identifier and secret hash |
| `ServiceAccount` | Entity | Non-human automation identity |
| `APIToken` | Child entity | Revocable long-lived registry credential owned by exactly one `ServiceAccount` through `ServiceAccountID` |
| `Session` | Entity | Web authentication session |
| `Username` | Value object | Normalized registry login name |
| `Email` | Value object | Normalized human email |
| `TokenSecret` | Value object | Reveal-once plaintext used only at creation/verification boundaries |

The bootstrap path creates the first user as the installation administrator.
Every later user is created with regular access and a reveal-once registration
link to choose an initial password. The user stays disabled until that link is
consumed; only an active installation administrator may promote a user to
installation administrator. Identity verifies credentials but does not decide
repository access. On the
first boot after a recovery restore, Identity atomically invalidates all
restored web sessions and password-reset capabilities. Service-account API
tokens remain durable credentials at the selected recovery point.

## Projects context

| Type | Kind | Responsibility |
|---|---|---|
| `Project` | Entity | Immutable project slug, display metadata, and administrator-controlled lifecycle |
| `Membership` | Entity | Principal assignment and project role |
| `ProjectSlug` | Value object | First repository path segment and authorization boundary |
| `ProjectRole` | Value object | Reader, Writer, or Admin |
| `AccessDecision` | Value object | Allowed subset of requested project actions |

Projects owns the authorization policy that maps memberships and roles to allowed actions.
Installation viewers are global read-only users: they cannot access user management,
and only receive project or registry visibility through an explicit project membership.
Their profile-scoped registry API tokens authenticate as the user but are always
reduced to `pull`, irrespective of the membership role.
Only installation administrators create or delete projects. Deletion is rejected
while Registry reports any logical repository for the project; an accepted
deletion cascades only the now-empty project's memberships.

## Registry context

| Type | Kind | Responsibility |
|---|---|---|
| `RepositoryScope` | Value object | Parsed repository name and requested actions |
| `RepositoryName` | Value object | Project-prefixed OCI repository path |
| `RegistryAction` | Value object | Pull, push, or delete request |
| `TokenGrant` | Value object | Authorized repository actions encoded in a short-lived JWT |
| `RepositorySummary` | Read model | Repository data presented in the management API |
| `ManifestSummary` | Read model | Digest, media type, size, and available metadata |
| `Repository` | Entity | Project-owned logical OCI repository with creation provenance, a passive inferred content profile, an optimistic policy-set version, and an `archived` state that blocks new pushes while preserving pull access |
| `Policy` | Child entity | Typed, replaceable repository behavior for protection, immutability, retention, tag naming, or manual deletion |
| `PolicyPreset` | Read model | Global read-only recommendation used only to populate the repository creation form |
| `ArtifactDeletionPreview` | Read model | Digest, aliases, OCI relationships, and deletion requirements resolved immediately before deletion |
| `ArtifactDeletion` | Entity | Persisted, audited result of one manually confirmed manifest deletion |
| `ManifestInventory` | Entity | Historical metadata for a Distribution manifest, including aliases, observation times, OCI type, and subject relationship |
| `ManifestObservation` | Value object | Metadata captured from a successful manifest push or reconciliation |
| `ManifestClassification` | Value object | Observed OCI kind, primary/referrer relationship, inferred repository profile, evidence source, and confidence |
| `LifecyclePreview` | Entity | Persisted, expiring dry-run calculated from a reconciled inventory, policy-set version, and evaluator version |
| `LifecyclePreviewItem` | Child entity | Digest-level eligible, retained, or blocked lifecycle decision with expected aliases and reasons |
| `LifecycleRun` | Entity | Audited manual execution created from exactly one valid preview |
| `LifecycleRunItem` | Child entity | Revalidated digest deletion, skip, or failure result |

Repository policies belong to exactly one repository and therefore inherit its
project boundary. The complete set is replaced under an optimistic repository
policy version, allowing push-created and reconciled repositories to receive
policies after creation without silently overwriting concurrent changes.
Presets are not persisted with projects or repositories and have no runtime
effect by themselves. Registry does not own project roles or identity
credentials.

Projects are created only by installation administrators. When an authenticated
registry client requests push scope, Grom may idempotently create an empty
logical repository only when its project already exists and the principal
currently has Writer or Admin membership. The same short-lived token grants
push, allowing the first upload to succeed. Reader access and pull requests
never create repositories.

Distribution remains the content source of truth. The Registry inventory is the
historical decision index used for lifecycle behavior; it never stores manifest
or blob payloads. Manifests participating in an OCI subject/referrer relationship
are blocked from lifecycle and manual deletion until an explicit cascade policy
exists.
Only tagged primary manifests influence a repository profile. Auxiliary referrers
remain visible in the inventory without changing the profile. Conflicting
specific primary classifications produce a reviewable `mixed` profile; inference
never activates a policy or rejects content.

An archived repository remains a logical record until an administrator removes
it. Removal never deletes OCI content: it requires the repository to be
archived, absent from the Distribution catalog, and free of live inventory
manifests (states other than deleted or missing). Both transitions are audited.

## Audit context

| Type | Kind | Responsibility |
|---|---|---|
| `AuditEvent` | Entity | Immutable record of a security-sensitive action |
| `AuditAction` | Value object | Stable event action identifier |
| `AuditResource` | Value object | Referenced resource kind and ID |

The Audit context records authentication outcomes, user and service-account
administration, access-key changes, project and membership changes, user
password changes, administrator reset-link creation, completed password resets,
lifecycle previews, and manual-execution events.
Detailed per-digest outcomes remain in Registry lifecycle run items.
Audit also records sanitized `platform.backup_created`,
`platform.backup_delete_requested`, and `platform.backup_deleted` events, plus
an idempotent `platform.restore_completed` event keyed by the backup ID after
restored ephemeral credentials are invalidated. Backup manifests, operations,
pagination cursors, and restore markers are platform-operational records rather
than domain entities.

## Constants ownership

Stable backend constants are defined in `backend/internal/constants`, split by concern.
Externally visible enum values are described by OpenAPI and generated for the frontend.
Frontend-only constants are defined under `frontend/src/shared/constants`.

Do not manually duplicate server-owned roles, statuses, principal kinds, or registry actions in frontend code.

## Update checklist

When adding or changing an architectural type:

1. Confirm its owning bounded context.
2. Reuse a fundamental struct when semantics match exactly.
3. Add a new foundation struct only when multiple contexts require identical semantics.
4. Keep ORM models and transport DTOs out of this inventory unless they become architectural contracts.
5. Update this file and `docs/code-map.md` in the same change.
