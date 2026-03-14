#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUNDLE_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

PLATFORM=""
APPLY=0
FORCE=0
INSTALL_ROOT="${AETHER_INSTALL_ROOT:-/opt/aether}"
STATE_DIR="${AETHER_STATE_DIR:-}"
ENV_FILE="${AETHER_ENV_FILE:-}"
SYSTEMD_UNIT_DIR="${AETHER_SYSTEMD_UNIT_DIR:-/etc/systemd/system}"
SYSTEMD_ENV_DIR="${AETHER_SYSTEMD_ENV_DIR:-/etc/aether}"
LAUNCHD_PLIST_DIR="${AETHER_LAUNCHD_PLIST_DIR:-/Library/LaunchDaemons}"
DAEMON_PORT="${AETHER_DAEMON_PORT:-8090}"
OBSERVABILITY_PORT="${AETHER_OBSERVABILITY_PORT:-8082}"
LAUNCHD_DAEMON_LABEL="${AETHER_LAUNCHD_DAEMON_LABEL:-io.nikkofu.aetherd}"
LAUNCHD_OBSERVABILITY_LABEL="${AETHER_LAUNCHD_OBSERVABILITY_LABEL:-io.nikkofu.aether-observability-api}"

log() {
  echo "[install-release] $*"
}

fail() {
  echo "[install-release] ERROR: $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  bash deployments/install_release.sh --platform <systemd|launchd> [options]

Options:
  --platform <systemd|launchd>  Target service manager. Auto-detected if omitted.
  --install-root <path>         Base install root. Default: /opt/aether
  --state-dir <path>            Runtime state directory. Platform-specific default.
  --env-file <path>             Shared runtime env file path. Platform-specific default.
  --systemd-unit-dir <path>     systemd unit target directory. Default: /etc/systemd/system
  --systemd-env-dir <path>      systemd env file directory. Default: /etc/aether
  --launchd-plist-dir <path>    launchd plist target directory. Default: /Library/LaunchDaemons
  --daemon-port <port>          Render AETHERD_PORT into the shared env file. Default: 8090
  --observability-port <port>   Render OBSERVABILITY_API_PORT into the shared env file. Default: 8082
  --launchd-daemon-label <id>   launchd label for the daemon plist. Default: io.nikkofu.aetherd
  --launchd-observability-label <id>
                                launchd label for the observability plist. Default: io.nikkofu.aether-observability-api
  --apply                       Execute the install. Default is dry-run.
  --dry-run                     Print the install plan without making changes.
  --force                       Replace an existing release slot if it already exists.
  --help                        Show this help text.

Examples:
  bash deployments/install_release.sh --platform systemd --dry-run
  bash deployments/install_release.sh --platform launchd --install-root /srv/aether --apply
EOF
}

escape_sed_replacement() {
  printf '%s' "$1" | sed 's/[\/&]/\\&/g'
}

print_cmd() {
  printf '%q ' "$@"
  printf '\n'
}

run_cmd() {
  if [[ "${APPLY}" == "1" ]]; then
    "$@"
    return
  fi

  printf '[dry-run] '
  print_cmd "$@"
}

render_file() {
  local src="$1"
  local dest="$2"
  shift 2

  if [[ "${APPLY}" == "1" ]]; then
    sed "$@" "${src}" > "${dest}"
    return
  fi

  log "[dry-run] render ${src} -> ${dest}"
}

copy_if_missing() {
  local src="$1"
  local dest="$2"

  if [[ -e "${dest}" ]]; then
    log "Preserving existing ${dest}"
    return
  fi

  run_cmd cp "${src}" "${dest}"
}

ensure_bundle_layout() {
  [[ -f "${BUNDLE_ROOT}/VERSION" ]] || fail "missing VERSION under ${BUNDLE_ROOT}"
  [[ -x "${BUNDLE_ROOT}/bin/aetherd" ]] || fail "missing bin/aetherd under ${BUNDLE_ROOT}; run this installer from an unpacked release bundle"
  [[ -x "${BUNDLE_ROOT}/bin/observability_api" ]] || fail "missing bin/observability_api under ${BUNDLE_ROOT}; run this installer from an unpacked release bundle"
  [[ -f "${BUNDLE_ROOT}/configs/config.example.yaml" ]] || fail "missing configs/config.example.yaml under ${BUNDLE_ROOT}"
  [[ -f "${BUNDLE_ROOT}/deployments/systemd/aetherd.service" ]] || fail "missing deployment templates under ${BUNDLE_ROOT}/deployments"
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
      fail "unable to auto-detect platform; pass --platform systemd or --platform launchd"
      ;;
  esac
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --platform)
        PLATFORM="${2:-}"
        shift 2
        ;;
      --install-root)
        INSTALL_ROOT="${2:-}"
        shift 2
        ;;
      --state-dir)
        STATE_DIR="${2:-}"
        shift 2
        ;;
      --env-file)
        ENV_FILE="${2:-}"
        shift 2
        ;;
      --systemd-unit-dir)
        SYSTEMD_UNIT_DIR="${2:-}"
        shift 2
        ;;
      --systemd-env-dir)
        SYSTEMD_ENV_DIR="${2:-}"
        shift 2
        ;;
      --launchd-plist-dir)
        LAUNCHD_PLIST_DIR="${2:-}"
        shift 2
        ;;
      --daemon-port)
        DAEMON_PORT="${2:-}"
        shift 2
        ;;
      --observability-port)
        OBSERVABILITY_PORT="${2:-}"
        shift 2
        ;;
      --launchd-daemon-label)
        LAUNCHD_DAEMON_LABEL="${2:-}"
        shift 2
        ;;
      --launchd-observability-label)
        LAUNCHD_OBSERVABILITY_LABEL="${2:-}"
        shift 2
        ;;
      --apply)
        APPLY=1
        shift
        ;;
      --dry-run)
        APPLY=0
        shift
        ;;
      --force)
        FORCE=1
        shift
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        fail "unknown argument: $1"
        ;;
    esac
  done
}

ensure_defaults() {
  detect_platform

  case "${PLATFORM}" in
    systemd|launchd)
      ;;
    *)
      fail "unsupported platform: ${PLATFORM}"
      ;;
  esac

  if [[ -z "${STATE_DIR}" ]]; then
    if [[ "${PLATFORM}" == "systemd" ]]; then
      STATE_DIR="/var/lib/aether"
    else
      STATE_DIR="${INSTALL_ROOT}/state"
    fi
  fi

  if [[ -z "${ENV_FILE}" ]]; then
    if [[ "${PLATFORM}" == "systemd" ]]; then
      ENV_FILE="${SYSTEMD_ENV_DIR}/aether-runtime.env"
    else
      ENV_FILE="${INSTALL_ROOT}/shared/aether-runtime.env"
    fi
  fi
}

prepare_paths() {
  VERSION="$(tr -d '[:space:]' < "${BUNDLE_ROOT}/VERSION")"
  RELEASE_DIR="${INSTALL_ROOT}/releases/v${VERSION}"
  CURRENT_LINK="${INSTALL_ROOT}/current"
  ENV_DIR="$(dirname "${ENV_FILE}")"
  LOG_DIR="${STATE_DIR}/log"
}

print_summary() {
  log "Bundle root: ${BUNDLE_ROOT}"
  log "Platform: ${PLATFORM}"
  log "Install root: ${INSTALL_ROOT}"
  log "Release dir: ${RELEASE_DIR}"
  log "Current link: ${CURRENT_LINK}"
  log "State dir: ${STATE_DIR}"
  log "Env file: ${ENV_FILE}"
  log "Daemon port: ${DAEMON_PORT}"
  log "Observability port: ${OBSERVABILITY_PORT}"
  if [[ "${PLATFORM}" == "systemd" ]]; then
    log "systemd unit dir: ${SYSTEMD_UNIT_DIR}"
  else
    log "launchd plist dir: ${LAUNCHD_PLIST_DIR}"
    log "launchd daemon label: ${LAUNCHD_DAEMON_LABEL}"
    log "launchd observability label: ${LAUNCHD_OBSERVABILITY_LABEL}"
  fi
  if [[ "${APPLY}" == "1" ]]; then
    log "Mode: apply"
  else
    log "Mode: dry-run"
  fi
}

stage_release_bundle() {
  local release_parent
  release_parent="$(dirname "${RELEASE_DIR}")"

  if [[ -e "${RELEASE_DIR}" ]]; then
    if [[ "${FORCE}" != "1" ]]; then
      fail "release slot already exists at ${RELEASE_DIR}; rerun with --force to replace it"
    fi
    run_cmd rm -rf "${RELEASE_DIR}"
  fi

  run_cmd mkdir -p "${release_parent}"
  if [[ "${APPLY}" == "1" ]]; then
    mkdir -p "${RELEASE_DIR}"
    cp -R "${BUNDLE_ROOT}/." "${RELEASE_DIR}/"
  else
    log "[dry-run] copy ${BUNDLE_ROOT}/. -> ${RELEASE_DIR}/"
  fi

  run_cmd ln -sfn "${RELEASE_DIR}" "${CURRENT_LINK}"
}

install_config_if_missing() {
  local config_target="${RELEASE_DIR}/configs/config.yaml"
  local config_example="${RELEASE_DIR}/configs/config.example.yaml"

  if [[ "${APPLY}" == "1" ]]; then
    [[ -f "${config_example}" ]] || fail "missing ${config_example} after staging release"
  fi

  copy_if_missing "${config_example}" "${config_target}"
}

install_env_file() {
  local template_src="$1"
  local template_db_path="$2"
  local db_path="${STATE_DIR}/aether.db"

  run_cmd mkdir -p "${ENV_DIR}" "${STATE_DIR}"
  if [[ "${PLATFORM}" == "launchd" ]]; then
    run_cmd mkdir -p "${LOG_DIR}"
  fi

  if [[ -e "${ENV_FILE}" ]]; then
    log "Preserving existing ${ENV_FILE}"
    return
  fi

  local escaped_template_db_path
  local escaped_db_path
  local escaped_daemon_port
  local escaped_observability_port
  escaped_template_db_path="$(escape_sed_replacement "${template_db_path}")"
  escaped_db_path="$(escape_sed_replacement "${db_path}")"
  escaped_daemon_port="$(escape_sed_replacement "${DAEMON_PORT}")"
  escaped_observability_port="$(escape_sed_replacement "${OBSERVABILITY_PORT}")"

  render_file "${template_src}" "${ENV_FILE}" \
    -e "s/${escaped_template_db_path}/${escaped_db_path}/g" \
    -e "s/^AETHERD_PORT=.*/AETHERD_PORT=${escaped_daemon_port}/" \
    -e "s/^OBSERVABILITY_API_PORT=.*/OBSERVABILITY_API_PORT=${escaped_observability_port}/"
}

install_systemd_assets() {
  local service_template_root="${RELEASE_DIR}/deployments/systemd"
  local escaped_current_link
  local escaped_env_file

  escaped_current_link="$(escape_sed_replacement "${CURRENT_LINK}")"
  escaped_env_file="$(escape_sed_replacement "${ENV_FILE}")"

  run_cmd mkdir -p "${SYSTEMD_UNIT_DIR}" "${STATE_DIR}" "${ENV_DIR}"
  install_env_file "${service_template_root}/aether-runtime.env.example" "/var/lib/aether/aether.db"

  render_file "${service_template_root}/aetherd.service" "${SYSTEMD_UNIT_DIR}/aetherd.service" \
    -e "s#/opt/aether/current#${escaped_current_link}#g" \
    -e "s#/etc/aether/aether-runtime.env#${escaped_env_file}#g"

  render_file "${service_template_root}/aether-observability-api.service" "${SYSTEMD_UNIT_DIR}/aether-observability-api.service" \
    -e "s#/opt/aether/current#${escaped_current_link}#g" \
    -e "s#/etc/aether/aether-runtime.env#${escaped_env_file}#g"
}

install_launchd_assets() {
  local template_root="${RELEASE_DIR}/deployments/launchd"
  local escaped_current_link
  local escaped_env_file
  local escaped_log_dir
  local escaped_daemon_label
  local escaped_observability_label

  escaped_current_link="$(escape_sed_replacement "${CURRENT_LINK}")"
  escaped_env_file="$(escape_sed_replacement "${ENV_FILE}")"
  escaped_log_dir="$(escape_sed_replacement "${LOG_DIR}")"
  escaped_daemon_label="$(escape_sed_replacement "${LAUNCHD_DAEMON_LABEL}")"
  escaped_observability_label="$(escape_sed_replacement "${LAUNCHD_OBSERVABILITY_LABEL}")"

  run_cmd mkdir -p "${LAUNCHD_PLIST_DIR}" "${ENV_DIR}" "${STATE_DIR}" "${LOG_DIR}"
  install_env_file "${template_root}/aether-runtime.env.example" "/opt/aether/state/aether.db"

  render_file "${template_root}/io.nikkofu.aetherd.plist" "${LAUNCHD_PLIST_DIR}/${LAUNCHD_DAEMON_LABEL}.plist" \
    -e "s#/opt/aether/current#${escaped_current_link}#g" \
    -e "s#/opt/aether/shared/aether-runtime.env#${escaped_env_file}#g" \
    -e "s#/opt/aether/state/log#${escaped_log_dir}#g" \
    -e "s#io.nikkofu.aetherd#${escaped_daemon_label}#g"

  render_file "${template_root}/io.nikkofu.aether-observability-api.plist" "${LAUNCHD_PLIST_DIR}/${LAUNCHD_OBSERVABILITY_LABEL}.plist" \
    -e "s#/opt/aether/current#${escaped_current_link}#g" \
    -e "s#/opt/aether/shared/aether-runtime.env#${escaped_env_file}#g" \
    -e "s#/opt/aether/state/log#${escaped_log_dir}#g" \
    -e "s#io.nikkofu.aether-observability-api#${escaped_observability_label}#g"
}

print_next_steps() {
  if [[ "${PLATFORM}" == "systemd" ]]; then
    log "Next: review ${SYSTEMD_UNIT_DIR}/aetherd.service and ${SYSTEMD_UNIT_DIR}/aether-observability-api.service"
    log "Next: run systemctl daemon-reload && systemctl enable --now aetherd.service aether-observability-api.service"
    return
  fi

  log "Next: review ${LAUNCHD_PLIST_DIR}/${LAUNCHD_DAEMON_LABEL}.plist and ${LAUNCHD_PLIST_DIR}/${LAUNCHD_OBSERVABILITY_LABEL}.plist"
  log "Next: run plutil -lint on the plist files, then bootstrap them with launchctl"
}

main() {
  parse_args "$@"
  ensure_bundle_layout
  ensure_defaults
  prepare_paths
  print_summary
  stage_release_bundle
  install_config_if_missing

  if [[ "${PLATFORM}" == "systemd" ]]; then
    install_systemd_assets
  else
    install_launchd_assets
  fi

  print_next_steps
}

main "$@"
