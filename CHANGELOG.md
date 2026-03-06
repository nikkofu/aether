# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.8.0] - 2026-03-05

### Added
- **ReAct Reasoning Paradigm**: All core agents (Planner, Coder, Reviewer) now follow the Thought -> Action -> Observation cycle for superior logic and reliability.
- **Visual Matrix CLI**: Implemented full ANSI color support for real-time typewriter feedback. Each agent role now has a distinct visual identity.
- **Autonomous Orchestration**: Supervisor now monitors the entire bus to facilitate seamless task handovers between roles without manual intervention.
- **Graceful Lifecycle Management**: CLI now automatically flushes traces and exits cleanly upon task completion.
- **Robustness Overhaul**: Eliminated nil pointer risks in agent handlers and refactored stream parsing using `json.Decoder` for zero-hang execution.

### Changed
- **Messaging Alignment**: Unified all CLI feedback to the `cli` subject for high-precision message routing.
- **Pre-warming Logic**: Introduced a cold-start buffer to ensure Ollama models and NATS/MemoryBus are fully ready before task dispatch.

## [1.7.0-alpha] - 2026-03-02
... (rest of previous logs)
