# Delivery Handoff

This note is the shortest path for an external receiver or operator who needs to accept the Aether release bundle and confirm it is ready for installation.

## What Is in the Bundle

The release bundle contains:

- compiled binaries for `aether`, `aetherd`, and `observability_api`
- the built `web-ui`
- `MANIFEST.txt` and `SHA256SUMS`
- deployment assets under `deployments/`
- the environment preflight under `scripts/preflight_local_env.sh`

## Receiver Acceptance Steps

1. Verify the artifact contents:

```bash
cat MANIFEST.txt
grep -F "deployments/install_release.sh" SHA256SUMS
grep -F "scripts/preflight_local_env.sh" SHA256SUMS
```

2. Run the environment preflight from the unpacked bundle root:

```bash
bash scripts/preflight_local_env.sh
```

3. Review:

- `deployments/README.md`
- `deployments/RELEASE_CHECKLIST.md`
- `deployments/ROLLBACK_SOP.md`

4. Run the installer in `dry-run` mode for the target platform:

```bash
bash deployments/install_release.sh --platform launchd
```

or:

```bash
bash deployments/install_release.sh --platform systemd
```

5. If the target host is approved for cutover, run the installer with `--apply`, then verify:

```bash
curl -fsS http://127.0.0.1:8090/api/v1/health
curl -fsS http://127.0.0.1:8082/healthz
```

## Evidence Capture

To record acceptance evidence without mutating the bundle itself, run:

```bash
bash scripts/collect_release_evidence.sh
```

The default output is written under `dist/release-evidence/vX.Y.Z/`.

## Sign-Off Conditions

The receiver should not sign off unless all of the following are true:

- the preflight reports zero `FAIL` lines
- the bundle contents match the expected manifest and checksums
- the target installation path and service manager are understood
- a rollback path is available
- post-install health checks succeed
