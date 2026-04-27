# Changelog

## v0.10.0 — 2026-04-27

### Added
- `ws-named-agent` — unified Python CLI entry point for named agent management (subcommands: `new`, `call`, `interrupt`, `print`, `check-mailbox`, `tail`, `override`); multi-backend routing (claude, codex, gemini stub); replaces the standalone bash scripts which are now one-liner shims
- `ws-named-agent tail` — reads last N assistant turns from the live session JSONL on disk (safe while agent is running); supports claude and codex session formats
- `ws-named-agent override` — persists tier-to-model overrides in `~/.claude/<repo>-ws.json`; resolution order: `WS_OVERRIDE_<TIER>` env var > config file > stored model
- codex backend — full feature parity with claude backend: session routing via `~/.codex/sessions`, `PostToolUse` hook for outbox drain, system prompt via `model_instructions_file`, compression handoff
- `ai-docs/ref/codex-integration.md` — probed codex CLI behavior reference (invocation, JSONL format, session management, hook config, model flag behavior, PATH inheritance)

### Changed
- `ws-call-named-agent`, `ws-new-named-agent`, `ws-interrupt-named-agent`, `ws-print-named-agent-output`, `ws-agent-check-mailbox` — converted to one-liner shims delegating to `ws-named-agent`
- Model resolution: frontmatter `model:` field read at `ws-named-agent new` to set initial tier; `--model` overrides frontmatter; backend shorthands (`claude`, `codex`, `gemini`) stored as model but never passed as `--model` to their CLIs
- `/sprint` Delegation Cycle — `ws-call-named-agent` calls use `run_in_background: true`
- `/implement` — feature-branch auto-merge mode reverted; approval gate now unconditional

### Fixed
- `ws-call-named-agent` — replaced path construction with `find` for Windows Git Bash portability
- `ws-agent-check-mailbox` hook path — now absolute so it resolves correctly regardless of hook shell's working directory

## v0.9.0 — 2026-04-27

### Added
- `claude-dash` — Rust PTY TUI multiplexer for worktree-scoped Claude sessions: tabbed interface, named-agent panel, prefix-key bindings (`<prefix>+q/w/e/r…`), mouse navigation, `--dangerously-skip-permissions` flag, and `claude --worktree` tab spawning

### Changed
- `ws-call-named-agent` — retry-with-backoff on "Session already in use"; carry pending interrupts across compaction handoff
- `ws-interrupt-named-agent` — removed dead argument-count guard branch

## v0.8.0 — 2026-04-27

### Added
- `ws-interrupt-named-agent` — queue a mid-task message into a named agent's outbox; PostToolBatch hook (`ws-agent-check-mailbox`) stops the agent at the next tool boundary; `ws-call-named-agent` drain loop delivers the message on resume
- `ws-agent-check-mailbox` — PostToolBatch hook script used internally by `ws-call-named-agent` to stop running agents when outbox is non-empty

### Changed
- `ws-call-named-agent` — hook settings now inlined as raw JSON via `--settings` flag (eliminates per-agent settings.json temp files); applies to all call paths including compression handoff
- `claude-watch` session discovery — also scans `~/.claude/projects/` for subdirectory names starting with the escaped main worktree path, surfacing sub-project sessions (e.g. `tools/claude-dash`)
- `/sprint` branch naming — infers a kebab-case slug from context when topic is clear; falls back to random `<adjective>-<noun>-<noun>` name; never prompts the user
- `/sprint` wrap-up discipline — each doc updater (spec-updater, mental-model-updater) commits its output immediately after completion; no batching

## v0.7.0 — 2026-04-24

### Added
- `ws-call-agent` — `claude -p` wrapper with permission bypass; `--agent` flag for deterministic UUID sessions (create-or-resume), `--session-id`, `--uuid`, `--system-prompt`
- `ws-agent` — deterministic UUID v5 from repo-root + branch + name
- `ws-declare-agent` — clears session files to scope agent slots to a run
- `ws-call-agent` formatted output: `[info]`/`[warn]` context window line (shown when `--agent` used and fill ≥50%; warn at ≥70% of 150K)

### Changed
- `ws:implement` internal orchestration rewritten — `TeamCreate`/`SendMessage`/`TeamDelete` replaced with `ws-call-agent`/`ws-declare-agent`
- `ws-*` scripts moved from `claude/infra/` to `claude/bin/` (PATH-accessible)

### Removed
- `/parallel-implement` skill — split-scope work now handled via split tickets + `/implement`
- `/team-lead` skill — no longer needed without `TeamCreate` machinery

## v0.6.0 — 2026-04-24

### Added

- `/add-rule` skill: classify an incoming rule as cross-cutting (→ `CLAUDE.md ## Architecture Rules`) or domain-scoped (→ `ai-docs/mental-model/<domain>.md ## Domain Rules`). Autonomous when clear, interactive when ambiguous. Append-only — never modifies existing rules.
- Ship config (`ai-docs/ship/ws.md`): version strategy, CHANGELOG procedure, pre-flight, tag, and push steps for the `ws` plugin.

### Changed

- **2-layer Architecture Rules split**: `CLAUDE.md ## Architecture Rules` is now scoped to cross-cutting invariants only. Domain-scoped rules belong in `ai-docs/mental-model/<domain>.md ## Domain Rules`.
- `mental-model-conventions.md`: added `## Directory Hierarchy` (flat vs `<domain>/index.md` + children), ancestor loading invariant, and `## Domain Rules` section with authorship and modification constraints.
- `ai-docs/mental-model.md`: updated index with directory hierarchy and Domain Rules sections.
- `claude/bin/list-mental-model`: rewritten with tree output (`├─`/`└─` glyphs); ancestor `index.md` auto-emitted alongside matching direct-child sub-domain in filtered mode.
- `mental-model-updater`: gains `/forge-mental-model` authority — creates new domain docs and splits flat docs to `<domain>/index.md` + children when diff shows code-structure change. Domain Rules promotion-only (upward during splits, never downward, never content-modified). Stale rules flagged in `## Stale Rules` output block; never edited autonomously.
- `executor-wrapup.md`: added `§Ancestor Loading` contract (3-step procedure) and Invariant bullet for one-level hierarchy reads.
- `edit`, `implement`, `parallel-implement` skills: propagate ancestor-loading contract to Invariants and spawn prompts, bounded to one-level hierarchies (`<domain>/<sub>.md` only).
- `CLAUDE.template.md` v0028: tightened `## Architecture Rules` inclusion test (explicitly excludes domain-scoped rules; directs authors to `/add-rule`); added v0028 migration item (reclassify existing Architecture Rules entries that are domain-scoped).
- `_index.md`: stale per-commit version-bump rule removed (ship config is now the authority); `/add-rule` added to skill inventory.

## v0.5.0 — 2026-04-24

### Added

- `project-survey` agent: pre-invocation context survey; returns `[Must|Maybe]`-tiered spec/mental-model/ticket reference list for a given brief.
- Auto-invoke integration: `edit`, `implement`, `parallel-implement`, `discuss` each spawn `project-survey` at step 0.
