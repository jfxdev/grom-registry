# Release operations

This guide covers the supported installation profile: one active Grom instance,
SQLite (the default) or PostgreSQL, local blob storage, and private CNCF
Distribution. PostgreSQL support does not permit multiple active Grom
instances.

## Install a published release

Use the immutable image reference recorded in the GitHub Release asset
`grom-<version>.image-digest.txt`. Set `GROM_IMAGE` to that value in the
installation environment; Grom, the backup agent, and recovery UI must use the
same digest.

For internet-facing installations, use the `strict` deployment profile,
an HTTPS `GROM_PUBLIC_URL`, and `GROM_SECURE_COOKIES=true`. Configure
`GROM_TRUSTED_PROXIES` only for immediate proxy peers. Start the supplied
Compose deployment with `docker compose pull` followed by
`docker compose up -d --no-build`, then wait for `/readyz`; verify `/api/docs`
and the `/v2/` Bearer challenge through the public Grom address. Release
installations must not use `make compose-up` or `docker compose up --build`,
which could replace the selected published digest with a local build.

## Upgrade

1. Create and download a verified recovery point in **Backup & recovery**.
2. Record the current image digest and update `GROM_IMAGE` to the new release
   digest.
3. Start the updated deployment. Grom applies reviewed migrations before it
   becomes ready.
4. Verify `/readyz`, administrator sign-in, project browsing, and a pull plus
   push through the public registry endpoint.

Forward upgrades from `v0.0.1` are accepted for its SQLite/local-storage
matrix. PostgreSQL support begins with the first stable release that advertises
it; a release must not imply a PostgreSQL upgrade path from the SQLite-only
`v0.0.1` baseline.
The [release-upgrade acceptance journey](https://github.com/jfxdev/grom-registry/pull/23)
upgrades the published `v0.0.1` baseline to a locally built candidate while
preserving SQLite and local-registry volumes; it verifies administrator access,
projects, Writer credentials, inventory, blobs, and a subsequent restart. Each
future stable baseline must pass the same journey before its upgrade path is
advertised. Do not attempt a database downgrade by replacing only the image.

## Rollback

If the new image fails before migrations complete, return to the recorded
previous digest and investigate readiness. If migrations have completed or data
integrity is uncertain, set `GROM_IMAGE` to the recorded previous digest or the
recovery bundle's compatible image version, then restore the verified recovery
bundle through the loopback-only recovery UI. Start the normal stack with that
same matching image before attempting another upgrade; this is the supported
data rollback procedure.

## Registry signing keys

Signing-key replacement and seamless rotation are unsupported in the MVP. Do
not replace signing material manually: doing so would invalidate existing
registry JWTs and may interrupt registry operations until clients obtain new
tokens (at most the configured registry JWT TTL). Preserve the signing material
in backups; a supported operator-facing replacement workflow is post-MVP.

## Supported matrix

The supported matrix is SQLite (default) or PostgreSQL with local blob storage,
one active Grom instance, and Docker image push/pull. The PostgreSQL test and
backup/recovery gates passed in [PR #33](https://github.com/jfxdev/grom-registry/pull/33).
S3-compatible storage, ORAS, and generic OCI compatibility remain unadvertised
until their dedicated gates pass.

Repository and project pages report logical accounted registry usage. It
deduplicates live OCI descriptors within each scope and is not filesystem
capacity or reclaimable GC space; Settings and garbage collection remain the
source for physical Distribution-storage measurements.
