# Release operations

This guide covers the supported first-release installation: one active Grom
instance, SQLite, local blob storage, and private CNCF Distribution.

## Install a published release

Use the immutable image reference recorded in the GitHub Release asset
`grom-<version>.image-digest.txt`. Set `GROM_IMAGE` to that value in the
installation environment; Grom, the backup agent, and recovery UI must use the
same digest.

For internet-facing installations, use the `strict` deployment profile,
an HTTPS `GROM_PUBLIC_URL`, and `GROM_SECURE_COOKIES=true`. Configure
`GROM_TRUSTED_PROXIES` only for immediate proxy peers. Start the supplied
Compose deployment and wait for `/readyz`; verify `/api/docs` and the `/v2/`
Bearer challenge through the public Grom address.

## Upgrade

1. Create and download a verified recovery point in **Backup & recovery**.
2. Record the current image digest and update `GROM_IMAGE` to the new release
   digest.
3. Start the updated deployment. Grom applies reviewed migrations before it
   becomes ready.
4. Verify `/readyz`, administrator sign-in, project browsing, and a pull plus
   push through the public registry endpoint.

The first release supports forward upgrades only. Do not attempt a database
downgrade by replacing only the image.

## Rollback

If the new image fails before migrations complete, return to the recorded
previous digest and investigate readiness. If migrations have completed or data
integrity is uncertain, restore the verified recovery bundle through the
loopback-only recovery UI; that is the supported data rollback procedure.

## Registry signing keys

The MVP does not provide seamless signing-key rotation. Replacing signing
material invalidates existing registry JWTs, which may interrupt registry
operations until clients obtain new tokens (at most the configured registry JWT
TTL). Preserve the signing material in backups. A supported operator-facing
key-replacement workflow is deferred with seamless rotation.

## Supported matrix

The first release supports SQLite, local blob storage, one active Grom
instance, and Docker image push/pull. PostgreSQL, S3-compatible storage, ORAS,
and generic OCI compatibility remain unadvertised until their dedicated gates
pass.
