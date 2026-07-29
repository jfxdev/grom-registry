#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd "${script_dir}/../.." && pwd -P)"
compose_file="${repository_root}/deploy/compose/docker-compose.yml"
backup_overlay="${repository_root}/deploy/backup/docker-compose.backup.yml"
env_file="${repository_root}/.env"

fail() {
  printf 'restore: %s\n' "$1" >&2
  exit 1
}

[ "$#" -eq 1 ] || fail "usage: restore-compose.sh /absolute/path/to/backup"
case "$1" in
  /*) ;;
  *) fail "backup path must be absolute" ;;
esac
[ -f "${env_file}" ] || fail "missing .env; copy .env.example and configure the installation"
command -v docker >/dev/null 2>&1 || fail "Docker CLI is required"
docker info >/dev/null 2>&1 || fail "Docker daemon is not accessible"
docker compose version >/dev/null 2>&1 || fail "Docker Compose is required"

backup_parent="$(dirname "$1")"
backup_name="$(basename "$1")"
[ -d "${backup_parent}" ] || fail "backup parent directory does not exist"
backup_parent="$(cd "${backup_parent}" && pwd -P)"
backup_path="${backup_parent}/${backup_name}"
[ -d "${backup_path}" ] || fail "backup set does not exist"

BACKUP_MOUNT_ROOT="${backup_parent}"
BACKUP_SET_NAME="${backup_name}"
export BACKUP_MOUNT_ROOT BACKUP_SET_NAME

compose=(
  docker compose
  --ansi never
  --project-directory "${repository_root}"
  --env-file "${env_file}"
  --file "${compose_file}"
  --file "${backup_overlay}"
)

running="$("${compose[@]}" ps --services --status running)"
if printf '%s\n' "${running}" | grep -Eq '^(grom|distribution)$'; then
  fail "Grom and Distribution must be stopped before restore"
fi

"${compose[@]}" build backup-inspect backup-restore
"${compose[@]}" run --rm --no-deps backup-inspect

cleanup() {
  local status=$?
  trap - EXIT
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"${compose[@]}" run --rm --no-deps backup-restore
printf 'Restore completed into empty Compose volumes. Services remain stopped.\n'
printf 'Start only after reviewing the recovered configuration: make compose-up\n'
