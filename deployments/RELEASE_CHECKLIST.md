# Release Checklist

Use this checklist before every external delivery, staged rollout, or production cutover.

## 1. Artifact Integrity

- [ ] Confirm the target release version in `VERSION`.
- [ ] Build a fresh release bundle:

```bash
bash scripts/build_release_bundle.sh
```

- [ ] Verify the bundle contains `MANIFEST.txt`, `SHA256SUMS`, `deployments/install_release.sh`, and `scripts/preflight_local_env.sh`.
- [ ] Record the bundle path and build timestamp from `MANIFEST.txt`.

## 2. Local Environment Preflight

- [ ] Run the environment preflight from the repository root or unpacked bundle root:

```bash
bash scripts/preflight_local_env.sh
```

- [ ] Confirm there are no `FAIL` lines.
- [ ] Review any `WARN` lines and decide whether they are acceptable for the target host.
- [ ] Confirm the default local model is still `gemma3:270m` unless the release plan explicitly says otherwise.

## 3. Formal Validation Gate

- [ ] Run the local release gate:

```bash
bash scripts/release_gate.sh
```

- [ ] For full release confidence, run the full gate with OTEL and deployment rehearsal enabled:

```bash
AETHER_RELEASE_GATE_RUN_OTEL_EXPORT_REHEARSAL=1 \
AETHER_RELEASE_GATE_RUN_DEPLOYMENT_REHEARSAL=1 \
bash scripts/release_gate.sh
```

- [ ] Confirm the gate ends with `Release gate passed.`
- [ ] Save the command output or CI job URL as release evidence.

## 4. Functional Experience Checks

- [ ] Run the self-enhancement walkthrough:

```bash
bash scripts/self_enhancement_experience.sh
```

- [ ] Confirm the first constrained task fails deterministically and a later task succeeds.
- [ ] Run the OTEL export rehearsal if the target environment uses OTLP export:

```bash
bash scripts/otel_export_rehearsal.sh
```

- [ ] Confirm spans are exported for the `aether-core` service.

## 5. Deployment Readiness

- [ ] Confirm the target host has the required runtime dependencies:
  - Go is only required for local build/test paths, not for running the packaged binaries.
  - Ollama is available if the deployment uses the local inference path.
  - `launchctl` or `systemctl` is available for the chosen host platform.
- [ ] Confirm the target ports are reserved or configurable.
- [ ] Confirm the shared runtime database path is correct for the host.
- [ ] Confirm the webhook secret and OTEL endpoint values are set if they are required.
- [ ] Confirm a previous release bundle remains available for rollback.

## 6. Installation and Cutover

- [ ] Run the installer in `dry-run` mode first:

```bash
bash deployments/install_release.sh --platform launchd
```

or:

```bash
bash deployments/install_release.sh --platform systemd
```

- [ ] Review the rendered target paths.
- [ ] Run the installer with `--apply` on the target host.
- [ ] Load or restart the services using `launchctl` or `systemctl`.
- [ ] Verify both health endpoints:

```bash
curl -fsS http://127.0.0.1:8090/api/v1/health
curl -fsS http://127.0.0.1:8082/healthz
```

## 7. Post-Cutover Verification

- [ ] Submit one real smoke task or run `scripts/release_smoke.sh` against the deployed daemon.
- [ ] Confirm the observability API can read recent traces.
- [ ] Confirm logs show normal startup with no crash loop.
- [ ] Capture the active release path, service status, and health payloads in the release record.

## 8. Go / No-Go Decision

Release only if all of the following are true:

- [ ] No blocking failures remain.
- [ ] Preflight has zero `FAIL` results.
- [ ] The release gate passed.
- [ ] Health checks are green after installation.
- [ ] A rollback path is available and documented.
