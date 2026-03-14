#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

VERSION="$(tr -d '[:space:]' < VERSION)"
RELEASE_DIR="${AETHER_RELEASE_BUNDLE_DIR:-${ROOT_DIR}/dist/release/v${VERSION}}"
GO_CACHE_DIR="${AETHER_RELEASE_BUNDLE_GOCACHE:-${ROOT_DIR}/.cache/go-build}"
GO_MOD_CACHE_DIR="${AETHER_RELEASE_BUNDLE_GOMODCACHE:-}"

log() {
  echo "[build-release-bundle] $*"
}

require_command() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || {
    echo "[build-release-bundle] ERROR: missing required command: ${cmd}" >&2
    exit 1
  }
}

sha256_tool() {
  if command -v shasum >/dev/null 2>&1; then
    echo "shasum"
    return
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    echo "sha256sum"
    return
  fi
  echo ""
}

log "Preparing release bundle for v${VERSION}"

require_command go
require_command npm
mkdir -p "${GO_CACHE_DIR}" "${RELEASE_DIR}/bin" "${RELEASE_DIR}/web-ui" "${RELEASE_DIR}/configs" "${RELEASE_DIR}/deployments" "${RELEASE_DIR}/scripts"
if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
  mkdir -p "${GO_MOD_CACHE_DIR}"
fi

log "Building Go binaries into ${RELEASE_DIR}/bin"
if [[ -n "${GO_MOD_CACHE_DIR}" ]]; then
  env CGO_ENABLED=0 GOCACHE="${GO_CACHE_DIR}" GOMODCACHE="${GO_MOD_CACHE_DIR}" go build -o "${RELEASE_DIR}/bin/aether" ./cmd/aether
  env CGO_ENABLED=0 GOCACHE="${GO_CACHE_DIR}" GOMODCACHE="${GO_MOD_CACHE_DIR}" go build -o "${RELEASE_DIR}/bin/aetherd" ./cmd/aetherd
  env CGO_ENABLED=0 GOCACHE="${GO_CACHE_DIR}" GOMODCACHE="${GO_MOD_CACHE_DIR}" go build -o "${RELEASE_DIR}/bin/observability_api" ./cmd/observability_api
else
  env CGO_ENABLED=0 GOCACHE="${GO_CACHE_DIR}" go build -o "${RELEASE_DIR}/bin/aether" ./cmd/aether
  env CGO_ENABLED=0 GOCACHE="${GO_CACHE_DIR}" go build -o "${RELEASE_DIR}/bin/aetherd" ./cmd/aetherd
  env CGO_ENABLED=0 GOCACHE="${GO_CACHE_DIR}" go build -o "${RELEASE_DIR}/bin/observability_api" ./cmd/observability_api
fi

log "Building web-ui production assets"
bash -lc "cd '${ROOT_DIR}/web-ui' && npm run build"
mkdir -p "${RELEASE_DIR}/web-ui"
cp -R "${ROOT_DIR}/web-ui/dist/." "${RELEASE_DIR}/web-ui/"

cp "${ROOT_DIR}/VERSION" "${RELEASE_DIR}/VERSION"
cp "${ROOT_DIR}/README.md" "${RELEASE_DIR}/README.md"
cp "${ROOT_DIR}/CHANGELOG.md" "${RELEASE_DIR}/CHANGELOG.md"
cp "${ROOT_DIR}/configs/config.example.yaml" "${RELEASE_DIR}/configs/config.example.yaml"
cp -R "${ROOT_DIR}/deployments/." "${RELEASE_DIR}/deployments/"
cp "${ROOT_DIR}/scripts/acceptance_release_readiness.sh" "${RELEASE_DIR}/scripts/acceptance_release_readiness.sh"
cp "${ROOT_DIR}/scripts/collect_release_evidence.sh" "${RELEASE_DIR}/scripts/collect_release_evidence.sh"
cp "${ROOT_DIR}/scripts/preflight_local_env.sh" "${RELEASE_DIR}/scripts/preflight_local_env.sh"

cat > "${RELEASE_DIR}/MANIFEST.txt" <<EOF
release_version=v${VERSION}
bundle_path=${RELEASE_DIR}
build_timestamp_utc=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
default_model=gemma3:270m
binaries=aether,aetherd,observability_api
frontend_dir=web-ui
deployment_assets=deployments
installer=deployments/install_release.sh
preflight=scripts/preflight_local_env.sh
acceptance=scripts/acceptance_release_readiness.sh
evidence_collector=scripts/collect_release_evidence.sh
EOF

checksum_cmd="$(sha256_tool)"
if [[ -n "${checksum_cmd}" ]]; then
  log "Generating SHA256SUMS"
  (
    cd "${RELEASE_DIR}"
    : > SHA256SUMS
    while IFS= read -r file; do
      if [[ "${checksum_cmd}" == "shasum" ]]; then
        shasum -a 256 "${file}" >> SHA256SUMS
      else
        sha256sum "${file}" >> SHA256SUMS
      fi
    done < <(find bin web-ui configs deployments scripts -type f | sort)
  )
else
  log "WARN no SHA-256 tool found; skipping SHA256SUMS generation"
fi

log "Release bundle ready at ${RELEASE_DIR}"
