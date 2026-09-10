#!/usr/bin/env bash
# Reference script for bind-mounted deployments. Named volumes (the compose
# default) are auto-chowned by Docker and don't need this. Run as root
# (e.g. sudo) against the host paths you bind-mount to /data, /certs, and
# /database-backups on the `grom` service, so the container's grom user
# (uid 100, gid 101) can write to them. backup-agent, distribution-config-init,
# and recovery need no host-side chown: they run as root or read-only.
set -euo pipefail

data_dir="${1:-./data}"
certs_dir="${2:-./certs}"
backups_dir="${3:-./database-backups}"

for dir in "${data_dir}" "${certs_dir}" "${backups_dir}"; do
  mkdir -p "${dir}"
  chown -R 100:101 "${dir}"
  echo "Prepared ${dir} for uid 100 / gid 101"
done
