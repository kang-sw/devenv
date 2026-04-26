---
title: Devenv Tools
summary: Custom tools built and maintained in this repo for local developer use.
---

# Devenv Tools

Custom utilities built from source in this repo and installed locally.

## Custom Rust Tools {#260426-claude-watch-installed}

Tools built from source within this repo and installed via `cargo install --path <tool-dir>`. The full-install phase will gain a step to build and install these after Homebrew tools.

| Tool | Source path | Purpose |
|---|---|---|
| `claude-watch` | `tools/claude-watch/` | TUI session viewer for Claude CLI subprocess history |
| 🚧 `claude-dash` | `tools/claude-dash/` | Interactive worktree-scoped Claude TUI multiplexer |

## Claude Session Viewer {#260426-claude-session-viewer}

`claude-watch` — a Rust TUI binary that browses `~/.claude/projects/` session history for the current project and shows live subprocess activity.

### Session Discovery

Scans `~/.claude/projects/<escaped-project-path>/` where `<escaped-project-path>` is the current working directory with `/` replaced by `-`. Lists all `.jsonl` files as browsable sessions.

Session labels follow the format `<name>(<last-edit-datetime>)`:
- If the session UUID matches a registered agent in `.git/ws@<repo>/agents/*.json`, the label uses the agent name.
- Otherwise, the label uses the UUID prefix (first 8 characters).

### Layout

Two-panel layout:

- **Left panel** — scrollable session list. Each entry shows the session label and last-modified timestamp.
- **Right panel** — turn-by-turn rendering of the selected session's JSONL. Renders user messages, assistant text, thinking blocks, and tool call/result pairs with pseudo-markdown formatting (bold, code blocks, headers) adapted for terminal display.

### Mouse Interaction {#260426-claude-watch-mouse-input}

Mouse capture is enabled on startup. Scroll wheel events scroll the right panel. Left-click on a left-panel entry selects that session.

### Live Process Indicator

Polls running processes (1–2 second interval) to find `claude` processes with a `-p` flag. Extracts the UUID from `--session-id` or `--resume` arguments. Sessions with a matching active process are highlighted green in the left panel.

> [!note] Constraints
> - macOS: reading process args requires same-user ownership. An empty result from the OS is handled gracefully — no active highlight, no error.
> - JSONL format is an undocumented internal format of the Claude CLI. The parser treats unknown fields as pass-through and degrades gracefully on format changes.
> - Windows native path escaping (`C:\Users\...`) differs from the Unix formula. On Windows the tool scans all `~/.claude/projects/` subdirectories and matches by UUID rather than relying on path derivation.

## 🚧 Claude Dash {#260426-claude-dash}

`claude-dash` — a Rust TUI binary that manages interactive Claude sessions across git worktrees within a single terminal window.

### Layout {#260426-claude-dash-layout}

Three-region layout:

- **Top bar** — horizontal tab strip, one tab per git worktree (main + named worktrees). `Ctrl+[` / `Ctrl+]` to cycle tabs.
- **Agent panel** — below the tab bar. Lists agents for the active worktree: main interactive slot (Ctrl+1) leftmost, named agents (Ctrl+2–9) ordered by recency.
- **Main panel** — fills remaining space. Content depends on selected agent slot.

### Worktree Tabs {#260426-claude-dash-worktree-tabs}

Tabs are discovered via `git worktree list --porcelain` at startup and refreshed every 5 seconds. New worktrees gain a tab automatically; removed worktrees whose process is already dead are closed.

On startup, any worktree directory present under `.claude/worktrees/` automatically spawns a fresh `claude` process as its main agent. Other worktrees spawn on first tab selection.

### Interactive Terminal {#260426-claude-dash-interactive-terminal}

When the main agent slot (Ctrl+1) is selected, the main panel hosts an embedded PTY terminal. `claude` runs as a direct subprocess with a PTY; all keyboard input forwards to the PTY stdin. VT screen state is maintained via `termwiz` (WezTerm) and rendered as a ratatui widget. PTY dimensions are updated via ioctl on panel resize.

### Named Agent Panel {#260426-claude-dash-named-agents}

Named agent slots (Ctrl+2–9) show a read-only JSONL session viewer — same rendering as `claude-watch`. Agents are discovered from `.git/ws@<repo>/agents/*.json`.

### Process Lifecycle {#260426-claude-dash-process-lifecycle}

- `claude` subprocesses are owned by claude-dash and terminate when claude-dash exits.
- Session resumption (via `--resume`) is the user's responsibility.
- When a main agent exits while its worktree still exists, a modal overlay offers: `[R] Restart` (fresh spawn in same worktree directory) or `[X] Close tab`.
- When a worktree is removed externally: tab persists until the process exits, then closes automatically.

### Keyboard Bindings {#260426-claude-dash-keybindings}

| Key | Action |
|---|---|
| `Ctrl+[` / `Ctrl+]` | Previous / next worktree tab |
| `Ctrl+1` – `Ctrl+9` | Select agent slot |
| All other keys (main agent active) | Forwarded to PTY stdin |

> [!note] Constraints
> - App-level hotkeys take priority over PTY input forwarding.
> - Requires `termwiz` and `portable-pty` (WezTerm project). VT compliance is bounded by `termwiz`'s implementation.
> - Worktree discovery relies on `git worktree list`. Non-git directories are not supported.
> - Named agents are limited to ws-framework registered agents in `.git/ws@<repo>/agents/*.json`.
