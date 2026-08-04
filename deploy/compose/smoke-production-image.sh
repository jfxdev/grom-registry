#!/usr/bin/env bash
set -euo pipefail

project_name="grom-production-smoke-${RANDOM}-${RANDOM}"
image_tag="grom-registry:smoke-${project_name}"
compose_file="deploy/compose/docker-compose.yml"
bootstrap_password="smoke-production-password"

cleanup() {
  docker compose --project-name "${project_name}" -f "${compose_file}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker image rm --force "${image_tag}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker build --build-arg GROM_VERSION=smoke -t "${image_tag}" .

image_user="$(docker image inspect --format '{{.Config.User}}' "${image_tag}")"
if [[ "${image_user}" != "grom" ]]; then
  echo "production image must run as grom; got ${image_user:-<empty>}" >&2
  exit 1
fi

GROM_IMAGE="${image_tag}" \
GROM_HTTP_PORT=0 \
GROM_BOOTSTRAP_ADMIN_PASSWORD="${bootstrap_password}" \
docker compose --project-name "${project_name}" -f "${compose_file}" up --detach --no-build

endpoint=""
for _ in $(seq 1 60); do
  endpoint="$(docker compose --project-name "${project_name}" -f "${compose_file}" port grom 8080 2>/dev/null || true)"
  if [[ -n "${endpoint}" ]] && curl --fail --silent --show-error "http://${endpoint}/readyz" >/dev/null; then
    break
  fi
  sleep 1
done

if [[ -z "${endpoint}" ]] || ! curl --fail --silent --show-error "http://${endpoint}/readyz" >/dev/null; then
  echo "Grom did not become ready" >&2
  docker compose --project-name "${project_name}" -f "${compose_file}" logs >&2 || true
  exit 1
fi

curl --fail --silent --show-error "http://${endpoint}/healthz" >/dev/null
curl --fail --silent --show-error "http://${endpoint}/api/docs" >/dev/null

registry_headers="$(curl --silent --show-error --dump-header - --output /dev/null "http://${endpoint}/v2/")"
if ! grep --ignore-case --quiet '^www-authenticate: Bearer ' <<<"${registry_headers}"; then
  echo "registry endpoint did not return a bearer challenge" >&2
  exit 1
fi
