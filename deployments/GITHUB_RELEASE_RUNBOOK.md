# GitHub Release Runbook

This runbook is for the repository maintainer path: take the current repository state, validate it, sync it to GitHub, and publish a tagged GitHub release with a downloadable release bundle.

This intentionally separates two different actions:

- syncing `main` to GitHub
- publishing a new versioned GitHub release

Do not treat those as the same thing. If `CHANGELOG.md` still contains material under `Unreleased`, that work is on `main` but is not yet a versioned release.

## 1. Align Release Metadata

Before you publish a new version:

1. Decide the next release version. The latest tagged release documented in the repository is currently `v1.8.1`.
2. If the intended release includes items still listed under `## [Unreleased]` in `CHANGELOG.md`, do not reuse `v1.8.1`.
3. Update:
   - `VERSION`
   - the release line at the top of `README.md`
   - `web-ui/package.json` if the frontend package version should stay aligned
4. Move the shipped notes from `## [Unreleased]` into a dated version section in `CHANGELOG.md`.
5. Rebuild any release artifacts or evidence that embed the release version.

## 2. Validate Locally

Run the minimum local validation before pushing:

```bash
bash scripts/preflight_local_env.sh
AETHER_RELEASE_GATE_SKIP_OLLAMA_CHECK=1 \
AETHER_RELEASE_GATE_SKIP_SMOKE=1 \
bash scripts/release_gate.sh
```

Run the full local validation before a public release when the environment is available:

```bash
AETHER_RELEASE_GATE_RUN_OTEL_EXPORT_REHEARSAL=1 \
AETHER_RELEASE_GATE_RUN_DEPLOYMENT_REHEARSAL=1 \
AETHER_RELEASE_GATE_RUN_ACCEPTANCE_SCENARIO=1 \
bash scripts/release_gate.sh
```

Expected outcome:

- the preflight summary reports zero `FAIL`
- the release gate ends with `Release gate passed.`

If the full gate is not possible on the current machine, record exactly which steps were skipped and why in the release notes or delivery evidence.

## 3. Sync the Repository to GitHub

Once local documentation and validation are in a publishable state:

```bash
git status --short
git add -A
git commit -m "docs: prepare GitHub release workflow and handoff"
git push origin main
```

After the push:

1. Wait for `Aether-CI` to complete on GitHub.
2. Confirm the `release-gate` job is green.
3. Confirm the README renders correctly on the repository front page.

Do not create a tag until the pushed branch state and CI results match the intended release record.

## 4. Build the GitHub-Hosted Release Bundle

Use the GitHub Actions workflow when you want a standard downloadable artifact attached to the repository history:

1. Open the `Aether-Release-Bundle` workflow in GitHub Actions.
2. Set `run_ci_release_gate=true` unless there is a documented reason to skip it.
3. Keep `artifact_name=aether-release-bundle` unless a different naming convention is required.
4. Start the workflow and wait for the artifact upload step to finish.

The workflow will:

- run the lightweight CI release gate
- build `dist/release/vX.Y.Z`
- preflight the built bundle on the runner
- upload the bundle as an Actions artifact

Download the artifact and keep its workflow URL with the release record.

## 5. Draft the GitHub Release

When the bundle workflow succeeds:

1. Create or verify the tag `vX.Y.Z`.
2. Draft a GitHub Release titled `Aether vX.Y.Z`.
3. Use the matching `CHANGELOG.md` section for the release notes.
4. Include links or references to:
   - the Actions run that produced the bundle
   - `deployments/README.md`
   - `deployments/RELEASE_CHECKLIST.md`
   - `deployments/ROLLBACK_SOP.md`
5. Mention any validation gaps explicitly if the full local gate was not run.

If you are using `gh` instead of the GitHub web UI, make sure the generated notes still match the curated `CHANGELOG.md` entry before publishing.

## 6. Capture Release Evidence

Keep a release record that includes:

- the pushed commit SHA
- the GitHub Actions URL for `Aether-CI`
- the GitHub Actions URL for `Aether-Release-Bundle`
- the artifact name and version
- the exact commands used for local validation
- any skipped checks and the reason

If local evidence is needed, generate it with:

```bash
bash scripts/collect_release_evidence.sh
```

## 7. Post-Publish Checks

After the GitHub Release is published:

1. Confirm the release tag matches `VERSION`.
2. Confirm the artifact version matches the tag.
3. Confirm the README and changelog link targets work from GitHub.
4. Confirm deployment assets in the bundle still include:
   - `deployments/install_release.sh`
   - `deployments/DELIVERY_HANDOFF.md`
   - `deployments/RELEASE_CHECKLIST.md`
   - `deployments/ROLLBACK_SOP.md`
5. Archive the evidence package and release URLs in the team handoff record.
