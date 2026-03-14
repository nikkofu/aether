#!/usr/bin/env bash
set -euo pipefail

INSTALL_ROOT="${AETHER_INSTALL_ROOT:-/opt/aether/current}"
ENV_FILE="${AETHER_ENV_FILE:-/opt/aether/shared/aether-runtime.env}"

if [[ -f "${ENV_FILE}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  set +a
fi

cd "${INSTALL_ROOT}"
exec "${INSTALL_ROOT}/bin/observability_api"
