---
title: claude-dash — Interactive worktree-scoped Claude TUI multiplexer
plans:
  - ai-docs/plans/2026-04/26-2352.claude-dash.md
spec:
  - 260426-claude-dash
  - 260426-claude-dash-layout
  - 260426-claude-dash-worktree-tabs
  - 260426-claude-dash-interactive-terminal
  - 260426-claude-dash-named-agents
  - 260426-claude-dash-process-lifecycle
  - 260426-claude-dash-keybindings
---

# claude-dash — Interactive worktree-scoped Claude TUI multiplexer

## Background

`claude-watch` is a read-only session viewer. The next step is an interactive TUI multiplexer that manages multiple Claude sessions across git worktrees — one tab per worktree, each hosting a live `claude` PTY subprocess. Named (headless) agents are visible as read-only panels in the same interface.

The tool replaces the tmux-per-workspace pattern: instead of manually managing tmux windows and panes, `claude-dash` handles worktree routing, process spawning, and session visibility in a single ratatui binary.

## Decisions

- **Stack**: `ratatui` (TUI), `portable-pty` (PTY spawn + resize), `termwiz` (VT escape parsing + screen state), `serde_json` + existing claude-watch `parser.rs`/`renderer.rs` (JSONL read-only view).
- **Process ownership**: `claude` subprocesses are owned by claude-dash; they terminate when claude-dash exits. Session resumption via `--resume` is the user's responsibility — the tool does not persist or restore sessions.
- **No global mode**: scoped to the current git repo and its worktrees only. Cross-repo or cross-OS tracking is out of scope.
- **PTY size**: panel inner dimensions are used as PTY columns/rows. ioctl is issued on every panel resize.
- **Worktree polling**: `git worktree list --porcelain` polled every 5 seconds. New entries gain tabs automatically; dead entries (process exited + worktree removed) are closed automatically.

## Constraints

- Requires a git repository. Non-git directories are not supported.
- VT compliance is bounded by `termwiz`'s implementation. Edge-case terminal behavior may differ from a full terminal emulator.
- App-level hotkeys (`Ctrl+[`, `Ctrl+]`, `Ctrl+1`–`9`) take priority over PTY input forwarding; they are never sent to the subprocess.
- Named agents are limited to ws-framework registered agents in `.git/ws@<repo>/agents/*.json`.

## Phases

### Phase 1: Scaffold + single interactive PTY terminal

Create `tools/claude-dash/` Cargo project. Dependencies: `ratatui`, `crossterm`, `portable-pty`, `termwiz`, `serde_json`, `chrono`.

Implement a minimal single-session ratatui app:

- Spawn `claude` as a PTY subprocess in the current working directory.
- Maintain VT screen state via `termwiz::terminal::buffered::BufferedTerminal` (or equivalent screen model).
- Render the VT screen cells into the ratatui main panel each frame.
- Forward all keyboard input to PTY stdin, except `Ctrl+Q` (quit).
- On panel resize, issue ioctl to update PTY dimensions.
- On `claude` process exit, show a modal overlay: `[R] Restart  [Q] Quit`.

Deliverable: `claude-dash` launches, shows a functional `claude` interactive session, and handles process exit gracefully.

### Phase 2: Worktree tab bar

Add the top tab bar and per-worktree PTY management.

- Discover worktrees via `git worktree list --porcelain` at startup. Each worktree becomes a tab.
- On startup, auto-spawn `claude` for any worktree whose directory exists under `.claude/worktrees/` (active ws worktrees). Other worktrees spawn on first tab selection.
- `Ctrl+[` / `Ctrl+]` cycles tabs. The active tab's PTY receives input; other PTYs continue running in background.
- Poll worktrees every 5 seconds: add tabs for new entries, remove tabs for entries whose worktree is gone and process is dead.
- Tab label: worktree directory name (last path component). Active tab highlighted.

Deliverable: multiple worktrees are independently managed; switching tabs switches the active PTY view.

### Phase 3: Named agent panel

Add the agent slot list and read-only JSONL viewer.

- Discover named agents from `.git/ws@<repo>/agents/*.json`, ordered by last-modified descending.
- Agent slot list rendered below the tab bar: main slot (Ctrl+1) + up to 8 named agent slots (Ctrl+2–9).
- Selecting main slot (Ctrl+1) shows the interactive PTY panel.
- Selecting a named agent slot shows a read-only JSONL viewer for that agent's session file — port `session.rs`, `parser.rs`, `renderer.rs` from `claude-watch` as a shared library or copy.
- Agent list refreshes every 5 seconds alongside worktree polling.

Deliverable: named agents from the ws framework are browsable without leaving claude-dash.

### Phase 4: Process lifecycle polish

- **Exit modal**: when a main agent process exits while the tab is still open, overlay a modal with `[R] Restart` and `[X] Close tab`. Restart spawns a fresh `claude` in the same worktree directory.
- **Worktree removal**: when `git worktree list` no longer includes a worktree but its process is still running, keep the tab open (show a `[removed]` indicator in tab label). Close the tab automatically when the process exits.
- **Token count**: display `input_tokens + output_tokens` total in the agent slot list entries, matching claude-watch's display format.
- **Scrollbar**: vertical scrollbar on the read-only JSONL panel, matching claude-watch's implementation.

Deliverable: all lifecycle edge cases are handled; UI matches claude-watch quality.
