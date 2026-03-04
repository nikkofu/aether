# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.3.0-alpha] - 2026-03-02

### Added
- **Long-term Engineering Memory (Lite RAG)**:
  - Implemented `Search` interface in `KnowledgeGraph` for semantic-ish experience retrieval.
  - `PlannerAgent` now automatically queries past task `Reflections` to avoid repeating historical mistakes.
  - `SupervisorAgent` automatically persists "Lessons Learned" into the graph after every task completion.
  - Closed the "Execution -> Reflection -> Learning -> Guidance" autonomous loop.

## [1.2.0-alpha] - 2026-03-02

### Added
- **Event-Driven Auto-Healing Daemon (`aetherd`)**:
  - Introduced a standalone background daemon process for Aether.
  - Implemented GitHub Webhook listener (`/webhooks/github`) to capture `issues` events.
  - Automatically translates external bug reports into actionable DAG task payloads (`system.spawn`) without human intervention, transforming Aether into an always-on AI team.

## [1.1.0-alpha] - 2026-03-02

### Added
- **Local LLM Support (Ollama)**:
  - Added native adapter for Ollama, enabling zero-cost, fully private offline agent execution.
  - Introduced new configuration block `[ollama]` with customizable `base_url`, `model`, and `temperature`.
  - Automatically falls back to local models when configured alongside OpenAI/Gemini.
- **DAG Visualization**:
  - Implemented `ToMermaid()` method for `Pipeline`.
  - Automatically exports task dependencies and nodes into standard Mermaid.js `graph TD` format for enhanced observability and debugging.

## [1.0.0-rc1] - 2026-03-02

### Added
- **Dynamic WASM Skill Loading**:
  - `SQLiteSkillEngine` now supports fetching WASM code from remote URLs.
  - Automated caching and atomic updates for WASM binary management.
  - Integrated `WASMExecutor` for secure, sandboxed skill execution.
- **DAO-like Governance**:
  - Enhanced `GovernanceBoard` with automated policy execution.
  - Proposals can now dynamically update the `Policy Engine` rules upon passing.
  - Added `UpdateRule` to `Policy` interface for real-time regulation.
- **Project Structure**:
  - Fully migrated to a **Top-tier Enterprise Layout** (Clean Architecture).
  - Categorized code into `domain`, `usecase`, `infrastructure`, `app`, and `pkg`.

### Changed
- Moved core entities (`Skill`, `SkillVersion`, `SkillEngine`) to `internal/domain/capability/skills` for architectural integrity.
- Refactored all internal packages to resolve circular dependencies and improve maintainability.

### Fixed
- Fixed nil pointer dereference in `LLMSkill` when `strategyStore` is not provided.
- Fixed incorrect interface and type references across `usecase` layers.

## [0.2.0-alpha] - 2026-03-02

### Added
- **Distributed Scheduling (NATS based)**:
  - Enhanced `Bus` interface with `SubscribeToSubject` for flexible message routing.
  - Implemented `NATSBus` support for cluster-wide communication.
  - Enhanced `Scheduler` with heartbeat-based worker registration and selection.
  - Updated `AgentManager` to support distributed task dispatching via NATS.
  - Support for `cluster-leader` and `cluster-worker` modes in `Runtime`.
  - Heartbeat mechanism for node health monitoring.

### Changed
- Refactored `NewDefaultRuntime` to handle cluster configuration.
- Improved `MemoryBus` to support subject-based subscriptions.

### Fixed
- Fixed various syntax errors in `internal/app/runtime.go`.
- Fixed unused import in `pkg/observability/metrics/calculator.go`.

## [0.1.0-alpha] - 2026-03-02

### Added
- **Core Architecture**: Initial multi-agent skeleton with `Manager`, `Planner`, `Coder`, `Reviewer`, and `Supervisor`.
- **Capabilities**: Native support for Email, File, Calendar, Alarm, Search, and Registry.
- **Observability**: Integrated Tracing and Metrics system with SQLite storage and Grafana-ready APIs.
- **Bus**: Event-driven communication supporting both in-memory and NATS.
- **Web UI**: Modern React-based dashboard for managing agents and monitoring metrics.
- **Governance**: Basic policy engine and evolution guard mechanisms.
- **DAG Runtime**: Foundation for task dependency management and execution.

---

[1.1.0-alpha]: https://github.com/nikkofu/aether/releases/tag/v1.1.0-alpha
[1.0.0-rc1]: https://github.com/nikkofu/aether/releases/tag/v1.0.0-rc1
[0.2.0-alpha]: https://github.com/nikkofu/aether/releases/tag/v0.2.0-alpha
[0.1.0-alpha]: https://github.com/nikkofu/aether/releases/tag/v0.1.0-alpha
