#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

PORT="${AETHER_ACCEPTANCE_PORT:-18124}"
DB_PATH="${AETHER_ACCEPTANCE_DB:-/tmp/aether-acceptance.db}"
LOG_PATH="${AETHER_ACCEPTANCE_LOG:-/tmp/aether-acceptance.log}"
WAIT_SECONDS="${AETHER_ACCEPTANCE_WAIT_SECONDS:-240}"
MANAGED_DAEMON="${AETHER_ACCEPTANCE_MANAGED_DAEMON:-1}"
API_BASE="${AETHER_ACCEPTANCE_API_BASE:-http://127.0.0.1:${PORT}}"
REPORT_PATH="${AETHER_ACCEPTANCE_REPORT_PATH:-${ROOT_DIR}/dist/acceptance/ACCEPTANCE_REPORT.md}"
GO_CACHE_DIR="${AETHER_ACCEPTANCE_GOCACHE:-${ROOT_DIR}/.cache/go-build}"
GO_MOD_CACHE_DIR="${AETHER_ACCEPTANCE_GOMODCACHE:-}"

QUALITY_DESCRIPTION="Write exactly 3 bullet points for the Aether release readiness memo. Use these exact bullet prefixes: '- Ship Recommendation:', '- Blockers:', and '- Next Action:'. 'Ship Recommendation' means the public launch decision, not a vessel. The '- Ship Recommendation:' bullet must mention 'public launch decision'. The '- Blockers:' bullet must mention 'OpenTelemetry collector is not running'. The '- Next Action:' bullet must mention 'fix the OpenTelemetry collector'. Use only these facts: daemon health is ok; 16 agents are ready; Go tests pass; the web UI production build passes; the default model is gemma3:270m; OpenTelemetry collector is not running. Keep the total under 80 words. No introduction or conclusion."
ARCHITECTURE_DESCRIPTION="作为 Aether 首席架构师，请输出一份正式发布前的验收摘要。直接给出以下三个里程碑的具体结论：1. 运行稳定性（包含后端测试通过、16个智能体就绪）；2. 可观测性（包含 traces 导出、metrics API 端口 8082 正常）；3. 交付交接（包含 v1.9.0 标签推送、发布包已生成）。内容请保持在 150 字以内，使用清晰的 Markdown 列表，不要有开场白或自我介绍。"

cleanup() {
  if [[ "${MANAGED_DAEMON}" == "1" && -n "${DAEMON_PID:-}" ]]; then
    kill "${DAEMON_PID}" >/dev/null 2>&1 || true
    wait "${DAEMON_PID}" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

log() {
  echo "[acceptance-release-readiness] $*" >&2
}

fail() {
  echo "[acceptance-release-readiness] ERROR: $*" >&2
  exit 1
}

require_command() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || fail "missing required command: ${cmd}"
}

start_daemon() {
  mkdir -p "$(dirname "${LOG_PATH}")"
  rm -f "${DB_PATH}" "${LOG_PATH}"

  if [[ -x "${ROOT_DIR}/bin/aetherd" ]]; then
    env \
      AETHER_RUNTIME_DATABASE_PATH="${DB_PATH}" \
      AETHERD_PORT="${PORT}" \
      "${ROOT_DIR}/bin/aetherd" >"${LOG_PATH}" 2>&1 &
    echo $!
    return
  fi

  require_command go
  mkdir -p "${GO_CACHE_DIR}"
  if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
    mkdir -p "${GO_MOD_CACHE_DIR}"
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
  echo $!
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
  local workflow_pattern="$1"
  local description="$2"
  local input_json="${3:-}"
  local payload

  if [[ -z "${input_json}" ]]; then
    input_json='{}'
  fi

  payload="$(jq -n \
    --arg source "acceptance_release_readiness" \
    --arg mode "agent" \
    --arg workflow_pattern "${workflow_pattern}" \
    --arg description "${description}" \
    --arg input_json "${input_json}" \
    '{
      source: $source,
      mode: $mode,
      workflow_pattern: $workflow_pattern,
      description: $description,
      input: ($input_json | fromjson)
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
    sleep 2
  done

  fail "task ${task_id} did not reach terminal state within ${WAIT_SECONDS}s"
}

assert_quality_task() {
  local payload="$1"
  local status
  local result
  status="$(printf '%s' "${payload}" | jq -r '.status')"
  result="$(printf '%s' "${payload}" | jq -r '.final_output // ""')"

  [[ "${status}" == "completed" ]] || fail "quality task failed: ${payload}"

  local bullet_count
  local non_bullet_content
  local word_count
  bullet_count="$(printf '%s\n' "${result}" | grep -Ec '^[[:space:]]*[-*•][[:space:]]+' || true)"
  non_bullet_content="$(printf '%s\n' "${result}" | grep -Ev '^[[:space:]]*$|^[[:space:]]*[-*•][[:space:]]+' || true)"
  word_count="$(printf '%s' "${result}" | grep -Eo '[[:alnum:]]+' || true)"
  word_count="$(printf '%s\n' "${word_count}" | wc -l | tr -d ' ')"

  [[ "${bullet_count}" -eq 3 ]] || fail "quality task violated bullet contract: ${result}"
  [[ -z "${non_bullet_content}" ]] || fail "quality task returned non-bullet prose: ${result}"
  [[ "${word_count}" -lt 80 ]] || fail "quality task exceeded word limit (${word_count}): ${result}"

  for prefix in "- Ship Recommendation:" "- Blockers:" "- Next Action:"; do
    printf '%s\n' "${result}" | grep -Fq -- "${prefix}" || fail "quality task missed prefix ${prefix}: ${result}"
  done

  for phrase in "public launch decision" "OpenTelemetry collector is not running" "fix the OpenTelemetry collector"; do
    printf '%s\n' "${result}" | grep -Fqi -- "${phrase}" || fail "quality task missed phrase '${phrase}': ${result}"
  done
}

assert_architecture_task() {
  local payload="$1"
  local events="$2"
  local status
  local final_output

  status="$(printf '%s' "${payload}" | jq -r '.status')"
  final_output="$(printf '%s' "${payload}" | jq -r '.final_output // ""')"

  [[ "${status}" == "completed" ]] || fail "architecture task failed: ${payload}"
  [[ -n "${final_output}" ]] || fail "architecture task completed without final output"

  for actor in "workflow.hierarchical" "strategic_director" "tactical_manager"; do
    printf '%s' "${events}" | jq -e --arg actor "${actor}" '.[] | select((.from // "") == $actor or (.to // "") == $actor)' >/dev/null \
      || fail "architecture task did not route through ${actor}"
  done

  printf '%s' "${events}" | jq -e '.[] | select(((.from // "") | startswith("operational-")) or ((.to // "") | startswith("operational-")))' >/dev/null \
    || fail "architecture task did not involve any operational worker"
}

write_report() {
  local health_payload="$1"
  local quality_task_id="$2"
  local quality_payload="$3"
  local architecture_task_id="$4"
  local architecture_payload="$5"
  local architecture_events="$6"

  mkdir -p "$(dirname "${REPORT_PATH}")"
  cat > "${REPORT_PATH}" <<EOF
# Acceptance Report

- Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
- API base: \`${API_BASE}\`
- Managed daemon: \`${MANAGED_DAEMON}\`
- Database path: \`${DB_PATH}\`

## Health

\`\`\`json
${health_payload}
\`\`\`

## Scenario 1: Release Quality

- Task ID: \`${quality_task_id}\`

\`\`\`json
${quality_payload}
\`\`\`

## Scenario 2: Hierarchical Architecture

- Task ID: \`${architecture_task_id}\`

\`\`\`json
${architecture_payload}
\`\`\`

### Hierarchical Events

\`\`\`json
${architecture_events}
\`\`\`
EOF
}

main() {
  require_command bash
  require_command curl
  require_command jq

  if [[ "${MANAGED_DAEMON}" == "1" ]]; then
    log "starting acceptance daemon on ${API_BASE}"
    DAEMON_PID="$(start_daemon)"
  fi

  log "waiting for daemon health"
  wait_for_health
  local health_payload
  health_payload="$(curl -fsS "${API_BASE}/api/v1/health")"
  log "health: $(printf '%s' "${health_payload}" | jq -c .)"

  log "running release-quality scenario"
  local quality_submit
  local quality_task_id
  local quality_payload
  quality_submit="$(submit_task "review_critique" "${QUALITY_DESCRIPTION}" '{"max_review_iterations":5}')"
  quality_task_id="$(printf '%s' "${quality_submit}" | jq -r '.id')"
  [[ -n "${quality_task_id}" && "${quality_task_id}" != "null" ]] || fail "failed to create quality task: ${quality_submit}"
  quality_payload="$(wait_for_terminal_task "${quality_task_id}")"
  assert_quality_task "${quality_payload}"

  log "running hierarchical architecture scenario"
  local architecture_submit
  local architecture_task_id
  local architecture_payload
  local architecture_events
  architecture_submit="$(submit_task "hierarchical" "${ARCHITECTURE_DESCRIPTION}" '{}')"
  architecture_task_id="$(printf '%s' "${architecture_submit}" | jq -r '.id')"
  [[ -n "${architecture_task_id}" && "${architecture_task_id}" != "null" ]] || fail "failed to create architecture task: ${architecture_submit}"
  architecture_payload="$(wait_for_terminal_task "${architecture_task_id}")"
  architecture_events="$(curl -fsS "${API_BASE}/api/v1/tasks/${architecture_task_id}/events")"
  assert_architecture_task "${architecture_payload}" "${architecture_events}"

  write_report \
    "${health_payload}" \
    "${quality_task_id}" \
    "${quality_payload}" \
    "${architecture_task_id}" \
    "${architecture_payload}" \
    "${architecture_events}"

  log "acceptance passed"
  log "report written to ${REPORT_PATH}"
}

main "$@"
