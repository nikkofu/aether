# Aether

Latest tagged release: `v1.9.0`  
Release date: `2026-03-14`  
Main branch status: post-`v1.9.0` release-prep work is tracked under [`CHANGELOG.md` -> `Unreleased`](CHANGELOG.md#unreleased)

Aether is a Go-based multi-agent runtime focused on three things:

- agent orchestration through a local message bus or NATS
- observable task execution with OpenTelemetry and SQLite-backed traces
- multiple operator surfaces: CLI, daemon mode, pipeline runner, and a small Web UI

The repository currently contains both the main Aether runtime and a few sidecar demos. The primary runnable surfaces are:

- `cmd/aether`: CLI entry for single-task execution, pipeline runs, strategic mode, and skill commands
- `cmd/aetherd`: daemon process exposing webhook and SSE event streaming
- `cmd/observability_api`: read-only trace and metrics API
- `web-ui`: Vite + React dashboard for live event streaming

## What Works Today

- Explicit workflow-driven task execution with `workflow.sequential`, `workflow.parallel`, `workflow.loop`, `workflow.review_critique`, `workflow.iterative_refinement`, `workflow.coordinator`, and `workflow.hierarchical`
- Deterministic task-level self-enhancement: terminal task outcomes persist reflections, update `coder`/`reviewer` strategies, and feed future runs through learned prompt rules
- Streaming token feedback in the CLI
- SQLite-backed memory, audit, ledger, knowledge graph, and trace storage
- Pipeline execution from YAML DAG definitions
- Skill compilation and WASM execution infrastructure
- Webhook and SSE daemon for event intake and live monitoring

## Current Status

The repository is usable as an experimental multi-agent runtime. The main CLI path is the most complete product surface today.

The daemon, Web UI, strategy engine, cluster scheduling, and skill/WASM layers are available, but parts of those areas are still evolving. Read [ARCHITECTURE.md](ARCHITECTURE.md) before extending the system.

Workflow support is intentionally constrained to explicit Google Cloud-style agentic patterns that have real runtime implementations. Unsupported pattern enums are not treated as hidden fallbacks.

The self-enhancement boundary is also explicit: workflow outcomes are observed at task termination, deterministic quality signals are persisted as reflections, and those reflections update the shared strategy store used by planner/coder/reviewer LLM calls on the next run.

The current workflow boundary is explicit:

- `sequential`: planner -> coder -> reviewer
- `parallel`: workflow -> spawned operational workers -> deterministic fan-in
- `loop`: generic loop-pattern workflow with bounded coder/reviewer iterations
- `review_critique`: coder/reviewer loop owned by a workflow agent
- `iterative_refinement`: official loop-pattern refinement flow with repeated coder/reviewer passes
- `coordinator`: workflow -> tactical manager -> operational workers
- `hierarchical`: workflow -> strategic director -> tactical manager -> operational workers

For `parallel` task input, the canonical contract is `input.parallel_branches = [{"name"?: string, "task": string}]`. CLI named branch syntax such as `Plan::Analyze architecture||Build::Implement change` is normalized into that shape before persistence and dispatch.

## Repository Layout

- `cmd/`: executable entrypoints
- `internal/app/`: runtime composition and dependency wiring
- `internal/domain/agent/`: agent roles and orchestration primitives
- `internal/usecase/dag/`: pipeline executor
- `internal/usecase/skills/`: skill engine, registry, compiler, sandbox
- `pkg/observability/`: traces, metrics, console rendering, OTEL setup
- `web-ui/`: event-stream dashboard
- `configs/`: local configuration examples

## Prerequisites

- Go `1.22+`
- Node.js `20+` for `web-ui`
- Ollama for local inference if you want the default local model path
- Optional Jaeger if you want to inspect traces visually

Recommended Ollama model:

```bash
ollama pull gemma3:270m
```

## Quick Start

1. Copy the sample configuration.

```bash
cp configs/config.example.yaml configs/config.yaml
```

2. Build the CLI.

```bash
go build -o aether cmd/aether/main.go
```

3. Run a single task.

```bash
./aether task "Design a Pet Store Agentic Salesperson AI"
```

4. Run an explicit parallel task with custom branches.

```bash
./aether task \
  -workflow=parallel \
  -parallel-branches="Plan::Analyze the current architecture||Build::Implement the runtime change||Verify::Write validation and rollout steps" \
  "Refactor the workflow runtime to support explicit parallel fan-out"
```

## Run the Daemon

The daemon exposes the live event stream and GitHub webhook endpoint.

```bash
AETHER_GITHUB_WEBHOOK_SECRET=change-me \
go run cmd/aetherd/main.go
```

Default endpoints:

- `GET http://localhost:8090/stream`
- `POST http://localhost:8090/webhooks/github`

You can override the daemon port with `AETHERD_PORT`.

Webhook notes:

- set `AETHER_GITHUB_WEBHOOK_SECRET` to enable `X-Hub-Signature-256` verification
- deliveries are deduplicated by `X-GitHub-Delivery` in SQLite, so replayed payloads do not create duplicate tasks
- interrupted daemon tasks are marked as `interrupted` on restart and can be retried from the control plane

## Run the Web UI

```bash
cd web-ui
npm install
VITE_AETHERD_URL=http://localhost:8090 npm run dev
```

If `VITE_AETHERD_URL` is not provided, the frontend falls back to `http://localhost:8090`.

When the selected workflow is `parallel`, the Web UI also lets you define explicit branch `name/task` pairs before submission.

## Run the Observability API

```bash
go run cmd/observability_api/main.go
```

Default endpoint:

- `http://localhost:8082`

You can override the port with `OBSERVABILITY_API_PORT`.

Useful routes:

- `GET /trace/:trace_id`
- `GET /trace/:trace_id/graph`
- `GET /org/:org_id/recent_traces`
- `GET /org/:org_id/metrics`

## Run a Pipeline

Example pipeline definition:

```bash
./aether pipeline run example.yaml --input '{"goal":"Design a task planner service"}'
```

The bundled `example.yaml` uses the `llm` skill with the `ollama` adapter.

## Skill Commands

List registered skills:

```bash
./aether skill list
```

Run the default LLM skill:

```bash
./aether skill run llm --input '{"prompt":"Summarize the current architecture."}'
```

Compile a source file into a WASM skill:

```bash
./aether skill compile --lang python --name demo_skill path/to/source.py
```

## Demo Sidecar

The repository also contains a small todo API under `cmd/todo_api`. It is a self-contained CRUD demo and not the primary Aether control plane.

## Development

Backend tests:

```bash
go test ./...
```

Frontend build:

```bash
cd web-ui
npm run build
```

Jaeger helper compose file:

```bash
docker compose -f deployments/docker-compose.observability.yml up -d
```

## Experience the Real Flow

Use these scripts when you want to exercise the project like a pre-release operator instead of just running unit tests.

Self-enhancement walkthrough:

```bash
bash scripts/self_enhancement_experience.sh
```

What it demonstrates:

- starts a fresh local daemon against an isolated SQLite runtime
- forces a deterministic contract failure so the runtime persists a reflection for `coder`
- shows the updated `strategies` row in SQLite
- submits a follow-up real task and verifies that it completes under the learned constraints

OTEL export walkthrough:

```bash
bash scripts/otel_export_rehearsal.sh
```

What it demonstrates:

- starts a local OTLP capture server without Docker
- runs the strict release smoke with `OTEL_EXPORTER_OTLP_ENDPOINT` enabled
- verifies that spans were exported for the `aether-core` service

Release acceptance walkthrough:

```bash
bash scripts/acceptance_release_readiness.sh
```

What it demonstrates:

- runs a strict user-visible release memo task through `review_critique`
- verifies a Google-style `hierarchical` orchestration path reaches completion through `strategic_director`, `tactical_manager`, and operational workers
- writes a reusable Markdown acceptance report to `dist/acceptance/ACCEPTANCE_REPORT.md`

Full local release rehearsal:

```bash
AETHER_RELEASE_GATE_RUN_OTEL_EXPORT_REHEARSAL=1 \
AETHER_RELEASE_GATE_RUN_DEPLOYMENT_REHEARSAL=1 \
bash scripts/release_gate.sh
```

What it demonstrates:

- backend tests, frontend build, and strict daemon E2E smoke
- local OTEL export wiring against the capture server
- binary-level deployment and rollback rehearsal from a staged release bundle

## Release Gate

Run the formal local delivery gate before external release:

```bash
bash scripts/preflight_local_env.sh
bash scripts/release_gate.sh
```

If you only want the environment and packaging readiness check:

```bash
bash scripts/preflight_local_env.sh
```

This gate validates:

- a best-effort preflight for the default local Ollama model `gemma3:270m` when the local Ollama endpoint is reachable
- `go test -count=1 ./...`
- `web-ui` production build
- the strict daemon E2E smoke in `scripts/release_smoke.sh`

Useful overrides:

- `AETHER_RELEASE_GATE_SKIP_FRONTEND=1` to skip the frontend build
- `AETHER_RELEASE_GATE_SKIP_SMOKE=1` to skip the daemon smoke
- `AETHER_RELEASE_GATE_SKIP_OLLAMA_CHECK=1` to bypass the local model availability check
- `AETHER_RELEASE_GATE_RUN_OTEL_EXPORT_REHEARSAL=1` to verify OTLP export with a local capture server
- `AETHER_RELEASE_GATE_RUN_DEPLOYMENT_REHEARSAL=1` to add binary-level deployment and rollback rehearsal
- `AETHER_RELEASE_GATE_RUN_ACCEPTANCE_SCENARIO=1` to add the release-quality plus hierarchical-architecture acceptance walkthrough
- `OTEL_EXPORTER_OTLP_ENDPOINT=...` to verify export wiring during the smoke run instead of running with OTEL export disabled by default

## Release Bundle

Build a local release bundle with binaries, frontend assets, manifest, and checksums:

```bash
bash scripts/build_release_bundle.sh
```

The default output path is `dist/release/v$(cat VERSION)`.

The bundle also includes operator-facing deployment assets under `deployments/`, including:

- `deployments/install_release.sh`
- `deployments/DELIVERY_HANDOFF.md`
- `deployments/RELEASE_CHECKLIST.md`
- `deployments/ROLLBACK_SOP.md`
- `deployments/systemd/aether-runtime.env.example`
- `deployments/systemd/aetherd.service`
- `deployments/systemd/aether-observability-api.service`
- `deployments/launchd/aether-runtime.env.example`
- `deployments/launchd/io.nikkofu.aetherd.plist`
- `deployments/launchd/io.nikkofu.aether-observability-api.plist`
- `deployments/docker-compose.observability.yml`

The bundle also includes:

- `scripts/acceptance_release_readiness.sh`
- `scripts/collect_release_evidence.sh`
- `scripts/preflight_local_env.sh`

For a standardized GitHub-hosted handoff artifact, run the `Aether-Release-Bundle` workflow from the Actions tab. It builds the bundle, runs the lightweight CI release gate, preflights the built artifact on the runner, and uploads `dist/release/vX.Y.Z` as a downloadable artifact.

If you are preparing a real GitHub release instead of only syncing `main`, first align `VERSION`, `CHANGELOG.md`, and the release line at the top of this README, then follow [`deployments/GITHUB_RELEASE_RUNBOOK.md`](deployments/GITHUB_RELEASE_RUNBOOK.md).

## Deployment Rehearsal

Run a binary-level deployment rehearsal, including rollback verification:

```bash
bash scripts/deployment_rehearsal.sh
```

This rehearsal:

- stages the current release bundle into a local deployment slot
- starts `aetherd` and `observability_api` from the staged binaries
- runs the strict smoke task against the deployed daemon
- verifies recent traces through the deployed observability API
- restarts the rollback slot and confirms the previous bundle can still boot against the same runtime database

## OTEL Export Rehearsal

Run a local OTLP export verification without Docker:

```bash
bash scripts/otel_export_rehearsal.sh
```

This rehearsal:

- starts a local OTLP capture server on `127.0.0.1:4317`
- runs the strict smoke task with `OTEL_EXPORTER_OTLP_ENDPOINT` enabled
- verifies that the runtime exported spans for the `aether-core` service

## Versioning

- Root release version is tracked in `VERSION`
- Human-readable release notes are in `CHANGELOG.md`
- Frontend package metadata is aligned to the same release line

## License

This project is released under the [MIT License](LICENSE).
