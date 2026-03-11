# Aether

Current release: `v1.8.1`  
Release date: `2026-03-11`

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

- Multi-agent task loop with `supervisor -> planner -> coder -> reviewer`
- Streaming token feedback in the CLI
- SQLite-backed memory, audit, ledger, knowledge graph, and trace storage
- Pipeline execution from YAML DAG definitions
- Skill compilation and WASM execution infrastructure
- Webhook and SSE daemon for event intake and live monitoring

## Current Status

The repository is usable as an experimental multi-agent runtime. The main CLI path is the most complete product surface today.

The daemon, Web UI, strategy engine, cluster scheduling, and skill/WASM layers are available, but parts of those areas are still evolving. Read [ARCHITECTURE.md](ARCHITECTURE.md) before extending the system.

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
ollama pull qwen3.5:0.8b
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

## Run the Daemon

The daemon exposes the live event stream and GitHub webhook endpoint.

```bash
AETHER_GITHUB_WEBHOOK_SECRET=change-me \
go run cmd/aetherd/main.go
```

Default endpoints:

- `GET http://localhost:8080/stream`
- `POST http://localhost:8080/webhooks/github`

You can override the daemon port with `AETHERD_PORT`.

Webhook notes:

- set `AETHER_GITHUB_WEBHOOK_SECRET` to enable `X-Hub-Signature-256` verification
- deliveries are deduplicated by `X-GitHub-Delivery` in SQLite, so replayed payloads do not create duplicate tasks
- interrupted daemon tasks are marked as `interrupted` on restart and can be retried from the control plane

## Run the Web UI

```bash
cd web-ui
npm install
VITE_AETHERD_URL=http://localhost:8080 npm run dev
```

If `VITE_AETHERD_URL` is not provided, the frontend falls back to `http://localhost:8080`.

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

## Versioning

- Root release version is tracked in `VERSION`
- Human-readable release notes are in `CHANGELOG.md`
- Frontend package metadata is aligned to the same release line

## License

This project is released under the [MIT License](LICENSE).
