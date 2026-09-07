# RBAC and access control

## Purpose

Grom authorizes access at the project level. A project's immutable slug is the
first segment of every OCI repository path, which makes it the security
boundary:

~~~text
<registry>/<project-slug>/<repository>:<tag>
~~~

Identity authenticates people and service accounts. Projects decides what a
principal can do in a project. Registry turns only that current decision into a
short-lived registry JWT.

## Principals

| Principal | Purpose | Authentication |
|---|---|---|
| Regular user | Human access to the UI and actions granted through project membership. | Email and web password. |
| Administrator | Global administration and access to every project. | Email and web password. |
| Viewer | Installation-level read-only user who can see only explicitly assigned projects. | Email and web password; an optional profile token for pulls. |
| Service account | CI/CD and other registry automation. | Registry username and access key. |

A web password never works at the registry-token endpoint. A service-account
access key is never a web-login credential.

## Project roles

Memberships connect a user or service account to a project. A principal can have
only one membership in a project; assigning it again replaces the current role.

| Role | View project, repositories, and inventory | OCI pull | OCI push | Manage members | Manage repositories, policies, deletion, and retention |
|---|---:|---:|---:|---:|---:|
| Reader | Yes | Yes | No | No | No |
| Writer | Yes | Yes | Yes | No | No |
| Admin | Yes | Yes | Yes | Yes | Yes |

Administrators have project-Admin permissions for every project
without membership. Changing or removing membership affects the next registry
token exchange immediately; access-key rotation is unnecessary.

No external principal receives the OCI delete action.

Viewer is an account type, not a project role. It is distinct from Reader:
Readers have read access through a project membership, while a Viewer also has
the global read-only account restrictions described here.

## Installation administration

| Action | Administrator | Project Admin | Regular user | Viewer |
|---|---:|---:|---:|---:|
| Create a project | Yes | No | No | No |
| Delete an empty project | Yes | No | No | No |
| See every project | Yes | Only assigned projects | Only assigned projects | Only assigned projects |
| Create, edit, disable, reactivate, or reset users | Yes | No | No | No |
| Promote an active user to administrator or viewer | Yes | No | No | No |
| Create or disable service accounts | Yes | No | No | No |
| Create, list, or revoke service-account keys | Yes | No | No | No |
| Create, download, or delete a local backup | Yes | No | No | No |
| Run Distribution garbage collection | Yes | No | No | No |
| Manage project memberships | Yes | Yes, in that project | No | No |
| Create, archive, unarchive, or remove logical repositories | Yes | Yes, in that project | No | No |
| Change policies, delete artifacts, or run retention | Yes | Yes, in that project | No | No |

Every signed-in user can see their own profile and change their own password
after providing the current password.

## User lifecycle

Bootstrap creates the first administrator. Every later user begins
as a disabled regular user with a registration link that is shown once. Using
the link sets a password and activates the account.

Only an active administrator can promote an active user to administrator or
viewer. Promoting a user to administrator removes viewer status; administrators
cannot also be viewers. The system does not allow an administrator to disable
their own account or the last active administrator. Disabling a user blocks new
logins and revokes their web sessions.

An administrator can edit a user's email or username at any time, including
their own; edits are validated for uniqueness the same way as user creation.
A disabled user can be reactivated, which restores sign-in ability; reactivation
does not touch web sessions since a disabled account has none. Every edit,
disable, and reactivate action produces an audit event.

Password-reset links are single-use, expire after 30 minutes, and place their
secret in the URL fragment. Creating a new link invalidates unused earlier
links. Completing a reset revokes the user's sessions but never reactivates a
disabled account. A signed-in user cannot use a reset link; they must change
their password in the profile or sign out first.

## Web sessions

After an email-and-password login, Grom creates an opaque server-side session
and an HTTP-only, SameSite cookie. The default lifetime is 24 hours and can be
configured. Signing out removes the session and clears the cookie. Protected
routes restore the session before navigation.

Cookie-authenticated mutations require an allowed Origin. Requests that have a
session cookie but no Origin are rejected. Failed logins are rate-limited by
resolved client address. Passwords, session secrets, and reset secrets are
stored as Argon2id hashes.

## Service accounts and keys

A service account has a display name, unique registry username, optional
description, and disabled state. It gains access only through project
membership. Disabling it blocks the next registry authentication.

Every access key belongs to one service account. Its secret uses the form
grm_<public-id>_<secret>, is shown once, and cannot be retrieved later. An
account can have no more than three active keys; expired or revoked keys no
longer count toward that limit. Revocation and expiry are checked at the next
token exchange, and successful authentication updates last use. The server
counts active keys across the complete history, not only the current page.

## Viewer profile token

A Viewer can keep one active, named registry credential in their profile. It is
shown once and can be revoked by that Viewer. It authenticates as
the user and grants only pull for projects with explicit membership.

That restriction is absolute: the token cannot grant push or delete, even when
the Viewer also has Writer or Admin project membership. Other user types do not
have personal registry tokens.

## Registry authorization

The token flow applies RBAC on every request:

1. The client receives a Bearer challenge from /v2/.
2. It sends a service-account credential, or an allowed viewer profile token, to
   /auth/token.
3. Grom validates the credential, expiry, revocation, and owner.
4. Each requested scope is evaluated separately; the leading slug locates its
   project.
5. Requested actions are intersected with the principal's current role.
6. Grom signs a short-lived JWT containing only the allowed subset.
7. Distribution validates the JWT for the OCI operation.

The JWT lasts five minutes by default and is configurable. Unsupported scopes
are ignored, and mixed requests receive only the allowed subset. There is no
global catalog permission or client-side registry deletion.

A Writer or Admin push can create a missing logical repository when the project
already exists. Pull and Reader requests cannot do that. An archived repository
blocks new push grants while preserving pull access. Project administrators can
unarchive it to restore push grants.

## Protections for sensitive operations

Administrative operations use the current role, never a client-side copy of
permissions. The API verifies that a principal exists before assigning
membership. Project deletion requires no logical repositories. Manifest deletion
and retention execution require a preview, immediate revalidation, policy
checks, and audit history; protected OCI relationships prevent deletion. Blob
collection is separate and reserved for administrators.

Audit events cover authentication, users, service accounts, keys, projects,
memberships, password changes, policies, backups, and destructive operations.
They never include passwords, tokens, keys, or Authorization headers.

## Implementation reference

| Area | Path |
|---|---|
| Identities, sessions, and credentials | backend/internal/identity/ |
| Roles, memberships, and access decisions | backend/internal/projects/ |
| JWT grants and registry gateway | backend/internal/registry/application/token_service.go and backend/internal/registry/infrastructure/distribution/ |
| HTTP routes and application layer | backend/internal/httpapi/ |
| Public contract | backend/api/openapi.yaml |
| Generated frontend types | frontend/src/shared/api/generated/ |

## Keeping this document current

Update this document whenever principals, authentication methods, roles, actions,
permission precedence, protected resources, or credential rules change. Update
docs/architecture.md as well when a change affects context boundaries, runtime
flows, or data ownership.
