#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

PORT="${AETHER_SELF_ENHANCEMENT_PORT:-18110}"
DB_PATH="${AETHER_SELF_ENHANCEMENT_DB:-/tmp/aether-self-enhancement.db}"
LOG_PATH="${AETHER_SELF_ENHANCEMENT_LOG:-/tmp/aether-self-enhancement.log}"
WAIT_SECONDS="${AETHER_SELF_ENHANCEMENT_WAIT_SECONDS:-120}"
API_BASE="${AETHER_SELF_ENHANCEMENT_API_BASE:-http://127.0.0.1:${PORT}}"
GO_CACHE_DIR="${AETHER_SELF_ENHANCEMENT_GOCACHE:-${ROOT_DIR}/.cache/go-build}"
GO_MOD_CACHE_DIR="${AETHER_SELF_ENHANCEMENT_GOMODCACHE:-}"
MANAGED_DAEMON="${AETHER_SELF_ENHANCEMENT_MANAGED_DAEMON:-1}"

FAIL_DESCRIPTION="Write exactly 3 bullet points for the Aether release readiness memo. Use these exact bullet prefixes: '- Ship Recommendation:', '- Blockers:', and '- Next Action:'. The '- Ship Recommendation:' bullet must mention 'public launch decision'. The '- Blockers:' bullet must mention 'OpenTelemetry collector is not running'. The '- Next Action:' bullet must mention 'fix the OpenTelemetry collector'. Keep the total under 10 words. No introduction or conclusion."
PASS_DESCRIPTION="Write exactly 3 bullet points for the Aether release readiness memo. Use these exact bullet prefixes: '- Ship Recommendation:', '- Blockers:', and '- Next Action:'. The '- Ship Recommendation:' bullet must mention 'public launch decision'. The '- Blockers:' bullet must mention 'OpenTelemetry collector is not running'. The '- Next Action:' bullet must mention 'fix the OpenTelemetry collector'. Use only these facts: daemon health is ok; 16 agents are ready; Go tests pass; the web UI production build passes; the default model is gemma3:270m; OpenTelemetry collector is not running. Keep the total under 80 words. No introduction or conclusion."

cleanup() {
  if [[ "${MANAGED_DAEMON}" == "1" && -n "${DAEMON_PID:-}" ]]; then
    kill "${DAEMON_PID}" >/dev/null 2>&1 || true
    wait "${DAEMON_PID}" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

log() {
  echo "[self-enhancement-demo] $*" >&2
}

fail() {
  echo "[self-enhancement-demo] ERROR: $*" >&2
  exit 1
}

require_command() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || fail "missing required command: ${cmd}"
}

wait_for_health() {
  local deadline=$(( $(date +%s) + WAIT_SECONDS ))

  while [[ $(date +%s) -lt ${deadline} ]]; do
    if curl -fsS "${API_BASE}/api/v1/health" >/dev/null 2>&1; then
      return 0
    fi
    if [[ "${MANAGED_DAEMON}" == "1" && -n "${DAEMON_PID:-}" ]] && ! kill -0 "${DAEMON_PID}" >/dev/null 2>&1; then
      sed -n '1,200p' "${LOG_PATH}" >&2 || true
      fail "daemon exited before becoming healthy"
    fi
    sleep 1
  done

  if [[ "${MANAGED_DAEMON}" == "1" && -f "${LOG_PATH}" ]]; then
    sed -n '1,200p' "${LOG_PATH}" >&2 || true
  fi
  fail "daemon did not become healthy within ${WAIT_SECONDS}s"
}

submit_task() {
  local description="$1"
  local payload
  payload="$(jq -n --arg d "${description}" '{
    source: "self_enhancement_demo",
    mode: "agent",
    workflow_pattern: "review_critique",
    description: $d,
    input: {
      max_review_iterations: 5
    }
  }')"

  curl -fsS "${API_BASE}/api/v1/tasks" \
    -H 'Content-Type: application/json' \
    -d "${payload}"
}

wait_for_terminal_task() {
  local task_id="$1"
  local deadline=$(( $(date +%s) + WAIT_SECONDS ))
  local current_payload=""

  while [[ $(date +%s) -lt ${deadline} ]]; do
    current_payload="$(curl -fsS "${API_BASE}/api/v1/tasks/${task_id}")"
    local status
    local stage
    status="$(printf '%s' "${current_payload}" | jq -r '.status')"
    stage="$(printf '%s' "${current_payload}" | jq -r '.current_stage')"
    log "task ${task_id}: ${status} (${stage})"

    case "${status}" in
      completed|failed|cancelled|interrupted)
        printf '%s' "${current_payload}"
        return 0
        ;;
    esac
    sleep 1
  done

  fail "task ${task_id} did not reach terminal state within ${WAIT_SECONDS}s"
}

print_sql_section() {
  local title="$1"
  local query="$2"

  echo
  echo "=== ${title} ==="
  sqlite3 -header -column "${DB_PATH}" "${query}"
}

main() {
  require_command curl
  require_command jq
  require_command sqlite3
  require_command go
  mkdir -p "${GO_CACHE_DIR}"
  if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
    mkdir -p "${GO_MOD_CACHE_DIR}"
  fi

  if [[ "${MANAGED_DAEMON}" == "1" ]]; then
    rm -f "${DB_PATH}" "${LOG_PATH}"
    log "starting daemon on ${API_BASE} with ${DB_PATH}"
    if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
      env \
        CGO_ENABLED=0 \
        GOCACHE="${GO_CACHE_DIR}" \
        GOMODCACHE="${GO_MOD_CACHE_DIR}" \
        AETHER_RUNTIME_DATABASE_PATH="${DB_PATH}" \
        AETHERD_PORT="${PORT}" \
        go run ./cmd/aetherd >"${LOG_PATH}" 2>&1 &
    else
      env \
        CGO_ENABLED=0 \
        GOCACHE="${GO_CACHE_DIR}" \
        AETHER_RUNTIME_DATABASE_PATH="${DB_PATH}" \
        AETHERD_PORT="${PORT}" \
        go run ./cmd/aetherd >"${LOG_PATH}" 2>&1 &
    fi
    DAEMON_PID=$!
  fi

  log "waiting for daemon health"
  wait_for_health
  log "health: $(curl -fsS "${API_BASE}/api/v1/health" | jq -c .)"

  print_sql_section "Initial Strategies" \
    "select agent_name, prompt_hint, retry_limit, routing_hint from strategies order by agent_name;"

  log "submitting impossible task to force deterministic failure"
  fail_submit="$(submit_task "${FAIL_DESCRIPTION}")"
  fail_task_id="$(printf '%s' "${fail_submit}" | jq -r '.id')"
  [[ -n "${fail_task_id}" && "${fail_task_id}" != "null" ]] || fail "failed to submit fail task: ${fail_submit}"
  echo
  echo "Fail task id: ${fail_task_id}"

  fail_terminal="$(wait_for_terminal_task "${fail_task_id}")"
  echo
  echo "=== Fail Task Terminal Payload ==="
  printf '%s\n' "${fail_terminal}" | jq '{id,status,current_stage,error_summary,final_output}'

  fail_status="$(printf '%s' "${fail_terminal}" | jq -r '.status')"
  [[ "${fail_status}" == "failed" ]] || fail "expected fail task to fail, got ${fail_status}"

  echo
  echo "=== Fail Task Events ==="
  curl -fsS "${API_BASE}/api/v1/tasks/${fail_task_id}/events" \
    | jq '[.[] | {type, from: .from, to: .to, approved: .payload.approved, review_decision_source: .payload.review_decision_source, quality_gate_violations: .payload.quality_gate_violations, reviewer_protocol_violations: .payload.reviewer_protocol_violations}]'

  print_sql_section "Reflections After Failure" \
    "select agent_name, success, substr(analysis,1,120) as analysis, substr(suggestions,1,160) as suggestions from reflections order by created_at desc limit 10;"
  print_sql_section "Strategies After Failure" \
    "select agent_name, prompt_hint, retry_limit, routing_hint from strategies order by agent_name;"

  log "submitting follow-up real task"
  pass_submit="$(submit_task "${PASS_DESCRIPTION}")"
  pass_task_id="$(printf '%s' "${pass_submit}" | jq -r '.id')"
  [[ -n "${pass_task_id}" && "${pass_task_id}" != "null" ]] || fail "failed to submit pass task: ${pass_submit}"
  echo
  echo "Pass task id: ${pass_task_id}"

  pass_terminal="$(wait_for_terminal_task "${pass_task_id}")"
  echo
  echo "=== Pass Task Terminal Payload ==="
  printf '%s\n' "${pass_terminal}" | jq '{id,status,current_stage,error_summary,final_output}'

  pass_status="$(printf '%s' "${pass_terminal}" | jq -r '.status')"
  [[ "${pass_status}" == "completed" ]] || fail "expected pass task to complete, got ${pass_status}"

  echo
  echo "=== Pass Task Events ==="
  curl -fsS "${API_BASE}/api/v1/tasks/${pass_task_id}/events" \
    | jq '[.[] | {type, from: .from, to: .to, approved: .payload.approved, review_decision_source: .payload.review_decision_source}]'

  print_sql_section "Final Strategies" \
    "select agent_name, prompt_hint, retry_limit, routing_hint from strategies order by agent_name;"
  print_sql_section "Final Reflections" \
    "select agent_name, success, substr(analysis,1,120) as analysis, substr(suggestions,1,160) as suggestions from reflections order by created_at desc limit 10;"

  echo
  echo "=== Final Output ==="
  printf '%s\n' "${pass_terminal}" | jq -r '.final_output'

  echo
  log "self-enhancement experience passed"
}

main "$@"
