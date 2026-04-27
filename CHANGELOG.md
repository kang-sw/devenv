# Changelog

## v0.11.1 — 2026-04-28

### Fixed
- `ws-named-agent`: CLAUDE.md injection moved from claude backend to codex backend — claude CLI reads CLAUDE.md natively; codex does not, so injecting it into `model_instructions_file` is required for codex agents to observe project behavioral constraints

## v0.11.0 — 2026-04-28

### Added
- `/write-code` skill — new delegated-implementation primitive: brief → judge: plan-depth (as-is/survey/research) → implementer named-agent → 3-reviewer loop (correctness, fit, test) with won't-fix disposition system and 3-cycle cap with lead adjudication at cycle 2

### Changed
- `/edit` recast as direct-edit primitive: lead edits directly, one named-agent reviewer covering correctness+fit (temp-file concatenation), 2-cycle relay cap, self-cleanup, no doc pipeline
- `/implement` recast as harness: `judge: execution-mode` routes to `ws:edit` or `ws:write-code`; doc pre-pass (spec-updater then mental-model-updater, each committed separately); approval gate; merge
- `/sprint` routing table updated: calls `ws:edit` | `ws:write-code` directly; Delegation Cycle template removed; wrap-up auto-merges via `ws-merge-branch`
- `/proceed` simplified: always routes to `/implement`; `judge: direct-edit` and `judge: execution-mode` removed (now owned by `/implement`)

### Removed
- `/write-plan` skill — brief writing and `judge: plan-depth` absorbed into `/implement`; plan-populator infra docs moved to `claude/infra/`

## v0.10.6 — 2026-04-27

### Fixed
- `ws-named-agent` (compression): replaced `ws-infra-path` subprocess call with direct `PLUGIN_DIR / "infra" / "agent-compression.md"` read in both claude and codex backends; on Windows, Git Bash's `pwd` returns a POSIX path (`/c/Users/...`) that Python's `pathlib` cannot resolve, causing `FileNotFoundError` at every compression handoff

## v0.10.5 — 2026-04-27

### Fixed
- `ws-named-agent`: added `_WIN_NO_WINDOW = getattr(subprocess, "CREATE_NO_WINDOW", 0)` and `_subrun()` wrapper; all 11 `subprocess.run` call sites replaced — suppresses the brief console window that appears per-spawn for `.cmd` shims on Windows
- `ws-named-agent tail` (codex): added `last_agent_message` fallback in `task_complete` handler — when no inline `agent_message` events are present in a turn, the final message is read from `payload.last_agent_message`; also added explicit `uuid`/`pattern`/`search-dir` diagnostics when no session file is found

## v0.10.4 — 2026-04-27

### Added
- `.cmd` shims for all 16 scripts in `claude/bin/` — Python scripts call `python "%~dp0<name>" %*`; bash scripts call `bash "%~dp0<name>" %*`; cmd.exe selects `.cmd` via PATHEXT, Git Bash selects the shebang file; no conflict
- `ws-named-agent`: `_inject_git_bash()` runs at module load on Windows — reads Git installation path from `HKLM\SOFTWARE\GitForWindows` registry key, falls back to `C:\Program Files\Git\bin`; injects into `PATH` so all subsequent subprocess calls (including hooks and `.cmd` shims) can resolve `bash`

### Fixed
- `ws-named-agent override`: local config path moved from `<git-root>/.claude/kang-sw-devenv-ws.json` to `~/.claude/kang-sw-devenv/ws/<escaped-proj>.json`; eliminates git tracking of machine-local config

## v0.10.3 — 2026-04-27

### Added
- `ws-named-agent override [-g]` — two-layer config: global (`~/.claude/kang-sw-devenv-ws.json`) and local (`<git-root>/.claude/kang-sw-devenv-ws.json`); `-g` writes to global, default writes to local; `override show` displays both layers separately; local wins on merge conflict

### Fixed
- `.gitignore`: added `.claude/kang-sw-devenv-ws.json` (local config is per-machine, not committed)

## v0.10.2 — 2026-04-27

### Added
- `ws-named-agent override default <model>` — sets a per-repo fallback model in the config file; used by `ws-named-agent new` when no `--model`, agent frontmatter, or `--agent-type` is present. Config file only — `WS_OVERRIDE_*` env vars are not involved.

### Fixed
- `ws-named-agent`: all `Path.read_text()` / `Path.write_text()` calls now pass `encoding='utf-8'` explicitly; on Windows systems with a non-UTF-8 locale (e.g. CP949), output files were saved in the system code page
- `ws-named-agent tail` (codex): rewrote `_tail_codex` for the actual session file format — `{"timestamp","type","payload"}` envelope with `event_msg{task_started/task_complete/agent_message}` and `response_item{function_call}` events; old parser was written against the `--json` stdout format and found zero turns
- `ws-named-agent` (codex compression): `_call_codex` haiku intent step used hardcoded `"claude"` instead of `_claude_exe()`; fails on Windows where claude is `claude.cmd`
- `ws-named-agent` (codex): `_run()` now uses `_codex_exe()` wrapper; `codex.cmd` on Windows cannot be invoked without `cmd /c`

## v0.10.1 — 2026-04-27

### Added
- `ws-named-agent` (claude backend) — injects CLAUDE.md into the system prompt between the agent definition and the registered system prompt; walks from git root to CWD collecting all CLAUDE.md files in outer-to-inner order

### Fixed
- `ws-named-agent` Windows portability: `claude.cmd` now invoked via `cmd /c` (resolved with `shutil.which`); `tmp.replace()` replaces `tmp.rename()` to handle existing destination; `find` subprocess calls replaced with `Path.glob`/`Path.rglob`; all `text=True` subprocess calls and temp file opens now specify `encoding="utf-8"`

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
