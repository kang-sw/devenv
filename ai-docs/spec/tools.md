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

### Live Process Indicator

Polls running processes (1–2 second interval) to find `claude` processes with a `-p` flag. Extracts the UUID from `--session-id` or `--resume` arguments. Sessions with a matching active process are highlighted green in the left panel.

> [!note] Constraints
> - macOS: reading process args requires same-user ownership. An empty result from the OS is handled gracefully — no active highlight, no error.
> - JSONL format is an undocumented internal format of the Claude CLI. The parser treats unknown fields as pass-through and degrades gracefully on format changes.
> - Windows native path escaping (`C:\Users\...`) differs from the Unix formula. On Windows the tool scans all `~/.claude/projects/` subdirectories and matches by UUID rather than relying on path derivation.
