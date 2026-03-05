# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.8.0-rc1] - 2026-03-05

### Added
- **Enterprise-Grade Observability (Jaeger/OTel Integration)**:
  - Full OpenTelemetry (OTel) instrumentation for all Agent and Skill executions.
  - Automatic Trace-Log correlation: Every `Zap` log now includes the current `trace_id` and `span_id`.
  - Rich Span Attributes: Captured full input/output snapshots (JSON) in Jaeger for all LLM and DAG nodes.
- **Real-time Stream Feedback (Typewriter Effect)**:
  - Implemented a unified asynchronous token broadcast system via `Bus`.
  - `Aether CLI` now supports real-time, zero-latency streaming feedback from Ollama/OpenAI.
- **Agentic Self-Healing & Robustness**:
  - Refactored `OllamaAdapter` to use `json.Decoder` for non-blocking stream parsing, resolving `Scanner` hang issues.
  - Implemented `ProtectedHandle` in `BaseAgent` with panic recovery and fault-injection tracing.
  - Added "First Byte" pre-warming logic to CLI task execution for high-reliability startup.
- **Command-Line Parametrization**:
  - Introduced `aether task "<description>"` for one-shot, autonomous goal execution without YAML.
  - Supported `--input '{"key": "val"}'` for dynamic pipeline variable injection.

### Changed
- **Config Schema Unified**: Refactored `Config` struct and `config.yaml` to remove deep nesting and hardcoded model fallbacks.
- **Bootstrap Logic**: `Runtime` now enforces strict model selection, eliminating silent 404 failures.

### Fixed
- Fixed nil pointer dereferences in `PlannerAgent` when `Tracer` or `Manager` was partially initialized.
- Resolved distributed race condition in `MemoryBus` where subscriptions could lag behind publications.

... (rest of the file)
