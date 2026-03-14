#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

VERSION="$(tr -d '[:space:]' < VERSION)"
BUNDLE_DIR="${AETHER_DEPLOY_BUNDLE_DIR:-${ROOT_DIR}/dist/release/v${VERSION}}"
RELEASES_DIR="${AETHER_DEPLOY_RELEASES_DIR:-${ROOT_DIR}/.deploy-rehearsal/releases}"
STATE_DIR="${AETHER_DEPLOY_STATE_DIR:-${ROOT_DIR}/.deploy-rehearsal/state}"
RUN_ID="${AETHER_DEPLOY_RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
DAEMON_PORT="${AETHER_DEPLOY_DAEMON_PORT:-18094}"
OBS_PORT="${AETHER_DEPLOY_OBS_PORT:-18095}"
RUNTIME_DB="${AETHER_DEPLOY_RUNTIME_DB:-${STATE_DIR}/aether-deploy-${RUN_ID}.db}"
WAIT_SECONDS="${AETHER_DEPLOY_WAIT_SECONDS:-60}"
GO_CACHE_DIR="${AETHER_DEPLOY_GOCACHE:-${ROOT_DIR}/.cache/go-build}"
GO_MOD_CACHE_DIR="${AETHER_DEPLOY_GOMODCACHE:-}"

CURRENT_DAEMON_PID=""
CURRENT_OBS_PID=""
ROLLBACK_DAEMON_PID=""
ROLLBACK_OBS_PID=""

log() {
  echo "[deploy-rehearsal] $*"
}

fail() {
  echo "[deploy-rehearsal] ERROR: $*" >&2
  exit 1
}

require_command() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || fail "missing required command: ${cmd}"
}

stop_pid() {
  local pid="$1"
  if [[ -n "${pid}" ]] && kill -0 "${pid}" >/dev/null 2>&1; then
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  fi
}

cleanup() {
  stop_pid "${CURRENT_DAEMON_PID}"
  stop_pid "${CURRENT_OBS_PID}"
  stop_pid "${ROLLBACK_DAEMON_PID}"
  stop_pid "${ROLLBACK_OBS_PID}"
}

trap cleanup EXIT

wait_for_http() {
  local url="$1"
  local label="$2"
  local pid="${3:-}"
  local log_path="${4:-}"
  local deadline=$(( $(date +%s) + WAIT_SECONDS ))

  while [[ $(date +%s) -lt ${deadline} ]]; do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    if [[ -n "${pid}" ]] && ! kill -0 "${pid}" >/dev/null 2>&1; then
      if [[ -n "${log_path}" && -f "${log_path}" ]]; then
        sed -n '1,200p' "${log_path}" >&2 || true
      fi
      fail "${label} exited before becoming ready"
    fi
    sleep 1
  done

  if [[ -n "${log_path}" && -f "${log_path}" ]]; then
    tail -n 80 "${log_path}" >&2 || true
  fi
  fail "${label} did not become ready within ${WAIT_SECONDS}s"
}

assert_recent_traces() {
  local base_url="$1"
  local payload
  payload="$(curl -fsS "${base_url}/org/default/recent_traces")" || fail "failed to query recent traces from ${base_url}"
  if ! printf '%s' "${payload}" | jq -e 'length > 0' >/dev/null; then
    fail "expected recent traces from ${base_url}, got ${payload}"
  fi
}

ensure_bundle() {
  if [[ -x "${BUNDLE_DIR}/bin/aetherd" && -x "${BUNDLE_DIR}/bin/observability_api" && -x "${BUNDLE_DIR}/bin/aether" ]]; then
    log "Using existing release bundle ${BUNDLE_DIR}"
    return
  fi

  log "Release bundle missing or incomplete; building bundle first"
  if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
    env \
      AETHER_RELEASE_BUNDLE_DIR="${BUNDLE_DIR}" \
      AETHER_RELEASE_BUNDLE_GOCACHE="${GO_CACHE_DIR}" \
      AETHER_RELEASE_BUNDLE_GOMODCACHE="${GO_MOD_CACHE_DIR}" \
      bash "${ROOT_DIR}/scripts/build_release_bundle.sh"
  else
    env \
      AETHER_RELEASE_BUNDLE_DIR="${BUNDLE_DIR}" \
      AETHER_RELEASE_BUNDLE_GOCACHE="${GO_CACHE_DIR}" \
      bash "${ROOT_DIR}/scripts/build_release_bundle.sh"
  fi
}

stage_candidate() {
  mkdir -p "${RELEASES_DIR}" "${STATE_DIR}" "${GO_CACHE_DIR}"
  if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
    mkdir -p "${GO_MOD_CACHE_DIR}"
  fi

  local previous_name=""
  if [[ -L "${RELEASES_DIR}/current" ]]; then
    previous_name="$(basename "$(readlink "${RELEASES_DIR}/current")")"
  fi

  CANDIDATE_DIR="${RELEASES_DIR}/${RUN_ID}-v${VERSION}"
  cp -R "${BUNDLE_DIR}" "${CANDIDATE_DIR}"
  ln -sfn "$(basename "${CANDIDATE_DIR}")" "${RELEASES_DIR}/current"

  if [[ -n "${previous_name}" && -d "${RELEASES_DIR}/${previous_name}" ]]; then
    ln -sfn "${previous_name}" "${RELEASES_DIR}/previous"
    PREVIOUS_DIR="${RELEASES_DIR}/${previous_name}"
    log "Rollback slot points to previous bundle ${PREVIOUS_DIR}"
    return
  fi

  ln -sfn "$(basename "${CANDIDATE_DIR}")" "${RELEASES_DIR}/previous"
  PREVIOUS_DIR="${CANDIDATE_DIR}"
  log "No prior bundle found; rollback slot seeded with the current candidate"
}

start_daemon() {
  local binary_path="$1"
  local port="$2"
  local db_path="$3"
  local log_path="$4"

  env \
    AETHER_RUNTIME_DATABASE_PATH="${db_path}" \
    AETHERD_PORT="${port}" \
    "${binary_path}" >"${log_path}" 2>&1 &

  echo $!
}

start_observability_api() {
  local binary_path="$1"
  local port="$2"
  local db_path="$3"
  local log_path="$4"

  env \
    AETHER_RUNTIME_DATABASE_PATH="${db_path}" \
    OBSERVABILITY_API_DATABASE_PATH="${db_path}" \
    OBSERVABILITY_API_PORT="${port}" \
    "${binary_path}" >"${log_path}" 2>&1 &

  echo $!
}

run_external_smoke() {
  env \
    AETHER_RELEASE_SMOKE_MANAGED_DAEMON=0 \
    AETHER_RELEASE_SMOKE_API_BASE="http://127.0.0.1:${DAEMON_PORT}" \
    bash "${ROOT_DIR}/scripts/release_smoke.sh"
}

main() {
  require_command bash
  require_command curl
  require_command jq
  require_command cp

  ensure_bundle
  stage_candidate

  local current_daemon_log="${STATE_DIR}/current-daemon-${RUN_ID}.log"
  local current_obs_log="${STATE_DIR}/current-observability-${RUN_ID}.log"
  local rollback_daemon_log="${STATE_DIR}/rollback-daemon-${RUN_ID}.log"
  local rollback_obs_log="${STATE_DIR}/rollback-observability-${RUN_ID}.log"

  log "Starting candidate daemon from ${CANDIDATE_DIR}"
  CURRENT_DAEMON_PID="$(start_daemon "${CANDIDATE_DIR}/bin/aetherd" "${DAEMON_PORT}" "${RUNTIME_DB}" "${current_daemon_log}")"
  wait_for_http "http://127.0.0.1:${DAEMON_PORT}/api/v1/health" "candidate daemon" "${CURRENT_DAEMON_PID}" "${current_daemon_log}"

  log "Starting candidate observability API from ${CANDIDATE_DIR}"
  CURRENT_OBS_PID="$(start_observability_api "${CANDIDATE_DIR}/bin/observability_api" "${OBS_PORT}" "${RUNTIME_DB}" "${current_obs_log}")"
  wait_for_http "http://127.0.0.1:${OBS_PORT}/healthz" "candidate observability API" "${CURRENT_OBS_PID}" "${current_obs_log}"

  log "Running strict smoke against deployed candidate"
  run_external_smoke

  log "Verifying candidate trace visibility"
  assert_recent_traces "http://127.0.0.1:${OBS_PORT}"

  stop_pid "${CURRENT_OBS_PID}"
  CURRENT_OBS_PID=""
  stop_pid "${CURRENT_DAEMON_PID}"
  CURRENT_DAEMON_PID=""

  log "Starting rollback daemon from ${PREVIOUS_DIR}"
  ROLLBACK_DAEMON_PID="$(start_daemon "${PREVIOUS_DIR}/bin/aetherd" "${DAEMON_PORT}" "${RUNTIME_DB}" "${rollback_daemon_log}")"
  wait_for_http "http://127.0.0.1:${DAEMON_PORT}/api/v1/health" "rollback daemon" "${ROLLBACK_DAEMON_PID}" "${rollback_daemon_log}"

  log "Starting rollback observability API from ${PREVIOUS_DIR}"
  ROLLBACK_OBS_PID="$(start_observability_api "${PREVIOUS_DIR}/bin/observability_api" "${OBS_PORT}" "${RUNTIME_DB}" "${rollback_obs_log}")"
  wait_for_http "http://127.0.0.1:${OBS_PORT}/healthz" "rollback observability API" "${ROLLBACK_OBS_PID}" "${rollback_obs_log}"
  assert_recent_traces "http://127.0.0.1:${OBS_PORT}"

  log "Deployment rehearsal passed."
}

main "$@"
