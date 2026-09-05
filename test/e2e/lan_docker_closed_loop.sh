#!/usr/bin/env bash
set -euo pipefail

# Full Docker closed loop: control plane AND the device Agent both run as
# containers. This is the deployable LAN shape a user runs on Ubuntu; the
# existing lan_upload_download.sh covers the Agent as a host process. This
# script proves the containerized Agent (pure-Go SQLite, CGO_ENABLED=0) serves
# authenticated tus uploads, list, Range/full download, SHA-256 and survives a
# container restart with persisted bytes.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
control_port="${CONTROL_SERVER_TEST_PORT:-18080}"
agent_port="${LAN_AGENT_TEST_PORT:-19090}"
work_dir="$(mktemp -d)"

for command in curl jq openssl sha256sum docker; do
  command -v "$command" >/dev/null || { echo "ERROR: missing $command" >&2; exit 1; }
done

compose() {
  (cd "$repo_root/deploy/compose" && \
    CONTROL_SERVER_BIND_ADDRESS=127.0.0.1 \
    CONTROL_SERVER_PORT="$control_port" \
    LAN_AGENT_BIND_ADDRESS=127.0.0.1 \
    LAN_AGENT_PORT="$agent_port" \
    POSTGRES_DB=share_disk \
    POSTGRES_USER=share_disk \
    POSTGRES_PASSWORD="$(cat "$work_dir/postgres_password")" \
    POSTGRES_PASSWORD_FILE="$work_dir/postgres_password" \
    POSTGRESQL_URL_FILE="$work_dir/postgresql_url" \
    BOOTSTRAP_TOKEN_FILE="$work_dir/bootstrap_token" \
    ACCESS_PRIVATE_KEY_FILE="$work_dir/access_private_key" \
    ACCESS_PUBLIC_KEY_FILE="$work_dir/access_public_key" \
    docker compose -f compose.yaml -f compose.agent-test.yaml -f "$work_dir/compose.override.yaml" "$@")
}

cleanup() {
  compose down -v >/dev/null 2>&1 || true
  rm -rf "$work_dir"
}
trap cleanup EXIT

wait_http() {
  local url="$1"
  for _ in $(seq 1 60); do
    curl -fsS --max-time 2 "$url" >/dev/null 2>&1 && return 0
    sleep 1
  done
  return 1
}

openssl rand -hex 32 >"$work_dir/bootstrap_token"
openssl genpkey -algorithm ED25519 -out "$work_dir/access_private_key"
openssl pkey -in "$work_dir/access_private_key" -pubout -out "$work_dir/access_public_key"
openssl rand -hex 32 >"$work_dir/postgres_password"
printf 'postgres://share_disk:%s@postgres:5432/share_disk?sslmode=disable' \
  "$(cat "$work_dir/postgres_password")" >"$work_dir/postgresql_url"
jq -n \
  --argjson port "$control_port" \
  --arg password "$(cat "$work_dir/postgres_password")" \
  --arg bootstrap_token "$(cat "$work_dir/bootstrap_token")" \
  --rawfile access_private_key "$work_dir/access_private_key" \
  '{server:{host:"0.0.0.0",port:$port,read_timeout:"30s",write_timeout:"30s",idle_timeout:"2m",shutdown_timeout:"30s",migrations_dir:"/app/migrations/postgres",worker_interval:"50ms"},database:{host:"postgres",port:5432,name:"share_disk",user:"share_disk",password:$password,ssl_mode:"disable",max_open_conns:25,max_idle_conns:10,conn_max_lifetime:"5m"},logging:{level:"info",format:"json",output:"stdout"},monitoring:{enabled:false,port:8081,external_access:false},auth:{bootstrap_token:$bootstrap_token,access_private_key:$access_private_key,access_token_ttl:"15m"}}' \
  >"$work_dir/server.json"
chmod 0600 "$work_dir/server.json"
printf 'services:\n  db-migrate:\n    volumes:\n      - "%s:/app/conf/server.json:ro"\n  control-server:\n    volumes:\n      - "%s:/app/conf/server.json:ro"\n  control-worker:\n    volumes:\n      - "%s:/app/conf/server.json:ro"\n' \
  "$work_dir/server.json" "$work_dir/server.json" "$work_dir/server.json" >"$work_dir/compose.override.yaml"

if ! compose up -d --wait postgres db-migrate control-server control-worker agent; then
  compose ps --all >&2
  compose logs postgres db-migrate control-server control-worker agent >&2
  exit 1
fi
wait_http "http://127.0.0.1:$control_port/readyz" || {
  compose ps >&2
  compose logs control-server >&2
  exit 1
}
wait_http "http://127.0.0.1:$agent_port/livez" || {
  compose logs agent >&2
  exit 1
}

control="http://127.0.0.1:$control_port"
agent="http://127.0.0.1:$agent_port"

auth_json="$(jq -nc \
  --arg token "$(cat "$work_dir/bootstrap_token")" \
  '{bootstrap_token:$token,account:"docker-closed-loop",password:"docker-closed-loop-pw",device_name:"android-e2e",platform:"android"}')"
access_token="$(curl -fsS \
  -H 'Content-Type: application/json' \
  --data "$auth_json" \
  "$control/v1/auth/bootstrap" | jq -er '.access_token')"

unauthorized_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  "$agent/v1/lan/files")"
[[ "$unauthorized_status" == 401 ]]
unauthorized_trash_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  "$agent/v1/lan/trash")"
[[ "$unauthorized_trash_status" == 401 ]]

fixture="$repo_root/README.md"
fixture_size="$(stat -c %s "$fixture")"
fixture_sha="$(sha256sum "$fixture" | awk '{print $1}')"
metadata="filename $(printf 'README.md' | base64 -w0),mime $(printf 'text/markdown' | base64 -w0)"

curl -fsS -D "$work_dir/create.headers" -o /dev/null \
  -X POST \
  -H "Authorization: Bearer $access_token" \
  -H 'Tus-Resumable: 1.0.0' \
  -H "Upload-Length: $fixture_size" \
  -H "Upload-Metadata: $metadata" \
  "$agent/v1/lan/uploads/"
upload_location="$(awk 'BEGIN{IGNORECASE=1} /^Location:/{sub(/\r$/, "", $2); print $2}' "$work_dir/create.headers")"
[[ -n "$upload_location" ]]
[[ "$upload_location" = http* ]] || upload_location="$agent$upload_location"

split_at=$((fixture_size / 2))
head -c "$split_at" "$fixture" >"$work_dir/part1"
tail -c "+$((split_at + 1))" "$fixture" >"$work_dir/part2"
curl -fsS -o /dev/null -X PATCH \
  -H "Authorization: Bearer $access_token" \
  -H 'Tus-Resumable: 1.0.0' \
  -H 'Upload-Offset: 0' \
  -H 'Content-Type: application/offset+octet-stream' \
  --data-binary @"$work_dir/part1" "$upload_location"
resume_offset="$(curl -fsS -I \
  -H "Authorization: Bearer $access_token" \
  -H 'Tus-Resumable: 1.0.0' "$upload_location" | \
  awk 'BEGIN{IGNORECASE=1} /^Upload-Offset:/{sub(/\r$/, "", $2); print $2}')"
[[ "$resume_offset" == "$split_at" ]]
curl -fsS -o /dev/null -X PATCH \
  -H "Authorization: Bearer $access_token" \
  -H 'Tus-Resumable: 1.0.0' \
  -H "Upload-Offset: $resume_offset" \
  -H 'Content-Type: application/offset+octet-stream' \
  --data-binary @"$work_dir/part2" "$upload_location"

files="$(curl -fsS -H "Authorization: Bearer $access_token" "$agent/v1/lan/files")"
file_id="$(jq -er 'if (.files | length) == 1 and .files[0].name == "README.md" then .files[0].id else empty end' <<<"$files")"
[[ "$(jq -er '.files[0].sha256' <<<"$files")" == "$fixture_sha" ]]
duplicate_status="$(curl -sS -o /dev/null -w '%{http_code}' -X POST \
  -H "Authorization: Bearer $access_token" \
  -H 'Tus-Resumable: 1.0.0' \
  -H "Upload-Length: $fixture_size" \
  -H "Upload-Metadata: $metadata" \
  "$agent/v1/lan/uploads/")"
[[ "$duplicate_status" == 409 ]]

curl -fsS -H "Authorization: Bearer $access_token" \
  -H 'Range: bytes=0-15' \
  "$agent/v1/lan/files/$file_id/content" >"$work_dir/range"
cmp "$work_dir/range" <(head -c 16 "$fixture")
curl -fsS -H "Authorization: Bearer $access_token" \
  "$agent/v1/lan/files/$file_id/content" >"$work_dir/download"
[[ "$(sha256sum "$work_dir/download" | awk '{print $1}')" == "$fixture_sha" ]]

renamed="$(curl -fsS -X PATCH \
  -H "Authorization: Bearer $access_token" \
  -H 'Content-Type: application/json' \
  --data '{"name":"README-renamed.md"}' \
  "$agent/v1/lan/files/$file_id")"
[[ "$(jq -er '.name' <<<"$renamed")" == "README-renamed.md" ]]
curl -fsS -X DELETE -H "Authorization: Bearer $access_token" \
  "$agent/v1/lan/files/$file_id" >/dev/null
[[ "$(curl -fsS -H "Authorization: Bearer $access_token" "$agent/v1/lan/files" | jq -er '.files | length')" == 0 ]]
[[ "$(curl -fsS -H "Authorization: Bearer $access_token" "$agent/v1/lan/trash" | jq -er '.files[0].name')" == "README-renamed.md" ]]
trashed_download_status="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $access_token" \
  "$agent/v1/lan/files/$file_id/content")"
[[ "$trashed_download_status" == 404 ]]

compose restart agent >/dev/null 2>&1
wait_http "http://127.0.0.1:$agent_port/livez" || {
  compose logs agent >&2
  exit 1
}
trash_after_restart="$(curl -fsS -H "Authorization: Bearer $access_token" \
  "$agent/v1/lan/trash")"
[[ "$(jq -er '.files[0].sha256' <<<"$trash_after_restart")" == "$fixture_sha" ]]
curl -fsS -X POST -H "Authorization: Bearer $access_token" \
  "$agent/v1/lan/trash/$file_id/restore" >/dev/null
curl -fsS -X DELETE -H "Authorization: Bearer $access_token" \
  "$agent/v1/lan/files/$file_id" >/dev/null
curl -fsS -X DELETE -H "Authorization: Bearer $access_token" \
  "$agent/v1/lan/trash/$file_id" >/dev/null
purged_status="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $access_token" \
  "$agent/v1/lan/files/$file_id/content")"
[[ "$purged_status" == 404 ]]

echo "Docker closed loop passed: Agent auth rejection, resumable transfer, name conflict, hash, rename, trash/restore/purge, restart persistence"
