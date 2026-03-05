# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.8.0-rc1] - 2026-03-05

### Added
- **Full-Stack Observability**: Integrated OpenTelemetry for end-to-end agentic workflow tracing.
- **Trace-Log Correlation**: Automatic injection of TraceID and SpanID into structured logs (Zap).
- **Asynchronous Token Stream**: High-performance token broadcasting via Bus for real-time CLI feedback.
- **Robustness & Self-Healing**: Implemented panic recovery and JSON-based fault injection analysis in Agent handlers.
- **Native Ollama Optimization**: Enhanced streaming performance via `json.Decoder` based chunk parsing.

### Changed
- **Unified Configuration**: Flattened configuration schema for better maintainability and environment parity.
- **Bootstrap Hardening**: Enforced strict capability registration to prevent silent initialization failures.

### Fixed
- Fixed nil pointer dereferences in `PlannerAgent` when `Tracer` or `Manager` was partially initialized.
- Resolved distributed race condition in `MemoryBus` where subscriptions could lag behind publications.

... (rest of the file)
