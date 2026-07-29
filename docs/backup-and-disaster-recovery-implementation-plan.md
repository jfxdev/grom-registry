# Backup and disaster recovery implementation plan

Status: implemented for the default SQLite and local-volume deployment.

This document records the implementation boundary and acceptance criteria. The
operator procedure lives in
[`backup-and-disaster-recovery.md`](backup-and-disaster-recovery.md).

## Product outcome

Backup and recovery are product capabilities of the self-contained Grom image.
An installation administrator creates and downloads recovery points from the
management UI. After a total loss, the same image starts a standalone recovery
UI that accepts a recovery bundle and restores empty Compose volumes.

The supported product flow never requires a source checkout, `make`, a shell
inside the application container, or access to the Docker socket.

The implementation covers one installation using:

- SQLite;
- local Distribution storage;
- Grom signing certificates;
- the Distribution configuration embedded in the release image and persisted
  in its own named volume;
- Docker Compose named volumes.

PostgreSQL, S3-backed Distribution, scheduled backups, and point-in-time
recovery remain outside this profile.

## Runtime topology

The production image contains three entry points:

1. `grom`, the normal control plane and web application;
2. `backup-agent`, an isolated helper that creates and streams recovery points;
3. `recovery`, a standalone local web application for disaster recovery.

The default Compose deployment adds:

- a `backup-agent` container using the exact same Grom image;
- read-only mounts for the Grom data, registry data, signing certificates, and
  Distribution configuration;
- a writable backup volume;
- a small runtime volume containing only a Unix socket shared with Grom;
- no network for the backup agent and no Docker socket anywhere;
- a `distribution-config-init` one-shot service that copies the configuration
  embedded in the image into its named volume on first installation.

The recovery service is behind an explicit Compose profile and binds to
loopback by default. It mounts the empty target volumes and the backup volume,
but it does not start the normal Grom or Distribution services.

## Creating a recovery point

The installation administrator opens **Backup & recovery** and confirms
creation. The API returns an operation immediately and the UI polls its status.

The backend then:

1. enters maintenance mode;
2. rejects new management mutations, registry token exchanges, and `/v2`
   traffic with `503 Service Unavailable`;
3. waits for already-running writes and registry requests to drain;
4. checkpoints and truncates the SQLite WAL;
5. asks the isolated backup agent to snapshot all required volumes;
6. validates the completed recovery point;
7. records a sanitized audit event;
8. always releases maintenance mode, including on failure.

Distribution upload purging is disabled in this deployment so it cannot write
to registry storage while Grom has quiesced public registry traffic.

The UI shows the current operation and complete local recovery points in
cursor-stable pages of five. A complete point can be downloaded as one portable
tar bundle for encrypted off-host retention. An administrator may permanently
delete a local point only after typing `DELETE`; downloaded bundles are not
affected. Partial sets are never listed, downloadable, or deletable.

## Recovery-set format

Each recovery point contains:

```text
grom-backup-<timestamp>-<backup-id>/
├── manifest.json
├── checksums.sha256
├── COMPLETE
├── grom-data.tar
├── registry-data.tar
├── signing-certs.tar
└── distribution-config.tar
```

Format version 2 identifies a quiesced backup created through the product UI.
Format version 1 remains readable for recovery points created by the earlier
offline utility. The manifest records the backup ID, creation time, source Grom
version, consistency method, included components, and their sizes and SHA-256
digests. `COMPLETE` is written only after validation and atomic publication.

Archive extraction rejects absolute paths, traversal, links, special files,
unknown payloads, duplicates, and checksum mismatches. Backup and recovery
errors never expose credential contents.

Deletion resolves only a validated backup UUID, confirms that its complete
directory is a direct child of the backup root, atomically renames it to a
private tombstone, and removes only that tombstone. The agent serializes create
and delete operations and holds downloads open against concurrent deletion.

## Standalone recovery

After loss of the original volumes, the operator starts the `recovery` profile
from the same deployment bundle and opens its loopback-only UI. The UI:

1. accepts a downloaded recovery bundle;
2. validates and imports it into the backup volume;
3. displays compatible complete recovery points;
4. requires the operator to type `RESTORE`;
5. restores only into empty target volumes;
6. reports completion before the operator starts the normal stack again.

The recovery server uses a random, process-local request token for mutations,
permits one restore at a time, and never exposes the backup volume through a
general file browser.

Restore validates the complete set before extraction, stages every component,
refuses non-empty targets, writes a restore marker, and publishes the staged
data to the target volumes. On the first normal boot after restore, Grom
invalidates restored web sessions and password-reset capabilities and records
an idempotent `platform.restore_completed` audit event. Service-account access
keys retain their state at the selected recovery point.

## Retention and off-host storage

The local backup volume is a staging area, not the disaster-recovery boundary.
Operators download completed bundles and place them in an encrypted,
authenticated, off-host repository such as Restic. Recommended starting
retention is seven daily, four weekly, and policy-defined monthly recovery
points. Grom does not store remote-backup credentials or run a background
scheduler in the current MVP.

## Developer and compatibility tools

`grom-backup create`, `inspect`, and `restore`, plus the corresponding `make`
targets, remain low-level development and compatibility tools. They are useful
for automated tests and unusual offline maintenance, but are not the product
interface and are not required in an installed deployment.

## Verification

Unit coverage includes deterministic manifests, checksum validation, safe tar
handling, atomic publication, maintenance draining, backup-agent communication,
portable bundle import, recovery HTTP authorization and validation, CLI command
parsing, empty-target enforcement, restore markers, failure-state reporting,
and administrative backup endpoint behavior.

The mandatory real-Docker recovery journey:

1. starts the default stack;
2. creates identities, authorization, and OCI content;
3. creates and downloads a recovery point through the public API used by the
   management UI;
4. destroys the original named volumes;
5. starts the recovery UI from the same image;
6. uploads, validates, and restores the bundle;
7. starts the normal stack and verifies authentication, browsing, pull, push,
   signing-key continuity, and restored-session invalidation;
8. corrupts a bundle and proves that recovery rejects it.

The `Backup Restore E2E` GitHub check runs this journey without uploading any
database, certificate, token, blob, or recovery bundle as an artifact.

## Completion checklist

- [x] Backup and recovery are reachable through non-technical web interfaces.
- [x] The installed product does not depend on `make` or a source checkout.
- [x] Backup uses a bounded, enforced quiescence window.
- [x] Maintenance mode is released after both success and failure.
- [x] The helper has no network or Docker-socket access and reads source
      volumes read-only.
- [x] All recovery-critical volumes are captured as one versioned unit.
- [x] Partial, corrupt, unsafe, or incompatible sets are rejected.
- [x] Recovery points are paginated five at a time and local deletion requires
      typed confirmation.
- [x] Creation, download, and deletion cannot race over the same recovery set.
- [x] Recovery accepts only empty targets and is loopback-only by default.
- [x] Restored ephemeral web credentials are invalidated.
- [x] A real-Docker volume-loss journey proves pull and push after recovery.
- [x] Backup payloads and secrets are never CI artifacts.

## Documentation maintenance

This feature adds platform-operational records rather than business-domain
entities. `docs/code-map.md`, `docs/domain-model.md`,
`docs/architecture-and-mvp.md`, `docs/product-features.md`, `README.md`, and
`AGENTS.md` must remain aligned whenever the backup format, runtime topology,
public API, UI, or recovery contract changes.
