# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
...
