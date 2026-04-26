---
title: claude-watch — Rust TUI for Claude CLI session history and live subprocess monitoring
spec:
  - 260426-claude-watch-installed
  - 260426-claude-session-viewer
related-mental-model:
  - executor-wrapup
---

# claude-watch — Rust TUI for Claude CLI session history and live subprocess monitoring

## Background

The ws orchestration workflow spawns `claude -p` subprocesses that run for minutes at a time with no live output visibility. Each session is recorded as a JSONL file at `~/.claude/projects/<escaped-project-path>/<uuid>.jsonl`, where every turn — including thinking blocks, tool calls, and results — is written as it occurs.

The goal is a standalone Rust TUI binary (`claude-watch`) that makes this session history browsable and shows which sessions are currently running as live subprocesses.

## Decisions

- **Stack**: `ratatui` (TUI framework), `sysinfo` (cross-platform process enumeration), `serde_json` (JSONL parsing), `tui-markdown` or `pulldown-cmark` + custom span renderer (pseudo-markdown in terminal).
- **JSONL format is undocumented**: Parse defensively — unknown fields are ignored, missing fields degrade gracefully. The parser must not panic on format changes from Claude CLI updates.
- **Windows path escaping**: The Unix formula (`/` → `-`) does not apply to Windows paths (`C:\Users\...`). On Windows, scan all `~/.claude/projects/` subdirectories and match by UUID instead of deriving the path.
- **Rejected alternative — stream-json + tee**: Would require modifying `ws-call-named-agent` to switch `--output-format` and refactor the output pipeline. More invasive with no benefit over direct JSONL file watching.
- **Rejected alternative — tee-to-file only**: Provides audit logging but no live monitoring since `--output-format json` emits a single blob at completion.

## Constraints

- macOS process arg reading via `sysinfo` requires same-user ownership. An empty result must be handled gracefully — no active highlight, no error shown.
- Tool lives at `tools/claude-watch/` inside this repo. `Cargo.toml` stays inside that subdirectory (not at repo root) to avoid workspace complications.

## Phases

### Phase 1: Scaffold and session discovery

Set up the Cargo project at `tools/claude-watch/` with the full dependency set. Implement session discovery:

- Compute the escaped project path: current working directory with `/` replaced by `-`.
- Scan `~/.claude/projects/<escaped>/` for `.jsonl` files.
- For each file, compute a session label: check `.git/ws@<repo-dir>/agents/*.json` for a UUID match; use the agent name if found, else the first 8 characters of the UUID.
- Read the file's last-modified timestamp for the label suffix.
- On Windows: scan all `~/.claude/projects/` subdirectories; match UUIDs across all of them regardless of subdirectory name.

Output: a `Vec<SessionEntry>` with label, UUID, last-modified, and file path — no UI yet.

### Phase 2: Two-panel TUI layout and session list

Wire up the `ratatui` app loop with a two-panel layout:

- **Left panel**: scrollable list of `SessionEntry` items. Each row renders as `<label>(<timestamp>)`. Arrow-key navigation selects the active session. Selected session is highlighted.
- **Right panel**: placeholder text until Phase 3.
- `q` / `Ctrl+C` to quit.

File-watch (via `notify` crate or mtime polling at ~1s interval) refreshes the session list when new `.jsonl` files appear or existing ones are modified.

Depends on Phase 1.

### Phase 3: Turn renderer

Parse the selected session's JSONL and render turns in the right panel:

Turn types to render:
- `type: "user"` → render `message.content` with a `USER` prefix.
- `type: "assistant"` with `message.content[].type == "text"` → render the text with pseudo-markdown.
- `type: "assistant"` with `message.content[].type == "thinking"` → render thinking text in a visually distinct style (dimmed or boxed). Collapsible with a key binding (default collapsed).
- Tool-use and tool-result turns → render as a compact block showing tool name and truncated input/output.

Pseudo-markdown rendering: bold (`**...**`), inline code (backtick), fenced code blocks, headings (`#`, `##`, `###`). Use `tui-markdown` if it covers these; otherwise render with `pulldown-cmark` + custom `ratatui` span mapping.

Scroll the right panel with `j`/`k` or `PgUp`/`PgDn`. Selecting a new session in the left panel resets the right panel scroll to the bottom (most recent turn).

Right panel refreshes automatically when the selected session's file is modified (live tail).

Depends on Phase 2.

### Phase 4: Live process indicator

Poll running processes every 1–2 seconds using `sysinfo`:

- Find processes where the executable name is `claude` and the args contain `-p`.
- Extract the UUID from `--session-id <uuid>` or `--resume <uuid>` in the process args.
- Mark matching `SessionEntry` items as active.
- Render active sessions with a green color indicator in the left panel.

On macOS, `sysinfo` reads args via `sysctl(KERN_PROCARGS2)` which silently returns empty for cross-user processes. An empty args result means "not readable" — treat the session as inactive, no error.

Depends on Phase 2 (requires the session list to be populated before matching).
