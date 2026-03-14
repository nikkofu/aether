# Deployment Assets

This directory contains operator-facing deployment materials that ship with the release bundle.

If you need a GitHub-hosted artifact instead of a locally built bundle, run the `Aether-Release-Bundle` workflow and download the uploaded `dist/release/vX.Y.Z` artifact from Actions. If you need to turn the current repository state into a tagged GitHub release, follow `GITHUB_RELEASE_RUNBOOK.md` in this directory.

## Included Assets

- `docker-compose.observability.yml`: optional Jaeger stack for local trace inspection
- `install_release.sh`: safe installer that stages the current release bundle and renders service assets
- `GITHUB_RELEASE_RUNBOOK.md`: maintainer-side checklist for turning validated repository state into a GitHub-hosted release
- `DELIVERY_HANDOFF.md`: receiver-facing acceptance note for the release bundle
- `RELEASE_CHECKLIST.md`: operator checklist for release readiness, cutover, and evidence capture
- `ROLLBACK_SOP.md`: rollback procedure for systemd and launchd installs
- `systemd/aether-runtime.env.example`: shared environment template for `aetherd` and `observability_api`
- `systemd/aetherd.service`: systemd unit for the main daemon
- `systemd/aether-observability-api.service`: systemd unit for the read-only observability API
- `launchd/aether-runtime.env.example`: shared environment template for macOS launchd installs
- `launchd/run-aetherd.sh`: wrapper that loads shared env and starts `aetherd`
- `launchd/run-observability-api.sh`: wrapper that loads shared env and starts `observability_api`
- `launchd/io.nikkofu.aetherd.plist`: launchd plist for the main daemon
- `launchd/io.nikkofu.aether-observability-api.plist`: launchd plist for the read-only observability API

## Recommended Layout

Use a stable symlink-based layout so roll-forward and rollback stay predictable:

```text
/opt/aether/releases/v1.8.1/
/opt/aether/current -> /opt/aether/releases/v1.8.1
/opt/aether/current/bin/aetherd
/opt/aether/current/bin/observability_api
/opt/aether/current/configs/config.yaml
/opt/aether/shared/aether-runtime.env
/opt/aether/state/aether.db
/opt/aether/state/log/
```

The bundled systemd units assume:

- install root: `/opt/aether/current`
- shared environment file: `/etc/aether/aether-runtime.env`
- runtime database: `/var/lib/aether/aether.db`

The bundled launchd assets assume:

- install root: `/opt/aether/current`
- shared environment file: `/opt/aether/shared/aether-runtime.env`
- runtime database: `/opt/aether/state/aether.db`
- log directory: `/opt/aether/state/log`

If your target host uses different paths, edit the unit files before installation.

## Automated Install

The release bundle includes `deployments/install_release.sh`. It stages the bundle into a versioned release slot, updates the `current` symlink, copies a default `config.yaml` if missing, and renders platform-specific service assets into the target directories you choose.

Before you install, run the local environment preflight from the repository root or unpacked release bundle root:

```bash
bash scripts/preflight_local_env.sh
```

If you want a Markdown evidence package for the release record, run:

```bash
bash scripts/collect_release_evidence.sh
```

If `dist/acceptance/ACCEPTANCE_REPORT.md` exists, the evidence collector will embed it automatically. You can override the path with `AETHER_EVIDENCE_ACCEPTANCE_REPORT_PATH=/custom/report.md`.

If you want a release-grade acceptance walkthrough that combines user-visible output quality with Google-style hierarchical orchestration validation, run:

```bash
bash scripts/acceptance_release_readiness.sh
```

Preview a systemd install:

```bash
bash deployments/install_release.sh --platform systemd
```

Apply a systemd install:

```bash
sudo bash deployments/install_release.sh --platform systemd --apply
```

Preview a launchd install:

```bash
bash deployments/install_release.sh --platform launchd
```

Apply a launchd install:

```bash
sudo bash deployments/install_release.sh --platform launchd --apply
```

Useful overrides:

- `--install-root /custom/aether` to place releases and the `current` symlink elsewhere
- `--state-dir /custom/state` to change the runtime database and log base directory
- `--env-file /custom/path/aether-runtime.env` to render a non-default shared env file
- `--systemd-unit-dir /custom/systemd` to install units outside `/etc/systemd/system`
- `--launchd-plist-dir /custom/LaunchDaemons` to install plists outside `/Library/LaunchDaemons`
- `--daemon-port 18122` and `--observability-port 18123` to avoid port collisions during rehearsal
- `--launchd-daemon-label io.nikkofu.aetherd.rehearsal` and `--launchd-observability-label io.nikkofu.aether-observability-api.rehearsal` to avoid label collisions during launchd rehearsal
- `--force` to replace an existing `releases/vX.Y.Z` slot

Useful preflight overrides:

- `AETHER_PREFLIGHT_PLATFORM=systemd` or `AETHER_PREFLIGHT_PLATFORM=launchd`
- `AETHER_PREFLIGHT_INSTALL_ROOT=/custom/aether`
- `AETHER_PREFLIGHT_DAEMON_PORT=18122`
- `AETHER_PREFLIGHT_OBSERVABILITY_PORT=18123`
- `AETHER_PREFLIGHT_OLLAMA_MODEL=gemma3:270m`

## macOS User-Domain Rehearsal

If you want a real `launchctl` rehearsal without installing into `/opt/aether` and `/Library/LaunchDaemons`, render temporary launch agents under your home directory with unique labels and non-default ports:

```bash
bash deployments/install_release.sh \
  --platform launchd \
  --install-root /tmp/aether-user-rehearsal \
  --state-dir /tmp/aether-user-rehearsal-state \
  --launchd-plist-dir "$HOME/Library/LaunchAgents/aether-rehearsal" \
  --daemon-port 18122 \
  --observability-port 18123 \
  --launchd-daemon-label io.nikkofu.aetherd.rehearsal \
  --launchd-observability-label io.nikkofu.aether-observability-api.rehearsal \
  --apply
```

Then load and verify them in your user domain:

```bash
launchctl bootstrap gui/$(id -u) "$HOME/Library/LaunchAgents/aether-rehearsal/io.nikkofu.aetherd.rehearsal.plist"
launchctl bootstrap gui/$(id -u) "$HOME/Library/LaunchAgents/aether-rehearsal/io.nikkofu.aether-observability-api.rehearsal.plist"
launchctl kickstart -k gui/$(id -u)/io.nikkofu.aetherd.rehearsal
launchctl kickstart -k gui/$(id -u)/io.nikkofu.aether-observability-api.rehearsal
curl -fsS http://127.0.0.1:18122/api/v1/health
curl -fsS http://127.0.0.1:18123/healthz
```

## Linux Install Steps

1. Build or unpack the release bundle into `/opt/aether/releases/vX.Y.Z`.
2. Point `/opt/aether/current` at the desired release directory.
3. Copy `configs/config.example.yaml` to `configs/config.yaml` and adjust runtime/model settings if needed.
4. Copy `deployments/systemd/aether-runtime.env.example` to `/etc/aether/aether-runtime.env` and set real ports, database path, webhook secret, and OTEL endpoint if used.
5. Copy the two systemd unit files into `/etc/systemd/system/`.
6. Reload systemd and start the services:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now aetherd.service
sudo systemctl enable --now aether-observability-api.service
```

## macOS Install Steps

1. Build or unpack the release bundle into `/opt/aether/releases/vX.Y.Z`.
2. Point `/opt/aether/current` at the desired release directory.
3. Create shared state directories:

```bash
sudo mkdir -p /opt/aether/shared /opt/aether/state/log /Library/LaunchDaemons
```

4. Copy `configs/config.example.yaml` to `configs/config.yaml` and adjust runtime/model settings if needed.
5. Copy `deployments/launchd/aether-runtime.env.example` to `/opt/aether/shared/aether-runtime.env` and set real ports, database path, webhook secret, and OTEL endpoint if used.
6. Copy the two plist files into `/Library/LaunchDaemons/`.
7. Validate and load them:

```bash
sudo plutil -lint /Library/LaunchDaemons/io.nikkofu.aetherd.plist
sudo plutil -lint /Library/LaunchDaemons/io.nikkofu.aether-observability-api.plist
sudo launchctl bootstrap system /Library/LaunchDaemons/io.nikkofu.aetherd.plist
sudo launchctl bootstrap system /Library/LaunchDaemons/io.nikkofu.aether-observability-api.plist
sudo launchctl kickstart -k system/io.nikkofu.aetherd
sudo launchctl kickstart -k system/io.nikkofu.aether-observability-api
```

## Health Checks

After startup, verify both surfaces explicitly:

```bash
curl -fsS http://127.0.0.1:8090/api/v1/health
curl -fsS http://127.0.0.1:8082/healthz
```

For release-level validation, run the full local gate before packaging:

```bash
AETHER_RELEASE_GATE_RUN_OTEL_EXPORT_REHEARSAL=1 \
AETHER_RELEASE_GATE_RUN_DEPLOYMENT_REHEARSAL=1 \
bash scripts/release_gate.sh
```
