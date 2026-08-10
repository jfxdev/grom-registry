# Backup and disaster recovery

Grom provides backup and recovery interfaces inside the published Docker image.
Operators do not need a source checkout, `make`, access to the Docker socket, or
shell access inside the running Grom container.

The supported profile is one active Grom installation using SQLite or PostgreSQL, local
Distribution filesystem storage, and the shipped container topology.
S3 storage, selective restore, downgrade, and restore over existing
data are not supported by this workflow.

## Create and download a recovery point

Sign in as an installation administrator and open **Backup & recovery**.
Selecting **Create backup** starts an asynchronous operation:

1. Grom temporarily rejects new management mutations, registry-token exchanges,
   pushes, and other `/v2` traffic that could change durable state.
2. Existing writes drain.
3. SQLite is checkpointed; PostgreSQL is checkpointed and exported as a
   validated logical dump.
4. The internal backup agent archives the Grom data, database payload, signing material, local
   Distribution storage, and effective Distribution configuration.
5. The complete set is checksummed, verified, and published atomically.
6. Maintenance ends and normal writes resume.

Read-only management pages remain available, while OCI pulls and pushes are
paused. The Distribution upload-purge
background task is disabled in this profile so Distribution cannot change its
filesystem behind the quiesced public gateway.

The backup agent uses the same Grom image. It has no network, no published
ports, and no Docker socket. Source volumes are mounted read-only. A private
Unix socket shared only with Grom accepts the narrow create, list, and download
protocol.

Completed recovery points appear five at a time in the UI. Download the `.tar`
bundle and move it to encrypted off-host storage. **Delete** permanently
removes only the selected local snapshot after the administrator types
`DELETE`; previously downloaded bundles remain valid and are not removed. Do
not delete the last known-good off-host recovery point until a newer bundle has
passed a restore drill.

A bundle contains password and API-token hashes, signing keys, metadata, audit
history, and all private image content. Local checksums detect corruption but
do not authenticate an attacker-controlled copy.

## Restore with the image recovery UI

Recovery is intentionally separate from the normal application because the
normal UI may be unavailable after volume loss.

1. Stop Grom, Distribution, and the backup agent.
2. Preserve damaged volumes separately when investigation may be required.
3. Create empty replacement Grom-data, signing, registry-data, and Distribution
   configuration volumes.
4. Start the same image with the `recovery` command, mounting the backup
   location and all empty targets.
5. Publish its port on loopback only and open the recovery UI.
6. Upload the downloaded `.tar` bundle or select a verified bundle already in
   the backup volume.
7. Type `RESTORE` and start recovery.
8. After success, stop the recovery container before starting the normal stack.

With the shipped Compose deployment, the isolated recovery UI is:

```text
docker compose --env-file .env -f docker-compose.yml --profile recovery up recovery
```

It listens on `http://127.0.0.1:8081` by default. A direct image deployment can
use the equivalent `recovery` image command with the five explicit volume
mounts. Never expose the recovery port to a public or untrusted network.

The UI verifies the bundle before extraction, refuses an incompatible image
version, stages all components, checks `grom.db` and the complete signing set,
and refuses every non-empty target. It never starts the normal services.

On the first normal boot after restore, Grom:

1. applies supported forward migrations;
2. invalidates restored browser sessions and password-reset links;
3. records one idempotent `platform.restore_completed` audit event;
4. consumes the restore marker;
5. becomes ready.

Service-account access keys return to their state at the selected recovery
point. Revocations made after that point are outside the RPO. Rotate credentials
after a suspected security incident.

## Persistent topology

The shipped deployment uses these durable volumes:

| Volume | Purpose |
|---|---|
| `grom-data` | SQLite database or application-owned restore-marker data |
| `postgres-data` | PostgreSQL database data when the PostgreSQL Compose overlay is enabled |
| `signing-certs` | Registry JWT private key, certificate, and JWKS |
| `registry-data` | OCI manifests, configurations, layers, and tags |
| `distribution-config` | Effective Distribution configuration |
| `grom-backups` | Locally retained recovery sets awaiting download or restore |

`backup-agent-run` contains only the private runtime socket and is not a backup
target.

The downloaded bundle is the portable recovery artifact. Do not rely only on
`grom-backups`: a host failure or `docker compose down --volumes` can remove it
with the application volumes.

## Encrypted off-host retention

Store downloads in an authenticated, encrypted backup repository outside the
Grom host and preferably outside its failure domain. Restic is one suitable
operator tool:

```text
restic -r <off-host-repository> backup /srv/backups/grom/grom-backup-....tar
restic -r <off-host-repository> check
restic -r <off-host-repository> forget --keep-daily 7 --keep-weekly 4 --keep-monthly 12 --prune
```

Keep at least one previous known-good recovery point and perform an isolated
restore drill at least quarterly. Separately retain the deployment environment,
image digest, ingress/TLS configuration, public URL, secure-cookie setting,
trusted proxies, and registry HTTP secret in encrypted storage.

## Compatibility and troubleshooting

- Format version 1 represents the original cold/offline sets. Format version 2
  represents the integrated quiesced workflow. Recovery accepts both.
- Missing or non-empty `COMPLETE`, unknown files, checksum differences, unsafe
  paths, symlinks, special files, or incompatible versions reject the bundle.
- A failed backup leaves the previous recovery points unchanged and always
  releases maintenance mode.
- A failed restore must not be followed by normal startup. Remove only the
  failed replacement volumes and retry with a verified bundle.
- The development-only `make backup`, `make backup-inspect`, and `make restore`
  wrappers remain for low-level testing and diagnostics. They are not the
  end-user product interface.
- `make test-backup-restore-e2e` validates the UI/API backup, portable download,
  original-volume destruction, recovery UI upload, empty-volume restore,
  restored sign-in and key identity, old-image pull, new push, and corruption
  rejection in an isolated Compose project.
