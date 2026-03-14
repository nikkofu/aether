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

For agentic task execution, Aether now intentionally follows explicit workflow patterns that map to the Google Cloud agentic design-pattern taxonomy. A workflow pattern is considered implemented only when it has a named workflow agent, executor, transition policy, and task-surface support.

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
2. CLI starts the runtime agents for the selected workflow pattern.
3. Task submission is normalized by `TaskService`, persisted, and dispatched through a workflow executor.
   For example, `parallel` submissions are canonicalized into `input.parallel_branches = [{"name"?: string, "task": string}]` before they reach the workflow agent, so CLI, API, daemon, and storage layers all operate on the same task contract.
4. `workflow.sequential`, `workflow.parallel`, `workflow.loop`, `workflow.review_critique`, `workflow.iterative_refinement`, `workflow.coordinator`, or `workflow.hierarchical` receives the start message and explicitly drives the next step.
5. Sequential and loop-driven flows use planner, coder, and reviewer through workflow-owned message contracts. `loop`, `review_critique`, and `iterative_refinement` all run on the same bounded loop engine, while still exposing distinct workflow identities at the task boundary.
6. Parallel flows use hardcoded fan-out and fan-in rules in `workflow.parallel`. The workflow agent spawns `operational` workers directly, dispatches predefined branches concurrently, and deterministically aggregates branch outputs without relying on a hidden supervisor or LLM-selected routing path.
7. Coordinator flows delegate milestone execution through `tactical_manager` and dynamically spawned `operational` workers. `tactical_manager` now owns the fan-out/fan-in boundary and synthesizes worker outputs into one milestone deliverable before routing the result back to `workflow.coordinator`.
8. Hierarchical flows delegate the top-level goal to `strategic_director`, which decomposes milestones, aggregates milestone feedback in planned order, and routes the final `goal.result` back to `workflow.hierarchical`.
9. The workflow agent emits `final_report` or `system.alert`, and task state is updated by the transition policy registry.

### 2. Daemon

`cmd/aetherd` starts the runtime as a long-lived process.

Current public surfaces:

- GitHub webhook intake
- SSE event stream at `/stream`

The daemon now also owns:

- task persistence and control-plane APIs
- webhook HMAC verification and delivery deduplication
- startup recovery that marks unfinished tasks as `interrupted`

The daemon is the natural place to evolve a future control plane API.

### 2.1 Task-Level Self-Enhancement

Aether now has a deterministic self-enhancement loop at the task boundary:

1. A task reaches a terminal state through an explicit workflow agent such as `workflow.review_critique`.
2. `TaskService` loads the ordered task event history.
3. A task-outcome observer synthesizes structured reflections for `planner`, `coder`, or `reviewer` from real runtime signals such as:
   - deterministic output-contract violations
   - reviewer protocol failures
   - fallback decision paths
   - planner plan-format failures
4. Reflections are persisted to SQLite and mirrored into the knowledge graph.
5. The learning engine updates the shared strategy store, which injects learned operating rules into the next LLM call for that agent.

This keeps self-improvement inside the same explicit Google-style workflow boundary rather than hiding adaptation inside a supervisor or an untracked post-processor.

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
- direct agent subscriptions with explicit `To` routing
- subject-based side subscriptions for UI or infrastructure consumers
- panic isolation around agent handlers
- supervisor-facing alert propagation on handler failures without hidden orchestration privileges

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

The task-level self-enhancement loop depends on this storage layout: terminal task events stay in `task_events`, synthesized reflections are written to `reflections`, and learned agent instructions are persisted in `strategies`.

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

This layer is intended to support milestone-based execution and longer-running goals. Strategic milestone execution now enters the same task control plane as CLI and daemon work through the explicit `coordinator` workflow, instead of publishing legacy `task -> supervisor` messages directly onto the bus.

The hierarchy now has two explicit contracts:

- `workflow.hierarchical -> strategic_director` for goal decomposition and final goal aggregation
- `strategic_director -> tactical_manager -> operational` for milestone decomposition and execution

This matters because state is no longer inferred from hidden supervisor behavior or a single shared tactical context. Goal-level and milestone-level coordination now live in separate, named agents with explicit return paths.

Parallel execution has its own separate contract:

- `workflow.parallel -> operational` for concurrent branch execution
- `operational -> workflow.parallel` for deterministic branch completion and fan-in

This keeps the base parallel pattern distinct from `coordinator` and `hierarchical`, which both add extra organizational decomposition layers on top of fan-out/fan-in.

Supervisor remains in the runtime for:

- alert aggregation
- progress observation
- reflection capture

It is no longer the hidden default orchestrator for `sequential` or `review_critique` task execution.

## Deployment Model

Typical local development layout:

- `aether` CLI for direct task execution
- `aetherd` on `:8090` for streaming and webhook intake
- `observability_api` on `:8082` for trace and metrics queries
- `web-ui` during local frontend development
- optional Jaeger via `deployments/docker-compose.observability.yml`
- `scripts/release_gate.sh` as the formal local release gate
- `scripts/release_smoke.sh` as the strict daemon E2E smoke task
- `scripts/otel_export_rehearsal.sh` as the local OTLP export verification step
- `scripts/build_release_bundle.sh` as the local artifact-packaging step
- `scripts/deployment_rehearsal.sh` as the binary-level deploy and rollback drill

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
