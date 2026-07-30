#!/bin/sh
set -eu

case "${1:-}" in
  backup-agent)
    shift
    exec /usr/local/bin/grom-backup agent "$@"
    ;;
  recovery)
    shift
    exec /usr/local/bin/grom-backup recovery "$@"
    ;;
  backup)
    shift
    exec /usr/local/bin/grom-backup "$@"
    ;;
  config-init)
    shift
    if [ ! -f /distribution-config/config.yml ]; then
      cp /opt/grom/distribution/config.yml /distribution-config/config.yml
      chmod 0600 /distribution-config/config.yml
    fi
    ;;
  *)
    exec /usr/local/bin/grom "$@"
    ;;
esac
