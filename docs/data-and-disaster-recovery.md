# Data, storage, and disaster recovery

## Purpose

This document explains how Grom protects installation data and why it reports
two different Distribution storage measures:

- **accounted usage**: logical OCI content attributable to a repository or
  project;
- **physical usage**: the sum of regular-file bytes in local Distribution
  storage.

They answer different questions. Neither one is a quota or a measure of free
disk capacity.

The supported deployment is one active Grom instance with SQLite or PostgreSQL
and local Distribution storage. Restoring over existing data, downgrading, S3
storage, and selective recovery are not supported.

## Durable data

| Data | Default volume | Role in recovery |
|---|---|---|
| Grom data | grom-data | SQLite database or application data, including the restore marker. |
| PostgreSQL | postgres-data | Database data when the PostgreSQL overlay is enabled. |
| Signing material | signing-certs | Registry-token key, certificate, and JWKS. |
| OCI content | registry-data | Manifests, tags, configurations, layers, and blobs. |
| Distribution configuration | distribution-config | The effective configuration that belongs with restored content. |
| Local recovery points | grom-backups | Sets ready to download or restore; they are not an off-host copy. |

The backup agent's runtime socket is not recovery data.

## Creating a recovery point

Only an administrator can start the operation through the backup
interface. Grom serializes creation and deletion of local points. Listings use a
stable cursor and show exactly five recovery points per page.

~~~mermaid
sequenceDiagram
  participant Admin as "Administrator"
  participant Grom
  participant Maint as "Maintenance"
  participant DB as "Database"
  participant Agent as "Isolated agent"
  participant Store as "Backup storage"

  Admin->>Grom: Create recovery point
  Grom->>Maint: Block writes and wait for drain
  Grom->>DB: SQLite checkpoint or PostgreSQL dump
  Grom->>Agent: Create set over Unix socket
  Agent->>Store: Archive read-only components
  Agent->>Agent: Verify and publish atomically
  Agent-->>Grom: Recovery point created
  Grom->>Maint: Resume operations
~~~

During maintenance, Grom drains writes and blocks management mutations, token
exchange, and registry traffic that could change durable state. The SQLite
checkpoint is non-blocking, so WAL frames retained by an already-running read
are included with the database in Grom data. The recovery point therefore
remains consistent without interrupting a read-only request. PostgreSQL uses
the internally consistent snapshot from `pg_dump` and does not require the
application database role to hold the administrative `pg_checkpoint` privilege.
Distribution's upload purge remains disabled during that window.

The agent uses the same Grom image, has no network or published ports, reads
source volumes only, and talks to Grom through its dedicated Unix socket. For
SQLite, the set includes Grom data, signing material, Distribution data, and
Distribution configuration. For PostgreSQL, it also includes a validated logical
database dump.

Every set has a manifest containing its ID, Grom version, deployment profile,
database, storage type, components, sizes, and SHA-256 checksums. It is
published only after it is complete and verified. Downloading creates a portable
bundle. Deleting the local copy requires confirmation, accepts only a valid
backup UUID, and never removes an already downloaded bundle.

Bundles include private OCI content, signing material, and credential hashes.
Keep them off-host with authenticated encryption and retain a verified external
copy.

## Recovering from volume loss

Recovery uses the recovery mode of the same image. It does not need a checkout,
make, a shell in the container, or a Docker socket. The UI is loopback-only and
the normal stack must be stopped.

1. Stop Grom, Distribution, and the backup agent.
2. Preserve damaged volumes separately if investigation is needed.
3. Create empty replacement volumes for Grom data, signing material,
   Distribution data, and configuration; add an empty PostgreSQL target when
   restoring a PostgreSQL backup.
4. Start the image in recovery mode on loopback.
5. Upload or choose the bundle and confirm by typing RESTORE.
6. After success, stop recovery and start the normal stack with a compatible
   image version.

The recovery UI validates a bundle before it can be selected. It rejects
incompatible formats or versions, missing files, checksum mismatches, unsafe
paths, links and special files, undeclared components, and non-empty targets.
Restoration is staged: targets receive data only after the full validation
succeeds.

On the first normal boot, Grom applies compatible migrations, invalidates
restored web sessions and reset links, records the recovery in audit history,
and consumes the restore marker. Service-account keys return to the recovery
point's state. Revocations after that point are not included and should be
rotated after an incident.

## Accounted usage by repository and project

Grom maintains a logical view of observed OCI content. Inventory records
descriptor facts by digest, including media type and size, plus the edges that
connect manifests to reachable descriptors. Descriptor size is canonical: a
later observation with a different size is rejected.

After a successful observation or reconciliation, Grom calculates snapshots for
the repository and project. The sum includes descriptors reachable only from
manifests in active or untagged state. Missing and deleted history does not
contribute.

The calculation deduplicates by digest inside its scope:

| Situation | Effect on accounted usage |
|---|---|
| The same image has two tags in one repository | Count it once. |
| A layer is shared by repositories in one project | Count it once for the project. |
| The same digest is referenced by different projects | Count it in each project. |
| A manifest is deleted but blobs still exist | Stop counting it once it is no longer live in inventory. |
| Inventory has no snapshot yet | Report pending, never zero. |

Every response includes a status, bytes when available, and the reconciliation
time:

| Status | Meaning |
|---|---|
| pending | There is not enough data for a snapshot yet. |
| ready | The snapshot was calculated from available inventory. |
| stale | The snapshot is out of date and needs observation or reconciliation. |
| unavailable | Grom could not read the measure. |

Snapshots are stored, and only a newer version can replace an older one. They
can be rebuilt entirely from Registry facts without reading the Distribution
volume. Accounted usage is therefore a product-level, scoped view, not a
filesystem measurement.

## Physical Distribution usage

The installation-settings page is limited to administrators. It
queries the private maintenance agent over a Unix socket. That agent walks
Distribution's local data root and adds the size of every regular file. The
result appears as usedBytes in installation status.

~~~mermaid
flowchart LR
  Settings["Installation settings"] --> Grom
  Grom --> Socket["Unix socket"]
  Socket --> Agent["Maintenance agent"]
  Agent --> Scan["Walk registry-data"]
  Scan --> Physical["usedBytes: regular-file bytes"]
~~~

This is a measure of files under the local registry root. It is not total or
free capacity, filesystem block allocation, directory size, volume metadata, or
host quota. Continue to monitor operational capacity in the deployment
environment.

## Deletion, garbage collection, and reclaimed space

Deleting a manifest removes its OCI reference and updates inventory. It does not
remove blobs immediately, so accounted usage can go down before physical usage.

Garbage collection is separate and reserved for administrators:

1. Grom enters global maintenance and blocks mutations and registry traffic.
2. The maintenance agent measures physical bytes before collection.
3. It stops Distribution and runs Distribution's own collector with exclusive
   access to local storage.
4. It measures physical bytes afterward, starts a fresh Distribution process,
   and waits for readiness.
5. Grom audits the start, completion, or failure and records reclaimed bytes.

Reclaimed bytes are the maximum of bytes before minus bytes after and zero. Grom
never runs the collector alongside Distribution, does not use Docker to control
it, and restarts Distribution to clear blob-existence caches.

## Reading the measures

| Question | Appropriate measure |
|---|---|
| How much live OCI content belongs to this project? | Project accounted usage. |
| How do shared layers contribute within a project? | Accounted usage, once per digest. |
| How much local file space does Distribution occupy? | usedBytes in Installation settings. |
| How much did the last collection free? | Collection reclaimedBytes. |
| How much free disk or volume capacity remains? | Host/provider monitoring; Grom does not calculate it. |

Do not sum projects to estimate physical usage: one digest can count in more
than one project. Do not use usedBytes as a project quota: it has no project
attribution and no per-scope deduplication.

## Implementation reference

| Component | Path |
|---|---|
| Backup and catalog | backend/internal/platform/backup/ |
| Recovery | backend/cmd/grom-backup/ and backend/internal/platform/backup/recovery.go |
| Maintenance drain | backend/internal/platform/maintenance/ |
| Physical usage and collection | backend/internal/platform/registrymaintenance/ |
| Accounted-usage snapshots | backend/internal/registry/infrastructure/persistence/bun/storage.go |
| Shared usage type | backend/internal/foundation/storage_usage.go |
| Physical status | GET /api/v1/settings/status |
| Blob collection | POST /api/v1/garbage-collections |

## Keeping this document current

Update this document when backup, bundle format, recovery, support boundaries,
accounted-usage logic, physical measurement, or garbage collection changes.
Update docs/architecture.md as well when the topology or platform boundaries
change.
