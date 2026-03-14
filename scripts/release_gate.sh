#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

SKIP_BACKEND="${AETHER_RELEASE_GATE_SKIP_BACKEND:-0}"
SKIP_FRONTEND="${AETHER_RELEASE_GATE_SKIP_FRONTEND:-0}"
SKIP_SMOKE="${AETHER_RELEASE_GATE_SKIP_SMOKE:-0}"
SKIP_OLLAMA_CHECK="${AETHER_RELEASE_GATE_SKIP_OLLAMA_CHECK:-0}"
RUN_OTEL_EXPORT_REHEARSAL="${AETHER_RELEASE_GATE_RUN_OTEL_EXPORT_REHEARSAL:-0}"
RUN_DEPLOYMENT_REHEARSAL="${AETHER_RELEASE_GATE_RUN_DEPLOYMENT_REHEARSAL:-0}"
RUN_ACCEPTANCE_SCENARIO="${AETHER_RELEASE_GATE_RUN_ACCEPTANCE_SCENARIO:-0}"
OLLAMA_BASE_URL="${AETHER_RELEASE_GATE_OLLAMA_BASE_URL:-http://127.0.0.1:11434}"
OLLAMA_MODEL="${AETHER_RELEASE_GATE_OLLAMA_MODEL:-gemma3:270m}"
GO_CACHE_DIR="${AETHER_RELEASE_GATE_GOCACHE:-${ROOT_DIR}/.cache/go-build}"
GO_MOD_CACHE_DIR="${AETHER_RELEASE_GATE_GOMODCACHE:-}"

log() {
  echo "[release-gate] $*"
}

fail() {
  echo "[release-gate] ERROR: $*" >&2
  exit 1
}

require_command() {
  local cmd="$1"
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    fail "missing required command: ${cmd}"
  fi
}

run_step() {
  local label="$1"
  shift

  log "START ${label}"
  "$@"
  log "PASS ${label}"
}

check_ollama_model() {
  if [[ "${SKIP_OLLAMA_CHECK}" == "1" ]]; then
    log "SKIP ollama model availability check"
    return
  fi

  require_command curl
  require_command jq

  log "Checking Ollama model ${OLLAMA_MODEL} via ${OLLAMA_BASE_URL}"

  local tags_payload
  if ! tags_payload="$(curl -fsS "${OLLAMA_BASE_URL%/}/api/tags")"; then
    log "WARN unable to query Ollama tags from ${OLLAMA_BASE_URL}; continuing because release smoke is the authoritative runtime check"
    return
  fi

  if ! printf '%s' "${tags_payload}" | jq -e --arg model "${OLLAMA_MODEL}" '.models[]? | select((.name // "") == $model or (.model // "") == $model)' >/dev/null; then
    fail "required Ollama model ${OLLAMA_MODEL} is unavailable; run: ollama pull ${OLLAMA_MODEL}"
  fi

  log "PASS ollama model preflight"
}

report_observability_mode() {
  if [[ -n "${OTEL_EXPORTER_OTLP_ENDPOINT:-}" ]]; then
    log "OTEL exporter configured at ${OTEL_EXPORTER_OTLP_ENDPOINT}; smoke run will exercise export wiring."
    return
  fi

  log "OTEL exporter not configured; release gate validates local runtime without external OTLP export."
}

main() {
  log "Repository root: ${ROOT_DIR}"
  report_observability_mode

  require_command go
  require_command curl
  mkdir -p "${GO_CACHE_DIR}"
  log "Using Go build cache: ${GO_CACHE_DIR}"
  if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
    mkdir -p "${GO_MOD_CACHE_DIR}"
    log "Using Go module cache: ${GO_MOD_CACHE_DIR}"
  fi

  if [[ "${SKIP_BACKEND}" != "1" ]]; then
    if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
      run_step "backend tests" env GOCACHE="${GO_CACHE_DIR}" GOMODCACHE="${GO_MOD_CACHE_DIR}" go test -count=1 ./...
    else
      run_step "backend tests" env GOCACHE="${GO_CACHE_DIR}" go test -count=1 ./...
    fi
  else
    log "SKIP backend tests"
  fi

  if [[ "${SKIP_FRONTEND}" != "1" ]]; then
    require_command npm
    run_step "frontend production build" bash -lc "cd '${ROOT_DIR}/web-ui' && npm run build"
  else
    log "SKIP frontend production build"
  fi

  if [[ "${SKIP_SMOKE}" != "1" ]]; then
    require_command jq
    check_ollama_model
    run_step "release smoke" bash "${ROOT_DIR}/scripts/release_smoke.sh"
  else
    log "SKIP release smoke"
  fi

  if [[ "${RUN_OTEL_EXPORT_REHEARSAL}" == "1" ]]; then
    run_step "OTEL export rehearsal" bash "${ROOT_DIR}/scripts/otel_export_rehearsal.sh"
  else
    log "SKIP OTEL export rehearsal"
  fi

  if [[ "${RUN_DEPLOYMENT_REHEARSAL}" == "1" ]]; then
    run_step "deployment rehearsal" bash "${ROOT_DIR}/scripts/deployment_rehearsal.sh"
  else
    log "SKIP deployment rehearsal"
  fi

  if [[ "${RUN_ACCEPTANCE_SCENARIO}" == "1" ]]; then
    run_step "acceptance release readiness" bash "${ROOT_DIR}/scripts/acceptance_release_readiness.sh"
  else
    log "SKIP acceptance release readiness"
  fi

  log "Release gate passed."
}

main "$@"
