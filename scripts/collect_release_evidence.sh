#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

VERSION="$(tr -d '[:space:]' < VERSION)"
BUNDLE_DIR="${AETHER_EVIDENCE_BUNDLE_DIR:-${ROOT_DIR}/dist/release/v${VERSION}}"
OUTPUT_DIR="${AETHER_EVIDENCE_OUTPUT_DIR:-${ROOT_DIR}/dist/release-evidence/v${VERSION}}"
OUTPUT_PATH="${AETHER_EVIDENCE_OUTPUT_PATH:-${OUTPUT_DIR}/DELIVERY_EVIDENCE.md}"
DAEMON_HEALTH_URL="${AETHER_EVIDENCE_DAEMON_HEALTH_URL:-}"
OBSERVABILITY_HEALTH_URL="${AETHER_EVIDENCE_OBSERVABILITY_HEALTH_URL:-}"
PREFLIGHT_PLATFORM="${AETHER_EVIDENCE_PREFLIGHT_PLATFORM:-}"
INCLUDE_FULL_GIT_STATUS="${AETHER_EVIDENCE_INCLUDE_FULL_GIT_STATUS:-0}"
ACCEPTANCE_REPORT_PATH="${AETHER_EVIDENCE_ACCEPTANCE_REPORT_PATH:-${ROOT_DIR}/dist/acceptance/ACCEPTANCE_REPORT.md}"

log() {
  echo "[collect-release-evidence] $*"
}

fail() {
  echo "[collect-release-evidence] ERROR: $*" >&2
  exit 1
}

require_command() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || fail "missing required command: ${cmd}"
}

write_section() {
  local title="$1"
  {
    echo
    echo "## ${title}"
    echo
  } >> "${OUTPUT_PATH}"
}

write_code_block() {
  local language="$1"
  shift
  {
    echo "\`\`\`${language}"
    "$@"
    echo "\`\`\`"
  } >> "${OUTPUT_PATH}"
}

main() {
  require_command bash
  require_command date
  require_command sed

  [[ -d "${BUNDLE_DIR}" ]] || fail "bundle dir missing: ${BUNDLE_DIR}"
  [[ -f "${BUNDLE_DIR}/MANIFEST.txt" ]] || fail "missing MANIFEST.txt in ${BUNDLE_DIR}"
  [[ -f "${BUNDLE_DIR}/SHA256SUMS" ]] || fail "missing SHA256SUMS in ${BUNDLE_DIR}"
  [[ -f "${ROOT_DIR}/scripts/preflight_local_env.sh" ]] || fail "missing scripts/preflight_local_env.sh"

  mkdir -p "${OUTPUT_DIR}"
  : > "${OUTPUT_PATH}"

  {
    echo "# Delivery Evidence"
    echo
    echo "- Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo "- Repository root: \`${ROOT_DIR}\`"
    echo "- Bundle dir: \`${BUNDLE_DIR}\`"
    echo "- Output path: \`${OUTPUT_PATH}\`"
  } >> "${OUTPUT_PATH}"

  write_section "Manifest"
  write_code_block text sed -n '1,120p' "${BUNDLE_DIR}/MANIFEST.txt"

  write_section "Checksums"
  write_code_block text sed -n '1,200p' "${BUNDLE_DIR}/SHA256SUMS"

  write_section "Preflight"
  {
    echo
    echo "\`\`\`text"
    env \
      AETHER_PREFLIGHT_BUNDLE_DIR="${BUNDLE_DIR}" \
      AETHER_PREFLIGHT_PLATFORM="${PREFLIGHT_PLATFORM}" \
      bash "${ROOT_DIR}/scripts/preflight_local_env.sh" 2>&1
    echo "\`\`\`"
  } >> "${OUTPUT_PATH}"

  if command -v git >/dev/null 2>&1 && git -C "${ROOT_DIR}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    write_section "Git Context"
    write_code_block text git -C "${ROOT_DIR}" rev-parse HEAD
    if [[ "${INCLUDE_FULL_GIT_STATUS}" == "1" ]]; then
      write_code_block text git -C "${ROOT_DIR}" status --short
    else
      write_code_block text git -C "${ROOT_DIR}" status --short -- README.md .github/workflows deployments scripts
    fi
  fi

  if [[ -n "${DAEMON_HEALTH_URL}" ]]; then
    require_command curl
    write_section "Daemon Health"
    {
      echo
      echo "\`\`\`json"
      curl -fsS "${DAEMON_HEALTH_URL}"
      echo
      echo "\`\`\`"
    } >> "${OUTPUT_PATH}"
  fi

  if [[ -n "${OBSERVABILITY_HEALTH_URL}" ]]; then
    require_command curl
    write_section "Observability Health"
    {
      echo
      echo "\`\`\`json"
      curl -fsS "${OBSERVABILITY_HEALTH_URL}"
      echo
      echo "\`\`\`"
    } >> "${OUTPUT_PATH}"
  fi

  if [[ -f "${ACCEPTANCE_REPORT_PATH}" ]]; then
    write_section "Acceptance Report"
    write_code_block markdown sed -n '1,240p' "${ACCEPTANCE_REPORT_PATH}"
  fi

  log "evidence written to ${OUTPUT_PATH}"
}

main "$@"
