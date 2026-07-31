#!/usr/bin/env bash
set -euo pipefail

web_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workspace_root="$(cd "$web_root/.." && pwd)"
run_root="$(mktemp -d "${TMPDIR:-/tmp}/ooshdaq-browser-e2e.XXXXXX")"
server_pid=""

cleanup() {
  if [[ -n "$server_pid" ]]; then
    kill -TERM "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  case "$run_root" in
    */ooshdaq-browser-e2e.*)
      rm -rf -- "$run_root"
      ;;
  esac
}
trap cleanup EXIT INT TERM

mkdir -p "$run_root/data" "$run_root/backups" "$web_root/.cache/go-build"

export GOCACHE="$web_root/.cache/go-build"
export GOTOOLCHAIN="local"
export OOSHDAQ_ADDR="${OOSHDAQ_E2E_ADDR:-127.0.0.1:18080}"
export OOSHDAQ_AUTH_MODE="mock"
export OOSHDAQ_DATA_PATH="$run_root/data/ooshdaq.sqlite"
export OOSHDAQ_BACKUP_DIR="$run_root/backups"
export OOSHDAQ_BACKUP_INTERVAL="744h"
export OOSHDAQ_BACKUP_RETENTION_COUNT="2"
export OOSHDAQ_COOKIE_SECURE="false"

cd "$workspace_root"
go build -o "$run_root/ooshdaq" ./cmd/ooshdaq
"$run_root/ooshdaq" serve &
server_pid="$!"
wait "$server_pid"
