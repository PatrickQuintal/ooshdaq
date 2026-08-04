#!/usr/bin/env bash

set -Eeuo pipefail

readonly repository_root="$(
  cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." >/dev/null 2>&1
  pwd -P
)"
readonly docker_bin="${DOCKER_BIN:-docker}"
readonly keep_resources="${OOSHDAQ_SMOKE_KEEP:-0}"
readonly suffix="$(date -u +%Y%m%dT%H%M%SZ)-$$"
readonly image="ooshdaq:container-smoke-${suffix}"
readonly work_directory="$(mktemp -d /tmp/ooshdaq-container-smoke.XXXXXX)"

declare -a containers=()
declare -a volumes=()
image_created=0
image_id=""
image_repo_digest=""
image_size=""
image_source=""
server_url=""
csrf_token=""

log() {
  printf '\n==> %s\n' "$*"
}

fail() {
  printf '\nERROR: %s\n' "$*" >&2
  exit 1
}

engine() {
  "$docker_bin" "$@"
}

cleanup() {
  local container volume

  if [[ "$keep_resources" == "1" ]]; then
    printf '\nSmoke resources retained for inspection:\n' >&2
    printf '  image: %s\n  work directory: %s\n' "$image" "$work_directory" >&2
    printf '  containers: %s\n  volumes: %s\n' \
      "${containers[*]:-(none)}" "${volumes[*]:-(none)}" >&2
    return
  fi

  set +e
  for container in "${containers[@]}"; do
    engine rm --force "$container" >/dev/null 2>&1
  done
  for volume in "${volumes[@]}"; do
    engine volume rm "$volume" >/dev/null 2>&1
  done
  if [[ "$image_created" == "1" ]]; then
    engine image rm "$image" >/dev/null 2>&1
  fi
  case "$work_directory" in
    /tmp/ooshdaq-container-smoke.*)
      rm -rf -- "$work_directory"
      ;;
  esac
}

trap cleanup EXIT

require_command() {
  command -v "$1" >/dev/null 2>&1 ||
    fail "required command is unavailable: $1"
}

create_volume() {
  local volume="$1"

  volumes+=("$volume")
  engine volume create "$volume" >/dev/null
}

wait_until_ready() {
  local container="$1"
  local attempt=0

  while ((attempt < 60)); do
    if curl --fail --silent --show-error --max-time 2 \
      "${server_url}/health/ready" >/dev/null 2>&1; then
      return
    fi
    if [[ "$(engine inspect --format '{{.State.Running}}' "$container")" != "true" ]]; then
      engine logs "$container" >&2 || true
      fail "container ${container} exited before becoming ready"
    fi
    attempt=$((attempt + 1))
    sleep 1
  done

  engine logs "$container" >&2 || true
  fail "container ${container} did not become ready within 60 seconds"
}

assert_runtime_policy() {
  local container="$1"
  local top_output

  [[ "$(engine inspect --format '{{.Config.User}}' "$container")" == "65532:65532" ]] ||
    fail "container ${container} is not configured for UID:GID 65532:65532"
  [[ "$(engine inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$container")" == "true" ]] ||
    fail "container ${container} does not have a read-only root filesystem"
  engine inspect "$container" |
    jq -e '.[0].HostConfig.CapDrop | index("ALL") != null' >/dev/null ||
    fail "container ${container} does not drop all Linux capabilities"
  engine inspect "$container" |
    jq -e '.[0].HostConfig.SecurityOpt | index("no-new-privileges:true") != null' >/dev/null ||
    fail "container ${container} does not enable no-new-privileges"
  engine inspect "$container" |
    jq -e '
      .[0].HostConfig.NanoCpus == 1000000000 and
      .[0].HostConfig.Memory == 268435456 and
      .[0].HostConfig.MemorySwap == 268435456
    ' >/dev/null ||
    fail "container ${container} does not enforce the 1 CPU/256 MiB lane"

  top_output="$(engine top "$container" -eo pid,user,group,comm)"
  printf '%s\n' "$top_output"
  printf '%s\n' "$top_output" |
    awk '
      NR > 1 {
        seen = 1
        if ($2 == "root" || $2 == "0") {
          exit 1
        }
      }
      END {
        if (!seen) {
          exit 1
        }
      }
    ' || fail "container ${container} has no visible non-root process"
}

start_server() {
  local container="$1"
  local mounted_data_volume="$2"
  local mounted_backup_volume="$3"
  local port_line host_port

  containers+=("$container")
  engine run --detach \
    --name "$container" \
    --user "65532:65532" \
    --read-only \
    --tmpfs "/tmp:rw,noexec,nosuid,nodev,size=16m,mode=1777" \
    --cap-drop ALL \
    --cpus "1.0" \
    --memory "256m" \
    --memory-swap "256m" \
    --security-opt "no-new-privileges:true" \
    --stop-timeout 60 \
    --mount "type=volume,src=${mounted_data_volume},dst=/data" \
    --mount "type=volume,src=${mounted_backup_volume},dst=/backups" \
    --env "OOSHDAQ_ADDR=:8080" \
    --env "OOSHDAQ_AUTH_MODE=mock" \
    --env "OOSHDAQ_ALLOW_MOCK_AUTH=true" \
    --env "OOSHDAQ_COOKIE_SECURE=false" \
    --env "OOSHDAQ_DATA_PATH=/data/ooshdaq.sqlite" \
    --env "OOSHDAQ_BACKUP_DIR=/backups" \
    --env "OOSHDAQ_BACKUP_INTERVAL=1m" \
    --env "OOSHDAQ_BACKUP_RETENTION_COUNT=4" \
    --publish "127.0.0.1::8080" \
    "$image" >/dev/null

  port_line="$(engine port "$container" 8080/tcp | head -n 1)"
  host_port="${port_line##*:}"
  [[ "$host_port" =~ ^[0-9]+$ ]] ||
    fail "could not determine the published port for ${container}: ${port_line}"
  server_url="http://127.0.0.1:${host_port}"
  wait_until_ready "$container"

  engine inspect "$container" |
    jq -e --arg data "$mounted_data_volume" --arg backup "$mounted_backup_volume" '
      (.[0].Mounts | any(.Name == $data and .Destination == "/data")) and
      (.[0].Mounts | any(.Name == $backup and .Destination == "/backups"))
    ' >/dev/null ||
    fail "container ${container} does not use the expected separate volumes"
  assert_runtime_policy "$container"
}

login() {
  local cookie_jar="$1"
  local session response

  session="$(curl --fail --silent --show-error \
    --cookie-jar "$cookie_jar" \
    "${server_url}/api/v1/session")"
  csrf_token="$(jq -er '
    .csrf_token | select(type == "string" and length > 0)
  ' <<<"$session")"
  response="$(curl --fail --silent --show-error \
    --cookie "$cookie_jar" \
    --cookie-jar "$cookie_jar" \
    --header "Content-Type: application/json" \
    --header "X-CSRF-Token: ${csrf_token}" \
    --data '{"user_id":"mock-alex"}' \
    "${server_url}/auth/mock-login")"
  jq -e '
    .authenticated == true and
    .user.id == "mock-alex" and
    .auth_mode == "mock"
  ' <<<"$response" >/dev/null ||
    fail "mock login did not return the expected authenticated session"
}

verify_value() {
  local cookie_jar="$1"
  local catalog_id="$2"
  local item_id="$3"
  local response

  response="$(curl --fail --silent --show-error \
    --cookie "$cookie_jar" \
    "${server_url}/api/v1/catalogs/${catalog_id}/collection?state=owned&limit=100")"
  jq -e --arg item "$item_id" '
    any(
      .entries[];
      .catalog_item.id == $item and
      .owned == 1 and
      .available == 1 and
      .wanted == 0
    )
  ' <<<"$response" >/dev/null ||
    fail "the expected collection value is absent"
}

stop_gracefully() {
  local container="$1"
  local logs

  engine stop --time 60 "$container" >/dev/null
  [[ "$(engine inspect --format '{{.State.ExitCode}}' "$container")" == "0" ]] ||
    fail "container ${container} did not exit successfully after SIGTERM"
  logs="$(engine logs "$container" 2>&1)"
  printf '%s\n' "$logs" |
    jq -eR 'fromjson? | select(.msg == "SQLite shutdown checkpoint complete")' \
      >/dev/null ||
    fail "container ${container} did not log a completed shutdown checkpoint"
}

latest_backup_record() {
  local container="$1"
  local attempt=0
  local record=""

  while ((attempt < 30)); do
    record="$(
      engine logs "$container" 2>&1 |
        jq -Rr '
          fromjson? |
          select(.msg == "SQLite online backup complete") |
          [.path, .sha256, (.size_bytes | tostring)] |
          @tsv
        ' |
        tail -n 1
    )"
    if [[ -n "$record" ]]; then
      printf '%s\n' "$record"
      return
    fi
    attempt=$((attempt + 1))
    sleep 1
  done

  fail "container ${container} did not publish an online backup within 30 seconds"
}

for command in curl jq file readelf sha256sum tar awk; do
  require_command "$command"
done
command -v "$docker_bin" >/dev/null 2>&1 ||
  fail "Docker CLI is unavailable; set DOCKER_BIN to an executable client path"

log "Checking the Docker client and daemon"
if ! engine version >"$work_directory/docker-version.txt" 2>&1; then
  cat "$work_directory/docker-version.txt" >&2
  fail "Docker daemon is unavailable; start it, then rerun scripts/container-smoke.sh"
fi
cat "$work_directory/docker-version.txt"

log "Building the pinned Dockerfile"
engine build --pull --progress plain --tag "$image" "$repository_root"
image_created=1
image_id="$(engine image inspect --format '{{.Id}}' "$image")"
[[ "$image_id" == sha256:* ]] || fail "locally built image has no immutable image ID"
image_repo_digest="$(
  engine image inspect --format '{{if .RepoDigests}}{{index .RepoDigests 0}}{{end}}' "$image"
)"
image_size="$(engine image inspect --format '{{.Size}}' "$image")"
image_source="$(
  engine image inspect --format '{{index .Config.Labels "org.opencontainers.image.source"}}' "$image"
)"
[[ "$image_source" == "https://github.com/PatrickQuintal/ooshdaq" ]] ||
  fail "image source label does not match the canonical repository"

log "Inspecting the scratch, static, non-root image"
readonly inspect_container="ooshdaq-smoke-inspect-${suffix}"
containers+=("$inspect_container")
engine create --name "$inspect_container" "$image" >/dev/null
[[ "$(engine image inspect --format '{{.Config.User}}' "$image")" == "65532:65532" ]] ||
  fail "image user is not 65532:65532"
[[ "$(engine image inspect --format '{{json .Config.Entrypoint}}' "$image")" == '["/ooshdaq"]' ]] ||
  fail "image entrypoint is not /ooshdaq"

engine cp "$inspect_container:/ooshdaq" "$work_directory/ooshdaq"
engine export --output "$work_directory/rootfs.tar" "$inspect_container"
file "$work_directory/ooshdaq" | tee "$work_directory/file.txt"
grep -q "statically linked" "$work_directory/file.txt" ||
  fail "image binary is not reported as statically linked"
if readelf -l "$work_directory/ooshdaq" |
  grep -q "Requesting program interpreter"; then
  fail "image binary declares a dynamic program interpreter"
fi
for forbidden in bin lib lib64 sbin usr var; do
  if tar -tf "$work_directory/rootfs.tar" |
    sed -e 's#^\./##' -e 's#/$##' |
    grep -Eq "^${forbidden}(/|$)"; then
    fail "scratch root filesystem unexpectedly contains /${forbidden}"
  fi
done
unexpected_rootfs_entries="$(
  tar -tf "$work_directory/rootfs.tar" |
    sed -e 's#^\./##' -e 's#/$##' |
    grep -Ev '^(\.dockerenv|backups|data|dev(/.*)?|etc(/(hostname|hosts|mtab|resolv\.conf))?|licenses(/.*)?|ooshdaq|proc(/.*)?|sys(/.*)?)$' ||
    true
)"
[[ -z "$unexpected_rootfs_entries" ]] ||
  fail "scratch root filesystem contains unexpected entries: ${unexpected_rootfs_entries}"
for required in ooshdaq data backups licenses/LICENSE licenses/NOTICE; do
  tar -tf "$work_directory/rootfs.tar" |
    sed -e 's#^\./##' -e 's#/$##' |
    grep -qx "$required" ||
    fail "scratch root filesystem is missing /${required}"
done
for owned in ooshdaq data/ backups/ licenses/; do
  tar --numeric-owner -tvf "$work_directory/rootfs.tar" |
    awk -v path="$owned" '
      $2 == "65532/65532" && $6 == path {
        found = 1
      }
      END {
        if (!found) {
          exit 1
        }
      }
    ' || fail "scratch root filesystem has incorrect ownership for /${owned}"
done
if command -v go >/dev/null 2>&1; then
  go version -m "$work_directory/ooshdaq"
fi

readonly data_volume="ooshdaq-smoke-data-${suffix}"
readonly backup_volume="ooshdaq-smoke-backups-${suffix}"
readonly restore_data_volume="ooshdaq-smoke-restore-data-${suffix}"
readonly restore_backup_volume="ooshdaq-smoke-restore-backups-${suffix}"
create_volume "$data_volume"
create_volume "$backup_volume"
create_volume "$restore_data_volume"
create_volume "$restore_backup_volume"

log "Starting generation 1 and writing an authenticated collection value"
readonly first_container="ooshdaq-smoke-first-${suffix}"
start_server "$first_container" "$data_volume" "$backup_volume"
readonly first_cookie_jar="$work_directory/first.cookies"
login "$first_cookie_jar"

catalog_response="$(curl --fail --silent --show-error \
  --cookie "$first_cookie_jar" \
  "${server_url}/api/v1/catalogs?limit=1")"
catalog_id="$(jq -er '.catalogs | select(length == 1) | .[0].id' <<<"$catalog_response")"
collection_response="$(curl --fail --silent --show-error \
  --cookie "$first_cookie_jar" \
  "${server_url}/api/v1/catalogs/${catalog_id}/collection?limit=1")"
item_id="$(jq -er '
  .entries | select(length == 1) | .[0].catalog_item.id
' <<<"$collection_response")"
update_response="$(curl --fail --silent --show-error \
  --request PUT \
  --cookie "$first_cookie_jar" \
  --header "Content-Type: application/json" \
  --header "X-CSRF-Token: ${csrf_token}" \
  --data '{"owned":1,"available":1,"wanted":0}' \
  "${server_url}/api/v1/collection/${item_id}")"
jq -e '
  .entry.owned == 1 and
  .entry.available == 1 and
  .entry.wanted == 0
' <<<"$update_response" >/dev/null ||
  fail "collection PUT did not return the expected value"
verify_value "$first_cookie_jar" "$catalog_id" "$item_id"
stop_gracefully "$first_container"
engine rm "$first_container" >/dev/null

log "Replacing generation 1 while preserving the data and backup volumes"
readonly second_container="ooshdaq-smoke-second-${suffix}"
start_server "$second_container" "$data_volume" "$backup_volume"
readonly second_cookie_jar="$work_directory/second.cookies"
login "$second_cookie_jar"
verify_value "$second_cookie_jar" "$catalog_id" "$item_id"

IFS=$'\t' read -r backup_name backup_sha256 backup_size \
  <<<"$(latest_backup_record "$second_container")"
[[ "$backup_name" =~ ^ooshdaq-[0-9]{8}T[0-9]{6}\.[0-9]{9}Z\.sqlite$ ]] ||
  fail "backup log reported an unexpected filename: ${backup_name}"
[[ "$backup_sha256" =~ ^[0-9a-f]{64}$ ]] ||
  fail "backup log reported an invalid SHA-256 digest"
[[ "$backup_size" =~ ^[1-9][0-9]*$ ]] ||
  fail "backup log reported an invalid size"

engine cp \
  "$second_container:/backups/$backup_name" \
  "$work_directory/$backup_name"
actual_backup_sha256="$(
  sha256sum "$work_directory/$backup_name" | awk '{print $1}'
)"
actual_backup_size="$(
  wc -c <"$work_directory/$backup_name" | tr -d '[:space:]'
)"
[[ "$actual_backup_sha256" == "$backup_sha256" ]] ||
  fail "copied backup digest does not match the verified application digest"
[[ "$actual_backup_size" == "$backup_size" ]] ||
  fail "copied backup size does not match the verified application size"
stop_gracefully "$second_container"

log "Restoring the verified backup into an isolated data volume"
readonly seed_container="ooshdaq-smoke-restore-seed-${suffix}"
start_server "$seed_container" "$restore_data_volume" "$restore_backup_volume"
stop_gracefully "$seed_container"

readonly restore_container="ooshdaq-smoke-restore-job-${suffix}"
containers+=("$restore_container")
engine run \
  --name "$restore_container" \
  --user "65532:65532" \
  --read-only \
  --tmpfs "/tmp:rw,noexec,nosuid,nodev,size=16m,mode=1777" \
  --cap-drop ALL \
  --cpus "1.0" \
  --memory "256m" \
  --memory-swap "256m" \
  --security-opt "no-new-privileges:true" \
  --mount "type=volume,src=${restore_data_volume},dst=/data" \
  --mount "type=volume,src=${backup_volume},dst=/backups" \
  "$image" \
  maintenance restore \
  "--backup=/backups/${backup_name}" \
  "--database=/data/ooshdaq.sqlite" \
  "--retain-replaced=/data/pre-restore.sqlite"
[[ "$(engine inspect --format '{{.State.ExitCode}}' "$restore_container")" == "0" ]] ||
  fail "offline backup restore did not exit successfully"

log "Reading the collection value from the isolated restored database"
readonly verify_container="ooshdaq-smoke-restore-verify-${suffix}"
start_server "$verify_container" "$restore_data_volume" "$restore_backup_volume"
readonly restore_cookie_jar="$work_directory/restore.cookies"
login "$restore_cookie_jar"
verify_value "$restore_cookie_jar" "$catalog_id" "$item_id"
stop_gracefully "$verify_container"

log "Container persistence smoke passed"
printf 'image=%s\nimage_id=%s\nimage_repo_digest=%s\nimage_size_bytes=%s\nimage_source=%s\nitem_id=%s\nbackup=%s\nbackup_sha256=%s\n' \
  "$image" \
  "$image_id" \
  "$image_repo_digest" \
  "$image_size" \
  "$image_source" \
  "$item_id" \
  "$backup_name" \
  "$backup_sha256"
