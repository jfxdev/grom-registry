#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd "${script_dir}/../.." && pwd -P)"
compose_file="${repository_root}/deploy/compose/docker-compose.yml"
backup_overlay="${repository_root}/deploy/backup/docker-compose.backup.yml"
env_file="${repository_root}/.env"

fail() {
  printf 'backup: %s\n' "$1" >&2
  exit 1
}

absolute_existing_directory() {
  local requested="$1"
  case "${requested}" in
    /*) ;;
    *) fail "path must be absolute" ;;
  esac
  [ -d "${requested}" ] || fail "directory does not exist: ${requested}"
  (cd "${requested}" && pwd -P)
}

absolute_existing_backup() {
  local requested="$1"
  case "${requested}" in
    /*) ;;
    *) fail "path must be absolute" ;;
  esac
  local parent base
  parent="$(dirname "${requested}")"
  base="$(basename "${requested}")"
  parent="$(absolute_existing_directory "${parent}")"
  [ -d "${parent}/${base}" ] || fail "backup set does not exist"
  printf '%s/%s\n' "${parent}" "${base}"
}

[ -f "${env_file}" ] || fail "missing .env; copy .env.example and configure the installation"
command -v docker >/dev/null 2>&1 || fail "Docker CLI is required"
docker info >/dev/null 2>&1 || fail "Docker daemon is not accessible"
docker compose version >/dev/null 2>&1 || fail "Docker Compose is required"

compose=(
  docker compose
  --ansi never
  --project-directory "${repository_root}"
  --env-file "${env_file}"
  --file "${compose_file}"
  --file "${backup_overlay}"
)

action="${1:-}"
case "${action}" in
  create)
    [ "$#" -eq 2 ] || fail "usage: backup-compose.sh create /absolute/backup/directory"
    case "$2" in
      /*) ;;
      *) fail "path must be absolute" ;;
    esac
    if [ ! -e "$2" ]; then
      mkdir -p "$2"
      chmod 0700 "$2"
    fi
    BACKUP_MOUNT_ROOT="$(absolute_existing_directory "$2")"
    BACKUP_SET_NAME="pending-backup-set"
    export BACKUP_MOUNT_ROOT BACKUP_SET_NAME

    "${compose[@]}" build backup-estimate backup-create backup-inspect
    grom_id="$("${compose[@]}" ps -q grom)"
    distribution_id="$("${compose[@]}" ps -q distribution)"
    [ -n "${grom_id}" ] && [ -n "${distribution_id}" ] ||
      fail "the exact Grom and Distribution services must already exist"
    [ "$(printf '%s\n' "${grom_id}" | wc -l | tr -d ' ')" = "1" ] ||
      fail "exactly one Grom container must be in scope"
    [ "$(printf '%s\n' "${distribution_id}" | wc -l | tr -d ' ')" = "1" ] ||
      fail "exactly one Distribution container must be in scope"
    source_mounts="$(docker inspect --format '{{range .Mounts}}{{if eq .Type "volume"}}{{println .Source}}{{end}}{{end}}' "${grom_id}" "${distribution_id}")"
    while IFS= read -r source_mount; do
      [ -z "${source_mount}" ] && continue
      case "${BACKUP_MOUNT_ROOT}/" in
        "${source_mount}/"*) fail "backup destination resolves inside an application volume" ;;
      esac
    done <<< "${source_mounts}"

    estimate_output="$("${compose[@]}" run --rm --no-deps backup-estimate)"
    printf '%s\n' "${estimate_output}"
    estimated_bytes="$(printf '%s\n' "${estimate_output}" | sed -n 's/^estimated_total_bytes=//p' | tail -n 1)"
    available_kib="$(df -Pk "${BACKUP_MOUNT_ROOT}" | tail -n 1 | awk '{print $4}')"
    case "${estimated_bytes}" in
      ''|*[!0-9]*) fail "could not determine required and available backup capacity" ;;
    esac
    case "${available_kib}" in
      ''|*[!0-9]*) fail "could not determine required and available backup capacity" ;;
    esac
    if [ "${estimated_bytes}" -gt $((available_kib * 1024)) ]; then
      fail "backup destination does not have enough available capacity"
    fi

    stopped=0
    restart_stack() {
      "${compose[@]}" up --detach --no-build grom distribution
      local attempt health
      attempt=0
      while [ "${attempt}" -lt 60 ]; do
        grom_id="$("${compose[@]}" ps -q grom)"
        health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${grom_id}" 2>/dev/null || true)"
        if [ "${health}" = "healthy" ]; then
          return 0
        fi
        attempt=$((attempt + 1))
        sleep 2
      done
      return 1
    }
    cleanup() {
      local status=$?
      trap - EXIT
      if [ "${stopped}" -eq 1 ]; then
        if ! restart_stack; then
          printf 'backup: failed to return Grom to readiness\n' >&2
          status=1
        fi
      fi
      exit "${status}"
    }
    trap cleanup EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM

    stopped=1
    "${compose[@]}" stop grom
    "${compose[@]}" stop distribution
    create_output="$("${compose[@]}" run --rm --no-deps backup-create)"
    printf '%s\n' "${create_output}"
    backup_path="$(printf '%s\n' "${create_output}" | sed -n 's/^backup_path=//p' | tail -n 1)"
    [ -n "${backup_path}" ] || fail "backup tool did not report the completed path"
    BACKUP_SET_NAME="$(basename "${backup_path}")"
    export BACKUP_SET_NAME

    restart_stack
    stopped=0
    "${compose[@]}" run --rm --no-deps backup-inspect
    printf '%s\n' "The local backup contains credentials, signing material, and private image content."
    printf 'Retain it off-host with authenticated encryption; see docs/backup-and-disaster-recovery.md.\n'
    ;;
  inspect)
    [ "$#" -eq 2 ] || fail "usage: backup-compose.sh inspect /absolute/path/to/backup"
    backup_path="$(absolute_existing_backup "$2")"
    BACKUP_MOUNT_ROOT="$(dirname "${backup_path}")"
    BACKUP_SET_NAME="$(basename "${backup_path}")"
    export BACKUP_MOUNT_ROOT BACKUP_SET_NAME
    "${compose[@]}" build backup-inspect
    "${compose[@]}" run --rm --no-deps backup-inspect
    ;;
  *)
    fail "usage: backup-compose.sh create|inspect PATH"
    ;;
esac
