# Rollback SOP

This SOP assumes the release was installed with the symlink-based layout described in `deployments/README.md`:

```text
/opt/aether/releases/vX.Y.Z
/opt/aether/current -> /opt/aether/releases/vX.Y.Z
```

The current Aether deployment rehearsal already validates binary rollback against the same SQLite runtime database. If a future release introduces incompatible database changes, do not assume binary-only rollback is safe until that compatibility is explicitly validated.

## 1. Rollback Triggers

Start rollback if any of the following happen after cutover:

- The daemon health endpoint does not return `status: ok`
- The observability API does not return `status: ok`
- The service enters a crash loop
- A smoke task cannot complete successfully
- Traces disappear or the observability API cannot query recent traces
- The release violates a critical business or compliance requirement

## 2. Immediate Containment

- Stop new cutover steps.
- Record the failing release version and timestamp.
- Capture the failing health responses and relevant logs before restarting services.
- Confirm which previous release should be restored.

## 3. Identify the Previous Release

Example:

```bash
ls -1 /opt/aether/releases
readlink /opt/aether/current
```

Confirm that the previous release directory still exists and contains:

- `bin/aetherd`
- `bin/observability_api`
- `deployments/`
- `configs/config.yaml` or `configs/config.example.yaml`

## 4. Linux systemd Rollback

1. Point `current` back to the previous bundle:

```bash
sudo ln -sfn /opt/aether/releases/vPREVIOUS /opt/aether/current
```

2. Restart the services:

```bash
sudo systemctl restart aetherd.service
sudo systemctl restart aether-observability-api.service
```

3. Verify status:

```bash
sudo systemctl status aetherd.service --no-pager
sudo systemctl status aether-observability-api.service --no-pager
curl -fsS http://127.0.0.1:8090/api/v1/health
curl -fsS http://127.0.0.1:8082/healthz
```

## 5. macOS launchd Rollback

1. Point `current` back to the previous bundle:

```bash
sudo ln -sfn /opt/aether/releases/vPREVIOUS /opt/aether/current
```

2. Restart the system-domain services:

```bash
sudo launchctl kickstart -k system/io.nikkofu.aetherd
sudo launchctl kickstart -k system/io.nikkofu.aether-observability-api
```

3. Verify status:

```bash
sudo launchctl print system/io.nikkofu.aetherd
sudo launchctl print system/io.nikkofu.aether-observability-api
curl -fsS http://127.0.0.1:8090/api/v1/health
curl -fsS http://127.0.0.1:8082/healthz
```

## 6. User-Domain launchd Rehearsal Rollback

If the rehearsal used custom labels, boot them out with those exact labels:

```bash
launchctl bootout gui/$(id -u) "$HOME/Library/LaunchAgents/aether-rehearsal/io.nikkofu.aetherd.rehearsal.plist"
launchctl bootout gui/$(id -u) "$HOME/Library/LaunchAgents/aether-rehearsal/io.nikkofu.aether-observability-api.rehearsal.plist"
```

If the rehearsal also changed a rehearsal `current` symlink, point it back to the previous rehearsal release before restarting.

## 7. Post-Rollback Verification

After rollback, always confirm:

- The daemon health endpoint is green.
- The observability API health endpoint is green.
- A smoke task or equivalent real request succeeds.
- Recent traces remain queryable.
- The restored release path matches the intended previous version.

Example:

```bash
curl -fsS http://127.0.0.1:8090/api/v1/health
curl -fsS http://127.0.0.1:8082/healthz
```

## 8. Evidence to Capture

Record all of the following in the rollback incident note:

- Failed release version
- Restored release version
- Rollback start and completion times
- Health responses before and after rollback
- Service status output
- Relevant log excerpts
- Whether the runtime database was reused unchanged

## 9. Follow-Up

Do not resume rollout until:

- the root cause is understood,
- the failed release is marked unsafe,
- and the release checklist is rerun for the next candidate.
