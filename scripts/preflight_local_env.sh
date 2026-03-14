#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

VERSION="$(tr -d '[:space:]' < VERSION)"
PLATFORM="${AETHER_PREFLIGHT_PLATFORM:-}"
INSTALL_ROOT="${AETHER_PREFLIGHT_INSTALL_ROOT:-/opt/aether}"
OLLAMA_BASE_URL="${AETHER_PREFLIGHT_OLLAMA_BASE_URL:-http://127.0.0.1:11434}"
OLLAMA_MODEL="${AETHER_PREFLIGHT_OLLAMA_MODEL:-gemma3:270m}"
DAEMON_PORT="${AETHER_PREFLIGHT_DAEMON_PORT:-8090}"
OBSERVABILITY_PORT="${AETHER_PREFLIGHT_OBSERVABILITY_PORT:-8082}"
BUNDLE_DIR="${AETHER_PREFLIGHT_BUNDLE_DIR:-}"
LAUNCHD_PLIST_DIR="${AETHER_PREFLIGHT_LAUNCHD_PLIST_DIR:-/Library/LaunchDaemons}"
SYSTEMD_UNIT_DIR="${AETHER_PREFLIGHT_SYSTEMD_UNIT_DIR:-/etc/systemd/system}"

PASS_COUNT=0
WARN_COUNT=0
FAIL_COUNT=0

log() {
  echo "[preflight] $*"
}

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  log "PASS $*"
}

warn() {
  WARN_COUNT=$((WARN_COUNT + 1))
  log "WARN $*"
}

fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  log "FAIL $*"
}

detect_platform() {
  if [[ -n "${PLATFORM}" ]]; then
    return
  fi

  case "$(uname -s)" in
    Darwin)
      PLATFORM="launchd"
      ;;
    Linux)
      PLATFORM="systemd"
      ;;
    *)
      PLATFORM="unknown"
      ;;
  esac
}

detect_bundle_dir() {
  if [[ -n "${BUNDLE_DIR}" ]]; then
    return
  fi

  if [[ -x "${ROOT_DIR}/bin/aetherd" && -f "${ROOT_DIR}/deployments/install_release.sh" ]]; then
    BUNDLE_DIR="${ROOT_DIR}"
    return
  fi

  BUNDLE_DIR="${ROOT_DIR}/dist/release/v${VERSION}"
}

check_command() {
  local cmd="$1"
  if command -v "${cmd}" >/dev/null 2>&1; then
    pass "command available: ${cmd}"
    return
  fi
  fail "missing required command: ${cmd}"
}

check_optional_command() {
  local cmd="$1"
  if command -v "${cmd}" >/dev/null 2>&1; then
    pass "optional command available: ${cmd}"
    return
  fi
  warn "optional command unavailable: ${cmd}"
}

check_file() {
  local path="$1"
  if [[ -e "${path}" ]]; then
    pass "found ${path}"
    return
  fi
  fail "missing ${path}"
}

check_bundle() {
  if [[ -d "${BUNDLE_DIR}" ]]; then
    pass "release bundle present: ${BUNDLE_DIR}"
  else
    fail "release bundle missing: ${BUNDLE_DIR}"
    return
  fi

  check_file "${BUNDLE_DIR}/bin/aetherd"
  check_file "${BUNDLE_DIR}/bin/observability_api"
  check_file "${BUNDLE_DIR}/MANIFEST.txt"
  check_file "${BUNDLE_DIR}/deployments/install_release.sh"
  check_file "${BUNDLE_DIR}/deployments/README.md"
  check_file "${BUNDLE_DIR}/scripts/preflight_local_env.sh"
  check_file "${BUNDLE_DIR}/SHA256SUMS"
}

check_platform_tools() {
  case "${PLATFORM}" in
    systemd)
      check_optional_command systemctl
      ;;
    launchd)
      check_optional_command launchctl
      check_optional_command plutil
      ;;
    *)
      warn "unsupported or unknown platform: ${PLATFORM}"
      ;;
  esac
}

check_permissions() {
  local install_parent
  install_parent="$(dirname "${INSTALL_ROOT}")"

  if [[ -w "${install_parent}" || ( -e "${INSTALL_ROOT}" && -w "${INSTALL_ROOT}" ) ]]; then
    pass "install path writable without sudo: ${INSTALL_ROOT}"
  else
    warn "install path requires elevated privileges or a custom --install-root: ${INSTALL_ROOT}"
  fi

  case "${PLATFORM}" in
    systemd)
      if [[ -w "${SYSTEMD_UNIT_DIR}" ]]; then
        pass "systemd unit dir writable: ${SYSTEMD_UNIT_DIR}"
      else
        warn "systemd unit dir not writable without sudo: ${SYSTEMD_UNIT_DIR}"
      fi
      ;;
    launchd)
      if [[ -w "${LAUNCHD_PLIST_DIR}" ]]; then
        pass "launchd plist dir writable: ${LAUNCHD_PLIST_DIR}"
      else
        warn "launchd plist dir not writable without sudo: ${LAUNCHD_PLIST_DIR}"
      fi
      ;;
  esac
}

check_port_free() {
  local port="$1"
  local label="$2"

  if ! command -v lsof >/dev/null 2>&1; then
    warn "skipping port check for ${label}; lsof is unavailable"
    return
  fi

  local listeners
  listeners="$(lsof -nP -iTCP:"${port}" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -z "${listeners}" ]]; then
    pass "${label} port is free: ${port}"
    return
  fi

  warn "${label} port already has a listener on ${port}: $(printf '%s' "${listeners}" | awk 'NR==2 {print $1 \" pid=\" $2}')"
}

check_ollama_model() {
  if ! command -v curl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
    warn "skipping Ollama model check; curl or jq is unavailable"
    return
  fi

  local tags_payload
  if ! tags_payload="$(curl -fsS "${OLLAMA_BASE_URL%/}/api/tags" 2>/dev/null)"; then
    warn "unable to query Ollama at ${OLLAMA_BASE_URL}; model check skipped"
    return
  fi

  if printf '%s' "${tags_payload}" | jq -e --arg model "${OLLAMA_MODEL}" '.models[]? | select((.name // "") == $model or (.model // "") == $model)' >/dev/null; then
    pass "Ollama model available: ${OLLAMA_MODEL}"
    return
  fi

  fail "Ollama endpoint is reachable but required model is missing: ${OLLAMA_MODEL}"
}

print_summary() {
  log "Summary: pass=${PASS_COUNT} warn=${WARN_COUNT} fail=${FAIL_COUNT}"
  if [[ "${FAIL_COUNT}" -gt 0 ]]; then
    exit 1
  fi
}

main() {
  detect_platform
  detect_bundle_dir
  log "Repository root: ${ROOT_DIR}"
  log "Version: v${VERSION}"
  log "Platform: ${PLATFORM}"
  log "Bundle dir: ${BUNDLE_DIR}"
  log "Install root: ${INSTALL_ROOT}"
  log "Daemon port: ${DAEMON_PORT}"
  log "Observability port: ${OBSERVABILITY_PORT}"

  check_command bash
  check_command go
  check_command curl
  check_command jq
  check_command npm
  check_command sqlite3
  check_optional_command ollama

  check_platform_tools
  check_bundle
  check_ollama_model
  check_permissions
  check_port_free "${DAEMON_PORT}" "daemon"
  check_port_free "${OBSERVABILITY_PORT}" "observability"
  print_summary
}

main "$@"
