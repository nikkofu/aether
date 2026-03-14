#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"
PORT="${AETHER_RELEASE_SMOKE_PORT:-18092}"
DB_PATH="${AETHER_RELEASE_SMOKE_DB:-/tmp/aether-release-smoke.db}"
LOG_PATH="${AETHER_RELEASE_SMOKE_LOG:-/tmp/aether-release-smoke.log}"
WAIT_SECONDS="${AETHER_RELEASE_SMOKE_WAIT_SECONDS:-180}"
MANAGED_DAEMON="${AETHER_RELEASE_SMOKE_MANAGED_DAEMON:-1}"
API_BASE="${AETHER_RELEASE_SMOKE_API_BASE:-http://127.0.0.1:${PORT}}"
GO_CACHE_DIR="${AETHER_RELEASE_SMOKE_GOCACHE:-${ROOT_DIR}/.cache/go-build}"
GO_MOD_CACHE_DIR="${AETHER_RELEASE_SMOKE_GOMODCACHE:-}"

cleanup() {
  if [[ "${MANAGED_DAEMON}" == "1" && -n "${DAEMON_PID:-}" ]]; then
    kill "${DAEMON_PID}" >/dev/null 2>&1 || true
    wait "${DAEMON_PID}" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

if [[ "${MANAGED_DAEMON}" == "1" ]]; then
  rm -f "${DB_PATH}" "${LOG_PATH}"
  mkdir -p "${GO_CACHE_DIR}"
  if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
    mkdir -p "${GO_MOD_CACHE_DIR}"
  fi

  if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
    env \
      CGO_ENABLED=0 \
      GOCACHE="${GO_CACHE_DIR}" \
      GOMODCACHE="${GO_MOD_CACHE_DIR}" \
      AETHER_RUNTIME_DATABASE_PATH="${DB_PATH}" \
      AETHERD_PORT="${PORT}" \
      go run cmd/aetherd/main.go >"${LOG_PATH}" 2>&1 &
  else
    env \
      CGO_ENABLED=0 \
      GOCACHE="${GO_CACHE_DIR}" \
      AETHER_RUNTIME_DATABASE_PATH="${DB_PATH}" \
      AETHERD_PORT="${PORT}" \
      go run cmd/aetherd/main.go >"${LOG_PATH}" 2>&1 &
  fi
  DAEMON_PID=$!
fi

echo "Waiting for daemon on ${API_BASE} ..."
for _ in $(seq 1 60); do
  if curl -fsS "${API_BASE}/api/v1/health" >/dev/null 2>&1; then
    break
  fi
  if [[ "${MANAGED_DAEMON}" == "1" ]] && ! kill -0 "${DAEMON_PID}" >/dev/null 2>&1; then
    echo "Daemon exited before health check became ready." >&2
    sed -n '1,200p' "${LOG_PATH}" >&2 || true
    exit 1
  fi
  sleep 1
done

health_payload="$(curl -fsS "${API_BASE}/api/v1/health")" || {
  echo "Daemon did not become healthy within the wait window." >&2
  if [[ "${MANAGED_DAEMON}" == "1" ]]; then
    sed -n '1,200p' "${LOG_PATH}" >&2 || true
  fi
  exit 1
}
echo "Health: ${health_payload}"

task_description="Write exactly 3 bullet points for the Aether release readiness memo. Use these exact bullet prefixes: '- Ship Recommendation:', '- Blockers:', and '- Next Action:'. 'Ship Recommendation' means the public launch decision, not a vessel. The '- Ship Recommendation:' bullet must mention 'public launch decision'. The '- Blockers:' bullet must mention 'OpenTelemetry collector is not running'. The '- Next Action:' bullet must mention 'fix the OpenTelemetry collector'. Use only these facts: daemon health is ok; 16 agents are ready; Go tests pass; the web UI production build passes; the default model is gemma3:270m; OpenTelemetry collector is not running. Keep the total under 80 words. No introduction or conclusion."
task_payload="$(jq -n --arg description "${task_description}" '{
  source: "release_smoke",
  mode: "agent",
  workflow_pattern: "review_critique",
  description: $description,
  input: {
    max_review_iterations: 5
  }
}')"

submit_payload="$(curl -fsS "${API_BASE}/api/v1/tasks" -H 'Content-Type: application/json' -d "${task_payload}")"
task_id="$(printf '%s' "${submit_payload}" | jq -r '.id')"
if [[ -z "${task_id}" || "${task_id}" = "null" ]]; then
  echo "Failed to create smoke task: ${submit_payload}" >&2
  exit 1
fi

echo "Submitted smoke task: ${task_id}"

deadline=$(( $(date +%s) + WAIT_SECONDS ))
terminal_payload=""
while [[ $(date +%s) -lt ${deadline} ]]; do
  current_payload="$(curl -fsS "${API_BASE}/api/v1/tasks/${task_id}")"
  status="$(printf '%s' "${current_payload}" | jq -r '.status')"
  stage="$(printf '%s' "${current_payload}" | jq -r '.current_stage')"
  echo "Task status: ${status} (${stage})"
  if [[ "${status}" = "completed" || "${status}" = "failed" || "${status}" = "cancelled" || "${status}" = "interrupted" ]]; then
    terminal_payload="${current_payload}"
    break
  fi
  sleep 2
done

if [[ -z "${terminal_payload}" ]]; then
  echo "Smoke task did not reach a terminal state within ${WAIT_SECONDS}s" >&2
  if [[ "${MANAGED_DAEMON}" == "1" ]]; then
    echo "Daemon log tail:" >&2
    tail -n 80 "${LOG_PATH}" >&2 || true
  fi
  exit 1
fi

status="$(printf '%s' "${terminal_payload}" | jq -r '.status')"
result="$(printf '%s' "${terminal_payload}" | jq -r '.final_output // ""')"
error_summary="$(printf '%s' "${terminal_payload}" | jq -r '.error_summary // ""')"

echo "Terminal task payload: ${terminal_payload}"

if [[ "${status}" != "completed" ]]; then
  echo "Smoke task failed with status=${status}: ${error_summary}" >&2
  if [[ "${MANAGED_DAEMON}" == "1" ]]; then
    tail -n 80 "${LOG_PATH}" >&2 || true
  fi
  exit 1
fi

bullet_count="$(printf '%s\n' "${result}" | grep -Ec '^[[:space:]]*[-*•][[:space:]]+' || true)"
non_bullet_content="$(printf '%s\n' "${result}" | grep -Ev '^[[:space:]]*$|^[[:space:]]*[-*•][[:space:]]+' || true)"
word_count="$(printf '%s' "${result}" | grep -Eo '[[:alnum:]]+' || true)"
word_count="$(printf '%s\n' "${word_count}" | wc -l | tr -d ' ')"

if [[ "${bullet_count}" -ne 3 ]]; then
  echo "Smoke output violated bullet contract: expected 3 bullets, got ${bullet_count}" >&2
  printf '%s\n' "${result}" >&2
  exit 1
fi

if [[ -n "${non_bullet_content}" ]]; then
  echo "Smoke output contained non-bullet prose despite explicit bullet contract" >&2
  printf '%s\n' "${result}" >&2
  exit 1
fi

if [[ "${word_count}" -ge 80 ]]; then
  echo "Smoke output violated word limit: expected under 80 words, got ${word_count}" >&2
  printf '%s\n' "${result}" >&2
  exit 1
fi

for prefix in "- Ship Recommendation:" "- Blockers:" "- Next Action:"; do
  if ! printf '%s\n' "${result}" | grep -Fq -- "${prefix}"; then
    echo "Smoke output missed required prefix: ${prefix}" >&2
    printf '%s\n' "${result}" >&2
    exit 1
  fi
done

for phrase in "public launch decision" "OpenTelemetry collector is not running" "fix the OpenTelemetry collector"; do
  if ! printf '%s\n' "${result}" | grep -Fqi -- "${phrase}"; then
    echo "Smoke output missed required phrase: ${phrase}" >&2
    printf '%s\n' "${result}" >&2
    exit 1
  fi
done

events_payload="$(curl -fsS "${API_BASE}/api/v1/tasks/${task_id}/events")"
agents_payload="$(curl -fsS "${API_BASE}/api/v1/agents")"

echo "Smoke task output:"
printf '%s\n' "${result}"
echo "Event count: $(printf '%s' "${events_payload}" | jq 'length')"
echo "Agent stats: $(printf '%s' "${agents_payload}" | jq -c '.stats')"
echo "Release smoke passed."
