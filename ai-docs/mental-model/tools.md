---
domain: tools
description: "Standalone binary tools built in tools/: claude-watch (TUI session viewer) and claude-dash (worktree PTY multiplexer)."
sources:
  - tools/
related:
  executor-wrapup: "claude-dash reads the named-agent registry at .git/ws@<repo>/agents/*.json, the same registry written by ws-new-named-agent and ws-call-named-agent. claude-dash is read-only; it does not invoke or modify registry entries."
---

# Tools

Binary tools built from source under `tools/` and installed via
`cargo install --path`. Each is an independent Rust crate.

## Entry Points

- `tools/claude-watch/` — JSONL session history browser; reads `~/.claude/projects/` JSONL files.
- `tools/claude-dash/` — interactive PTY multiplexer; spawns `claude` subprocesses per worktree and displays named-agent panels read-only.

## Module Contracts

### claude-dash

- `claude-dash` is an interactive terminal binary. It has no programmatic API and cannot be driven by another tool or script.
- The **main slot** (Ctrl+1) per worktree tab spawns `claude` as a PTY subprocess directly — this process is outside the named-agent registry. `ws-new-named-agent` / `ws-call-named-agent` have no visibility into it.
- **Named agent panels** (Ctrl+2–9) are read-only JSONL viewers. They discover agents by reading `<git-dir>/ws@<repo>/agents/*.json`, extract the UUID, and locate the matching JSONL session file under `~/.claude/projects/`. They never write to the registry or invoke `ws-call-named-agent`.
- Named agents are capped at 8 slots, sorted newest-first by session-file mtime. Agents beyond 8 are not displayed; the oldest drops off when a ninth is registered.
- **Auto-spawn on startup**: a worktree directory is eligible for immediate PTY spawn if `<repo-root>/.claude/worktrees/<worktree-name>` exists as a directory. Other worktrees spawn their PTY on first tab selection.
- Worktree tabs are discovered via `git worktree list --porcelain` at startup and re-polled every 5 seconds. New worktrees gain tabs automatically; tabs whose worktree is removed externally persist until the PTY exits, then close.
- When the PTY exits while the worktree still exists, a modal offers `[R] Restart` (fresh spawn, same cwd) or `[X] Close tab`.

### claude-watch

- `claude-watch` is a read-only TUI that scans `~/.claude/projects/` for JSONL session files. It has no side effects on the named-agent registry or any workspace state.

## Coupling

- `claude-dash` reads the named-agent registry but never writes it. Agents registered via `ws-new-named-agent` appear automatically in the claude-dash panel within the next poll cycle (up to 5 seconds). No additional action is required from the registering skill.
- `claude-dash` uses the same JSONL session format as `claude-watch` for its named-agent panel renderer. Both share a dependency on the undocumented Claude CLI JSONL format; unknown fields are passed through gracefully.
- The `.claude/worktrees/<name>` directory marker is the ws-framework convention for marking a worktree as active. Claude-dash uses this marker for auto-spawn only. Creating or deleting this directory affects startup behavior.

## Common Mistakes

- Expecting `ws-call-named-agent` or `ws-interrupt-named-agent` output to appear in the claude-dash main PTY slot — the main slot is a plain PTY, not a named-agent session.
- Expecting to send input to named-agent panels via claude-dash keyboard — named panels are read-only. Use `ws-call-named-agent` or `ws-interrupt-named-agent` directly from the shell.
- Assuming the ninth+ named agent is accessible via a higher `Ctrl+N` binding — claude-dash shows at most 8 named agents (Ctrl+2–9). The oldest drops off the panel when capacity is exceeded.
