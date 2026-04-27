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
- The **main slot** (`[q:main]`) per worktree tab spawns `claude` as a PTY subprocess directly — this process is outside the named-agent registry. `ws-new-named-agent` / `ws-call-named-agent` have no visibility into it.
- **Named agent panels** (`[w:…]` `[e:…]` `[r:…]` …) are read-only JSONL viewers. They discover agents by reading `<git-dir>/ws@<repo>/agents/*.json`, extract the UUID, and locate the matching JSONL session file under `~/.claude/projects/`. They never write to the registry or invoke `ws-call-named-agent`.
- Named agents are capped at 8 slots, sorted newest-first by session-file mtime. Agents beyond 8 are not displayed; the oldest drops off when a ninth is registered.
- **Keybinding system**: all multi-key actions use a Ctrl+B prefix. Pressing Ctrl+B activates prefix mode (shown as `[^B]` in the tab bar and as a centred help overlay). The next keystroke dispatches:
  - `1`–`9` — switch to worktree tab by 1-based index (clamped to tab count).
  - `0` — switch to worktree tab 10 (index 9, clamped).
  - `q` `w` `e` `r` `t` `y` `u` `i` `o` `p` — switch agent slot within the current tab (`q` = main PTY, `w`–`p` = named agents 1–9).
  - `n` — open a new session tab running `claude` at the git root (or current tab cwd).
  - `N` — open a new worktree tab running `claude --worktree` at the git root.
  - Ctrl+B or any unrecognised key — cancel prefix mode with no action.
  - Ctrl+Q always quits regardless of prefix mode (not a prefix command).
- **New session tab** (prefix+n): spawns `claude` in the git root directory; the new tab is appended and immediately made active. If a tab with the same derived name already exists, a ` #2`, ` #3`, … suffix is appended to keep names unique.
- **New worktree tab** (prefix+N): spawns `claude --worktree` in the git root; the tab is marked as a provisional worktree spawn (`is_worktree_spawn = true`). When `reconcile_worktrees` detects a new worktree that matches no existing tab path, it claims the first eligible provisional tab in-place — updating its path and name — instead of creating a duplicate tab.
- **Mouse navigation**: left-clicking on the tab bar (row 0) switches to that tab; left-clicking on the slot row (row 1) switches to that slot. Mouse events in the main PTY panel are forwarded to the subprocess as X10-encoded VT sequences (button-motion tracking enabled at spawn via `\x1b[?1002h`). Mouse wheel scrolls the named-agent JSONL panel when a named slot is active.
- **`--dangerously-skip-permissions` flag**: when `claude-dash` is launched with this flag, every spawned `claude` subprocess (initial tabs, on-demand spawns, restart after exit, prefix+n, prefix+N) receives `--dangerously-skip-permissions`. The flag propagates uniformly; there is no per-tab override.
- **Auto-spawn on startup**: a worktree directory is eligible for immediate PTY spawn if `<repo-root>/.claude/worktrees/<worktree-name>` exists as a directory. Other worktrees spawn their PTY on first tab selection.
- Worktree tabs are discovered via `git worktree list --porcelain` at startup and re-polled every 5 seconds. New worktrees gain tabs automatically; tabs whose worktree is removed externally persist until the PTY exits, then close.
- When the PTY exits while the worktree still exists, a modal offers `[R] Restart` (fresh spawn, same cwd) or `[X] Close tab`.

### claude-watch

- `claude-watch` is a read-only TUI that scans `~/.claude/projects/` for JSONL session files. It has no side effects on the named-agent registry or any workspace state.

## Coupling

- `claude-dash` reads the named-agent registry but never writes it. Agents registered via `ws-new-named-agent` with the `claude` backend appear automatically in the claude-dash panel within the next poll cycle (up to 5 seconds). Agents registered with the `codex` backend are invisible to claude-dash — their session files live at `~/.codex/sessions/`, not `~/.claude/projects/`, and claude-dash only resolves JSONL paths under `~/.claude/projects/`. No additional action is required from the registering skill for claude-backend agents.
- `claude-dash` uses the same JSONL session format as `claude-watch` for its named-agent panel renderer. Both share a dependency on the undocumented Claude CLI JSONL format; unknown fields are passed through gracefully.
- The `.claude/worktrees/<name>` directory marker is the ws-framework convention for marking a worktree as active. Claude-dash uses this marker for auto-spawn only. Creating or deleting this directory affects startup behavior.

## Common Mistakes

- Expecting codex-backend named agents to appear in the claude-dash named-agent panel — claude-dash resolves JSONL files under `~/.claude/projects/` only; codex sessions live at `~/.codex/sessions/` and are not visible.
- Expecting `ws-call-named-agent` or `ws-interrupt-named-agent` output to appear in the claude-dash main PTY slot — the main slot is a plain PTY, not a named-agent session.
- Expecting to send input to named-agent panels via claude-dash keyboard — named panels are read-only. Use `ws-call-named-agent` or `ws-interrupt-named-agent` directly from the shell.
- Assuming the ninth+ named agent is accessible via a higher slot key — claude-dash shows at most 8 named agents (prefix+w through prefix+p maps to agents 1–9). The oldest drops off the panel when capacity is exceeded.
- Sending a bare key (e.g. `1`, `n`) expecting tab or slot switching — all multi-key navigation requires the Ctrl+B prefix first. Bare keys are forwarded to the active PTY subprocess.
- Assuming prefix+N creates an immediately-named worktree tab — the tab starts with a provisional name derived from the git root directory. The name and path update in-place when the new worktree is detected by the 5-second poll cycle.
