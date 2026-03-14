# Gemini CLI Handoff

## Snapshot

- Date: `2026-03-14`
- Repository: `aether`
- Latest tagged release documented in the repo: `v1.8.1`
- Current branch state: `main` contains additional work still listed under `CHANGELOG.md -> Unreleased`
- GitHub sync status from this handoff: pending local commit/push details

## Completed In This Turn

1. Clarified release status in `README.md` so GitHub readers can distinguish the latest tag from unreleased `main` branch work.
2. Added `deployments/GITHUB_RELEASE_RUNBOOK.md` as the maintainer-facing path for:
   - version alignment
   - local validation
   - push to GitHub
   - Actions bundle creation
   - GitHub Release drafting
3. Updated `deployments/README.md` to point maintainers at the new runbook.
4. Updated `.github/workflows/release_bundle.yml` so the workflow summary points back to the runbook.
5. Updated `CHANGELOG.md` to record the documentation/release-process change.

## Validation Already Run

The following commands were run successfully on this machine:

```bash
bash scripts/preflight_local_env.sh
bash scripts/release_gate.sh
```

Observed results:

- `preflight_local_env.sh`: `pass=20 warn=2 fail=0`
- Warnings were expected local privilege warnings for `/opt/aether` and `/Library/LaunchDaemons`
- `release_gate.sh`: passed
- Included checks:
  - backend tests
  - frontend production build
  - standard release smoke against the local daemon
- Skipped by default in this run:
  - OTEL export rehearsal
  - deployment rehearsal
  - acceptance release readiness scenario

## Important Release Constraint

Do not publish a new GitHub Release as `v1.8.1` unless you intentionally want to republish the already tagged release line.

Reason:

- the repository currently contains additional changes under `CHANGELOG.md -> Unreleased`
- `README.md` now explicitly treats `v1.8.1` as the latest tagged release, not the full content of current `main`

The next maintainer must decide whether to cut `v1.8.2` or another version before publishing.

## Remaining Tasks For Gemini CLI

1. Decide the next release version.
   Current repo state is ahead of the documented `v1.8.1` tag.

2. If a new release will be cut, align all versioned surfaces:
   - `VERSION`
   - `README.md` top release line
   - `web-ui/package.json`
   - `CHANGELOG.md` by moving shipped notes out of `Unreleased`

3. Run the full pre-release validation when the environment is available:

```bash
AETHER_RELEASE_GATE_RUN_OTEL_EXPORT_REHEARSAL=1 \
AETHER_RELEASE_GATE_RUN_DEPLOYMENT_REHEARSAL=1 \
AETHER_RELEASE_GATE_RUN_ACCEPTANCE_SCENARIO=1 \
bash scripts/release_gate.sh
```

4. Rebuild release outputs after the version decision:
   - `bash scripts/build_release_bundle.sh`
   - `bash scripts/collect_release_evidence.sh`

5. Push the finalized branch state to GitHub and verify `Aether-CI`.

6. Run the `Aether-Release-Bundle` workflow in GitHub Actions and preserve the workflow URL.

7. Draft or publish the GitHub Release by following `deployments/GITHUB_RELEASE_RUNBOOK.md`.

## Known Follow-Up Worth Investigating

`dist/acceptance/ACCEPTANCE_REPORT.md` shows the hierarchical acceptance scenario completed, but the generated result was still meta-instructional instead of a clean acceptance summary. That is not a blocker for syncing documentation, but it is a quality issue worth fixing before a polished public release.

## Recommended Starting Commands

```bash
git status --short
sed -n '1,120p' VERSION
sed -n '1,120p' CHANGELOG.md
sed -n '1,220p' deployments/GITHUB_RELEASE_RUNBOOK.md
```
