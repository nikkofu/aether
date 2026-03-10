# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.8.1] - 2026-03-11

### Changed
- Rewrote the root `README.md` to describe the repository as it runs today, including CLI, daemon, Web UI, observability API, pipeline runner, and sidecar demos.
- Expanded `ARCHITECTURE.md` from a slogan-level note into an implementation-oriented overview of runtime composition, execution paths, storage, messaging, and observability.
- Replaced the frontend template `README` with actual project-specific usage instructions.
- Updated the bundled pipeline example to use the `ollama` adapter instead of the removed Gemini-default path.
- Aligned the release line across root documentation, `VERSION`, and `web-ui/package.json`.

### Fixed
- `web-ui` now reads the daemon base URL from `VITE_AETHERD_URL`, with `http://localhost:8080` as the default.
- `observability_api` now defaults to port `8082` and supports `OBSERVABILITY_API_PORT`, removing the default port collision with `aetherd`.
- Corrected outdated todo demo documentation and fixed the backend test path in its README.
- Repaired `LLMSkill` unit tests after constructor signature and streaming behavior changes.

## [1.8.0] - 2026-03-05

### Added
- ReAct reasoning flow for planner, coder, and reviewer roles.
- ANSI-colored streaming CLI feedback for agent tokens.
- Supervisor-driven autonomous orchestration across the local bus.
- Graceful lifecycle handling around task completion and trace flushing.
- Hardening around panic recovery and stream parsing.

### Changed
- Unified CLI feedback routing on the `cli` subject.
- Added cold-start buffering before task dispatch.

Historical alpha milestones prior to `v1.8.0` are preserved in Git history and tags.
