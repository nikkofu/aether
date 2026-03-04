# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0-alpha] - 2026-03-02

### Added
- **Core Architecture**: Initial multi-agent skeleton with `Manager`, `Planner`, `Coder`, `Reviewer`, and `Supervisor`.
- **Capabilities**: Native support for Email, File, Calendar, Alarm, Search, and Registry.
- **Observability**: Integrated Tracing and Metrics system with SQLite storage and Grafana-ready APIs.
- **Bus**: Event-driven communication supporting both in-memory and NATS.
- **Web UI**: Modern React-based dashboard for managing agents and monitoring metrics.
- **Governance**: Basic policy engine and evolution guard mechanisms.
- **DAG Runtime**: Foundation for task dependency management and execution.

### Changed
- Refactored `internal/observability` to support cleaner database queries.

### Fixed
- Fixed unused import in `internal/observability/metrics/calculator.go`.

---

[0.1.0-alpha]: https://github.com/nikkofu/aether/releases/tag/v0.1.0-alpha
