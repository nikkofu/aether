#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

GRPC_ADDR="${AETHER_OTEL_REHEARSAL_GRPC_ADDR:-127.0.0.1:4317}"
HTTP_ADDR="${AETHER_OTEL_REHEARSAL_HTTP_ADDR:-127.0.0.1:18096}"
SUMMARY_PATH="${AETHER_OTEL_REHEARSAL_SUMMARY_PATH:-/tmp/aether-otlp-capture.json}"
LOG_PATH="${AETHER_OTEL_REHEARSAL_LOG_PATH:-/tmp/aether-otlp-capture.log}"
SMOKE_PORT="${AETHER_OTEL_REHEARSAL_SMOKE_PORT:-18097}"
SMOKE_DB_PATH="${AETHER_OTEL_REHEARSAL_SMOKE_DB_PATH:-/tmp/aether-otlp-smoke.db}"
SMOKE_LOG_PATH="${AETHER_OTEL_REHEARSAL_SMOKE_LOG_PATH:-/tmp/aether-otlp-smoke.log}"
WAIT_SECONDS="${AETHER_OTEL_REHEARSAL_WAIT_SECONDS:-60}"
GO_CACHE_DIR="${AETHER_OTEL_REHEARSAL_GOCACHE:-${ROOT_DIR}/.cache/go-build}"
GO_MOD_CACHE_DIR="${AETHER_OTEL_REHEARSAL_GOMODCACHE:-}"

cleanup() {
  if [[ -n "${CAPTURE_PID:-}" ]]; then
    kill "${CAPTURE_PID}" >/dev/null 2>&1 || true
    wait "${CAPTURE_PID}" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

log() {
  echo "[otel-rehearsal] $*"
}

fail() {
  echo "[otel-rehearsal] ERROR: $*" >&2
  exit 1
}

wait_for_http() {
  local url="$1"
  local label="$2"
  local deadline=$(( $(date +%s) + WAIT_SECONDS ))

  while [[ $(date +%s) -lt ${deadline} ]]; do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    if [[ -n "${CAPTURE_PID:-}" ]] && ! kill -0 "${CAPTURE_PID}" >/dev/null 2>&1; then
      sed -n '1,200p' "${LOG_PATH}" >&2 || true
      fail "${label} exited before becoming ready"
    fi
    sleep 1
  done

  sed -n '1,200p' "${LOG_PATH}" >&2 || true
  fail "${label} did not become ready within ${WAIT_SECONDS}s"
}

wait_for_export_summary() {
  local url="$1"
  local deadline=$(( $(date +%s) + WAIT_SECONDS ))

  while [[ $(date +%s) -lt ${deadline} ]]; do
    local payload
    payload="$(curl -fsS "${url}")" || {
      sleep 1
      continue
    }
    if printf '%s' "${payload}" | jq -e '.request_count > 0 and .span_count > 0 and (.services | index("aether-core") != null)' >/dev/null; then
      printf '%s' "${payload}"
      return 0
    fi
    sleep 1
  done

  return 1
}

rm -f "${SUMMARY_PATH}" "${LOG_PATH}"
mkdir -p "${GO_CACHE_DIR}"
if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
  mkdir -p "${GO_MOD_CACHE_DIR}"
fi

if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
  env \
    GOCACHE="${GO_CACHE_DIR}" \
    GOMODCACHE="${GO_MOD_CACHE_DIR}" \
    OTLP_CAPTURE_GRPC_ADDR="${GRPC_ADDR}" \
    OTLP_CAPTURE_HTTP_ADDR="${HTTP_ADDR}" \
    OTLP_CAPTURE_OUTPUT="${SUMMARY_PATH}" \
    go run ./cmd/otlp_capture >"${LOG_PATH}" 2>&1 &
else
  env \
    GOCACHE="${GO_CACHE_DIR}" \
    OTLP_CAPTURE_GRPC_ADDR="${GRPC_ADDR}" \
    OTLP_CAPTURE_HTTP_ADDR="${HTTP_ADDR}" \
    OTLP_CAPTURE_OUTPUT="${SUMMARY_PATH}" \
    go run ./cmd/otlp_capture >"${LOG_PATH}" 2>&1 &
fi
CAPTURE_PID=$!

log "Waiting for OTLP capture server on ${HTTP_ADDR}"
wait_for_http "http://${HTTP_ADDR}/healthz" "OTLP capture server"

log "Running strict smoke with OTLP exporter ${GRPC_ADDR}"
env \
  OTEL_EXPORTER_OTLP_ENDPOINT="${GRPC_ADDR}" \
  AETHER_RELEASE_SMOKE_PORT="${SMOKE_PORT}" \
  AETHER_RELEASE_SMOKE_DB="${SMOKE_DB_PATH}" \
  AETHER_RELEASE_SMOKE_LOG="${SMOKE_LOG_PATH}" \
  bash "${ROOT_DIR}/scripts/release_smoke.sh"

summary_payload="$(wait_for_export_summary "http://${HTTP_ADDR}/summary")" || fail "expected exported spans for aether-core, got $(curl -fsS "http://${HTTP_ADDR}/summary" 2>/dev/null || cat "${SUMMARY_PATH}" 2>/dev/null || echo '{}')"

log "OTLP capture summary: ${summary_payload}"
log "OTEL export rehearsal passed."
