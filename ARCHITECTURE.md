# Architecture

## Overview

Aether is a layered Go system that combines:

- a runtime container that wires storage, bus, policies, tracing, and capabilities
- a set of agent roles that communicate through messages
- a DAG pipeline executor for deterministic workflow execution
- optional daemon and Web UI surfaces for event streaming and monitoring

The codebase currently exposes two main execution models:

- agent-driven execution through the CLI and daemon
- DAG-driven execution through `aether pipeline run`

## Design Rule

LLMs do not directly perform side effects. They produce plans or outputs that still flow through the runtime:

`decision -> policy -> execution -> observation`

This separation is reflected in the package structure:

- `internal/domain/`: roles, entities, policies, knowledge, governance
- `internal/usecase/`: orchestration logic such as DAGs, skills, scheduler, learning
- `internal/infrastructure/`: external adapters and concrete capability implementations
- `pkg/`: cross-cutting concerns such as config, bus, observability, logging, RBAC

## Runtime Composition

`internal/app/runtime.go` is the central composition root.

The runtime initializes:

- configuration
- SQLite-backed stores when a database path is configured
- the message bus, either memory bus or NATS
- the capability registry and capability gateway
- the LLM adapter set and the `llm` skill
- the agent manager and pre-registered roles
- tracing, metrics, audit, RBAC, scheduler, knowledge graph, and ledger

## Execution Surfaces

### 1. CLI

`cmd/aether` is the most complete entrypoint today.

It supports:

- `task`
- `pipeline`
- `skill`
- `strategic`
- `knowledge`
- `export`

The typical interactive task path is:

1. CLI subscribes to the `cli` subject for streamed tokens and final reports.
2. CLI starts the runtime agents.
3. CLI publishes a `task` message to `supervisor`.
4. Supervisor forwards planning work to `planner`.
5. Planner calls the `llm` skill and emits an `instruction` for `coder`.
6. Coder produces code or implementation text and sends a `review_request`.
7. Reviewer evaluates the output and returns `review_result`.
8. Coder or supervisor emits `final_report` back to the CLI.

### 2. Daemon

`cmd/aetherd` starts the runtime as a long-lived process.

Current public surfaces:

- GitHub webhook intake
- SSE event stream at `/stream`

The daemon is the natural place to evolve a future control plane API.

### 3. Pipeline Executor

`internal/usecase/dag` runs YAML-defined DAGs where each node calls a registered capability.

Properties:

- dependency-aware parallel execution
- policy evaluation before capability execution
- per-node events for progress reporting
- memory persistence of outputs

This path is better suited to deterministic automation than the conversational agent loop.

## Messaging Model

The default local transport is `pkg/bus.MemoryBus`.

Key characteristics:

- non-blocking in-memory queue
- direct agent subscriptions
- subject-based side subscriptions for UI or infrastructure consumers
- panic isolation around agent handlers
- supervisor-facing alert propagation on handler failures

This allows the same underlying messages to feed:

- orchestration
- CLI streaming
- SSE broadcasting
- scheduler heartbeat processing

## Capabilities and Skills

The system distinguishes between:

- capabilities: executable units registered in the capability registry
- skills: higher-level abstractions, including the default `llm` skill

Relevant implementations:

- `internal/domain/capability/skills/llm.go`
- `internal/usecase/skills/engine/compiler.go`
- `internal/usecase/skills/sandbox/wasm_executor.go`

The `llm` skill is the core shared primitive used by planner, coder, reviewer, and strategic components.

## Storage

When SQLite is enabled, the runtime stores:

- memory records
- traces
- metrics
- issues
- reflection output
- strategies
- strategic goals and milestones
- knowledge graph entities and relations
- ledger entries
- audit logs
- RBAC state

This makes SQLite the default local control-plane database.

## Observability

Observability is a first-class concern in the codebase.

Instrumentation includes:

- OpenTelemetry spans
- SQLite-backed trace persistence
- console rendering for CLI feedback
- metrics calculation APIs
- SSE event broadcasting for the frontend

The observability stack is split across:

- `pkg/observability/`
- `cmd/observability_api`
- `internal/delivery/api/stream.go`

## Organizational and Strategic Layer

The repository also includes a higher-level organizational model:

- `vision_board`
- `strategic_director`
- `tactical_manager`
- `operational` workers

This layer is intended to support milestone-based execution and longer-running goals. It is present in the runtime, but parts of the result-handling loop are still evolving compared to the simpler CLI task path.

## Deployment Model

Typical local development layout:

- `aether` CLI for direct task execution
- `aetherd` on `:8080` for streaming and webhook intake
- `observability_api` on `:8082` for trace and metrics queries
- `web-ui` during local frontend development
- optional Jaeger via `deployments/docker-compose.observability.yml`

## Practical Boundary

If you are extending the system, treat these areas as the stable base:

- runtime composition
- bus and agent message flow
- CLI task execution
- DAG pipeline execution
- LLM skill abstraction
- SQLite-backed observability

Treat these areas as active iteration surfaces:

- webhook-driven autonomous task intake
- cluster scheduling and bidding
- strategy and organization result loops
- WASM skill productization
- Web UI control-plane features
