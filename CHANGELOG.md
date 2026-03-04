# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
- Fixed various syntax errors in `internal/runtime/runtime.go`.
- Fixed unused import in `internal/observability/metrics/calculator.go`.

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

[0.2.0-alpha]: https://github.com/nikkofu/aether/releases/tag/v0.2.0-alpha
[0.1.0-alpha]: https://github.com/nikkofu/aether/releases/tag/v0.1.0-alpha
