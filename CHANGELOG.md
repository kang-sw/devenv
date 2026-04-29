# Changelog

## v0.15.0 — 2026-04-29

### Added
- `/exit-session` skill — 4-phase session handoff: commit staged work, write context note to `_index.md ## Session Notes` from memory only (no tool calls), user approval, commit `_index.md`. Optimizes next-session orientation cost via file-path citations and `(uncertain)` markers.

### Changed
- `ws-subquery`: Explore-level tool access — Bash (read-only), Read, Glob, Grep, WebFetch, WebSearch permitted; Edit, Write, NotebookEdit, Agent prohibited. Matches the native `Explore` subagent type.
- `ws-subquery`: stdin/heredoc support — accepts inline string, `-` sentinel (reads stdin), or piped stdin. Enables multi-line prompts via `ws-subquery --deep-research - <<'PROMPT' ... PROMPT`.
- `forge-spec`: all native `Agent()` calls replaced with `ws-subquery --deep-research` (survey dispatches) and `ws-oneshot-agent -p clerk` (ticket association). No native Agent tool dependency.
- `forge-mental-model`: all native `Agent()` calls replaced with `ws-subquery --deep-research`. No native Agent tool dependency.
- `clerk.md`: moved from `claude-plugin/agents/` to `claude-plugin/infra/prompts/`. Invoked via `ws-oneshot-agent -p clerk --model sonnet`.

## v0.14.0 — 2026-04-29

### Added
- `ws-ask-api` — new bin tool: queries a per-project `ai-docs/.deps/` external API documentation cache. 2-layer routing: Haiku pre-router resolves canonical domain names; persistent `api-doc-<domain>` named-agent sessions handle fetch, cache, and answer. Supports `--refresh`, `--check-stale`, `--list`. Parallel dispatch for multi-domain queries. Exit code propagated from all call sites.
- `api-doc-manager` infra prompt — per-domain executor agent: bootstraps `l1–l3.md` + scripts on first use, answers queries from cache, re-fetches on stale detection.
- `pre-router` infra prompt — Haiku one-shot agent: maps free-text prompts to canonical `.deps/` domain names with fuzzy matching and exact-match bypass.
- `--prompt-cond BINARY[=PROMPT]` flag on `ws-new-named-agent` — appends a named prompt to the system prompt only when the specified binary is present in PATH at registration time.
- `cargo-brief.md` infra prompt — injected via `--prompt-cond cargo-brief`; instructs agents to use `cargo brief` for Rust API exploration.
- `ws-ask-api` entry in `ws:workflow` skill primitives reference.

### Changed
- `workflow-for-agent.md`: added `## API Documentation` section — agents must use `ws-ask-api` for external library API lookup; direct `WebSearch`/`WebFetch` for API docs prohibited.

### Fixed
- `ws-ask-api-internal`: flock timeout removed — kernel releases fd on any exit including crash; Windows mkdir fallback replaced with PID-based stale lock detection (no timeout, crash-safe).
- `ws-ask-api-internal`: `api-doc-manager` now registered with `--no-doc-system` to prevent recursive `ws-ask-api` invocation from within the cache agent.

## v0.13.2 — 2026-04-29

### Fixed
- Remove all `timeout: 600000` mentions from skill and mental-model docs — blanket timeout instruction was causing downstream agents to insert incorrect timeout values into Bash calls, triggering 127 errors.

## v0.13.1 — 2026-04-29

### Changed
- `sprint` skill: inject project map at skill invocation via `!`ws-proj-tree`` — mirrors the pattern already present in `discuss`.

## v0.13.0 — 2026-04-29

### Added
- `ws-oneshot-agent` — new bin tool: registers, calls, and erases a named agent in one invocation. Accepts `-p <stem>` (multi-flag), `--model`, `--no-doc-system`. Doc-system injected by default. Stdin or inline positional prompt. EXIT trap guarantees cleanup.
- `ws-named-agent erase` — removes a named agent's registry entry and its associated Claude session file.
- `/workflow` skill — loads the WS orchestration primitives reference into session context; survives compaction via the Skill tool mechanism.
- `claude-plugin/infra/prompts/subquery.md` — extracted subquery worker prompt; standard agent layout (Identity/Constraints/Process/Output/Doctrine).

### Changed
- `ws-named-agent new`: accepts `-p <stem>` multi-flag; resolves against `infra/prompts/` → `infra/` → cwd; concatenates bodies with `---`; first frontmatter `model:` sets tier. A leading `ws:` prefix on `-p` values is silently stripped.
- `ws-named-agent new`: removed `--agent` and `--agent-type` flags (all call sites migrated to `-p`). `--system-prompt` retained as internal flag for compression re-registration only.
- `ws-subquery`: delegates to `ws-oneshot-agent -p subquery`; full tool access replaces prior `--allowed-tools` restriction; doc-system injected by default.
- Agent prompts consolidated from `claude/agents/` and `claude/infra/` into `claude-plugin/infra/prompts/` (single resolution root for `-p`).
- Plugin directory renamed `claude/` → `claude-plugin/`.
- `doc-system.md` renamed to `workflow-for-agent.md` (more accurate name for the orientation doc).

### Fixed
- `install.sh`: plugin snapshot correctly generates `marketplace.json` so `claude plugin install` discovery works on fresh machines.
- `install.sh`: purge stale registry entry before reinstall to bypass version no-op.

## v0.12.0 — 2026-04-28

### Added
- `/update-spec` skill — lead-driven spec audit: loads `spec-conventions.md` and `write-spec/SKILL.md`, scans a commit range for caller-visible behavior changes (`judge: spec-impact`), adds missing entries, strips `🚧` markers, and handles removals. No subagent delegation. Wired into `/edit` (after cleanup), `/implement` (doc pre-pass step 1), and `/sprint` (wrap-up step 2).
- `claude/infra/doc-system.md` — orientation doc for 3rd-party subagents: explains the three doc layers (spec/mental-model/tickets), `{#YYMMDD-slug}` stems, and `🚧` = planned-but-unimplemented. Auto-injected by `ws-named-agent new` into every agent system prompt.

### Changed
- `ws-named-agent new`: prepends `doc-system.md` to stored system prompt automatically. Pass `--no-doc-system` to suppress (for narrow-role agents such as sprint-survey, project-survey, compression helpers).
- `/sprint` wrap-up spec-update pass: replaced inline 11-line procedure with `Invoke ws:update-spec`.
- `/implement` doc pre-pass: replaced `ws:spec-updater` dispatch with `ws:update-spec` Skill invocation.
- `/edit`: added step 6 — invoke `ws:update-spec` on the edit's commit range; adds `Spec:` line to completion report.
- `ws-named-agent` (`codex` backend): compression disabled — multiple interacting bugs made it unreliable; token count still tracked for observability.
- `ws-named-agent`: `_subrun` now defaults `stdin=subprocess.DEVNULL` when neither `stdin` nor `input` is provided — prevents child claude/codex processes from inheriting the caller's stdin fd and blocking on a heredoc pipe.

### Fixed
- `ws-named-agent`: reconfigure `stdin` to UTF-8 on Windows — non-ASCII characters in heredoc prompts (e.g. `×`, `—`) were read via CP949-encoded stdin, producing surrogates that caused `UnicodeEncodeError` when passed as subprocess input.

## v0.11.4 — 2026-04-28

### Fixed
- `ws-named-agent`: reconfigure `stdout`/`stderr` to UTF-8 on Windows at module load — prevents `UnicodeEncodeError` and mojibake on non-UTF-8 locales (e.g. CP949). Uses `None` guard so the path under `pythonw.exe` (hook invocation) silently skips.

## v0.11.3 — 2026-04-28

### Added
- `ws-print-infra` now accepts bare stems (no `.md` suffix): probes exact match first, then appends `.md`; consistent with `ws-named-agent --system-prompt` resolution.

### Fixed
- `ws-named-agent`: PostToolBatch/PostToolUse hook uses `pythonw.exe` on Windows — suppresses per-tool-call console window flashes when running inside a PTY (e.g. claude-dash).
- `ws-named-agent` (codex, Windows): prompt now delivered via stdin (`-`) to bypass `cmd.exe /c` newline truncation that silently cut multi-line prompts to their first line.
- `ws-named-agent` (codex, Windows): compression re-registration now uses `sys.executable + SCRIPT_DIR` path — bare name `ws-named-agent` was unresolvable via `CreateProcess` on Windows.
- `ws-named-agent` (codex): `_codex_tokens()` no longer double-counts `cached_input_tokens`; OpenAI reports it as a subset of `input_tokens`, not an additive field — was inflating compression threshold checks by ~3×.

## v0.11.2 — 2026-04-28

### Added
- `claude/infra/searcher.md` — resident codebase-search agent with domain accumulation; lead agents spawn once per domain and reset via `ws-new-named-agent` on domain shift

### Changed
- `ws-new-named-agent --system-prompt` now accepts bare stems and bare names: probe order is `infra/<name>`, `infra/<name>.md`, `cwd/<name>`, `cwd/<name>.md`, then error; explicit paths pass through unchanged. Removes the `$(ws-infra-path xxx)` boilerplate from all call sites.

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
