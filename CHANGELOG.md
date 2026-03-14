# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- GitHub webhook signature verification through `AETHER_GITHUB_WEBHOOK_SECRET`.
- SQLite-backed webhook delivery deduplication keyed by `X-GitHub-Delivery`.
- Startup recovery that marks unfinished daemon tasks as `interrupted` so they can be retried safely.

### Changed
- The runtime now implements seven explicit workflow agents: `workflow.sequential`, `workflow.parallel`, `workflow.loop`, `workflow.review_critique`, `workflow.iterative_refinement`, `workflow.coordinator`, and `workflow.hierarchical`, and daemon startup preloads all supported task workflows.
- `supervisor` is now restricted to alert/progress/reflection observation and no longer performs hidden task forwarding or self-healing retries.
- Strategic milestone execution now submits standard persisted tasks through the explicit `coordinator` workflow instead of publishing legacy `task` messages to `supervisor`.
- Dynamic worker spawning now subscribes spawned agents to the bus, and SQLite ledger initialization now migrates legacy schemas that predate `org_id`.
- Daemon task retry now naturally extends interrupted attempts instead of only terminal success/failure paths.
- Release documentation now separates the latest tagged release from unreleased `main` branch work and adds a maintainer runbook for GitHub publication.
- `aetherd` now defaults to port `8090`, and the Web UI fallback daemon URL now matches that default to reduce local port collisions.
- Hierarchical execution is now a real workflow path with explicit `workflow.hierarchical -> strategic_director -> tactical_manager -> operational` handoffs, and strategic planning no longer reuses a vision-only template for milestone decomposition.
- Tactical coordination state is now scoped by `task_id + milestone_id`, fixing state collisions when one task fans out into multiple concurrent milestones.
- Tactical aggregation now preserves subtask order and synthesizes multiple worker outputs into a single milestone deliverable before returning `coordination.result` or `milestone.feedback`.
- Goal aggregation now preserves milestone planning order so hierarchical final reports stay deterministic even when milestones finish out of order.
- `iterative_refinement` is now a first-class workflow pattern with its own workflow agent, executor, policy, CLI flag support, and Web UI option instead of remaining an unimplemented enum value.
- `loop` is now a first-class workflow pattern, and the three loop-style workflows now share one underlying loop engine instead of duplicating the same review/revise state machine three times.
- `parallel` is now a first-class workflow pattern with its own workflow agent, executor, policy, CLI flag support, and Web UI option instead of remaining an enum without an explicit runtime path.
- Parallel fan-out no longer needs to hide behind coordinator-style decomposition. `workflow.parallel` now dispatches spawned `operational` workers directly and performs deterministic fan-in on branch completion.
- Task submission now rejects non-taxonomy workflow enums such as `swarm`, `react`, and `custom_logic` at validation time instead of exposing them as valid-but-unimplemented task modes.
- CLI and Web UI task submission now support custom `parallel` branch definitions, making the explicit fan-out plan configurable instead of relying only on the workflow's built-in default branches.
- Custom `parallel` branches can now preserve explicit branch names end-to-end, so final fan-in reports and task events no longer depend on deriving labels from free-form task text.
- `parallel` branch input is now normalized once in the task domain, so CLI text syntax, API payloads, persisted SQLite task input, and `workflow.parallel` all share the same canonical `parallel_branches` schema.

## [1.8.1] - 2026-03-11

### Changed
- Rewrote the root `README.md` to describe the repository as it runs today, including CLI, daemon, Web UI, observability API, pipeline runner, and sidecar demos.
- Expanded `ARCHITECTURE.md` from a slogan-level note into an implementation-oriented overview of runtime composition, execution paths, storage, messaging, and observability.
- Replaced the frontend template `README` with actual project-specific usage instructions.
- Updated the bundled pipeline example to use the `ollama` adapter instead of the removed Gemini-default path.
- Aligned the release line across root documentation, `VERSION`, and `web-ui/package.json`.

### Fixed
- `web-ui` now reads the daemon base URL from `VITE_AETHERD_URL`, with a local daemon fallback when the variable is not set.
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
