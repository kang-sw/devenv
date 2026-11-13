---
title: "Hosted-session detection by session identity instead of process ancestry"
related:
  261112-bug-ws-hosted-windows-cmd-shim-host-pid-mismatch: same symptom family (Windows wrapper between harness and child)
---

# Hosted-session detection by session identity instead of process ancestry

## Background

A dashboard-hosted session is told apart from an ordinary one, and from a
second harness started inside it, by process ancestry: the daemon writes the
harness PID as the hosted row's `host_pid`, and each component compares it
with "the process that started me":

- ws-mcp: `WS_MCP_HARNESS_PID`, set by the launcher to `os.getppid()`, read
  through `Server.harnessPID()` (`agents-plugin-tool/internal/mcp/hosted_login.go`)
  by `resolveHostedSession` and the session registration.
- Claude mod: `HARNESS_PID_ARGV` probe run through `$.process.run`
  (`agents-plugin/hooks/claude-mod/hosted.ts`, `resolveHostedMode`).
- Pi extension: its own `process.pid` (in-process, no probe).

On Windows, Claude Code puts a `cmd.exe` between itself and both its stdio MCP
servers and its host commands (measured 2026-11-13: `claude.exe -> cmd.exe ->
python3.13.exe`). Both checks failed, which produced a duplicate session row,
a freshly minted key in place of `WS_HOSTED_SESSION_KEY`, and later every
reply and user message recorded twice (the mod's own dashboard feed beside the
daemon runner's). Fixed by skipping exactly one `cmd.exe` level on Windows:
558807689 (Go) and e71684683 (mod).

The owner asked whether that one-level skip is duct tape. The answer kept
it (whiteboard item (07), option c) and opened this research for a detection
that does not depend on how a harness happens to spawn its children.

## Question

Can hosted detection compare an identity the harness itself owns (for
example the hosted row's `backend_session_id`, already in the session's
`agent.json`, against a session id the harness exposes to its MCP child and
to the mod) instead of PID ancestry, for every supported harness, while
keeping the existing guarantees?

Guarantees any replacement keeps:

- A second harness started from the hosted session's shell (inheriting the
  `WS_HOSTED_*` variables) is not the hosted harness.
- A CLI command run from the hosted session's own shell is not treated as
  the hosted harness (the `daemonRow` handling in
  `agents-plugin-tool/internal/mcp/dashboard_sessions.go`).
- Any lookup failure degrades to an ordinary session, as today.

## To investigate

- Claude Code: whether a session id (or another per-session value) reaches
  stdio MCP server processes and mod host commands on Windows, macOS and
  Linux, and whether it is stable across `/reload-plugins`, resume, and
  compaction.
- Codex and Pi: the equivalent value, if any.
- Whether the daemon can learn that value at spawn (it already records
  `backend_session_id` for Claude).
- Compatibility tier of moving the check (env var or hello field added
  alongside `WS_MCP_HARNESS_PID`; T0 if additive).

## Outcome Ledger

### Verified Findings

- On Windows, Claude Code 2.1.323 starts stdio MCP servers as
  `cmd.exe /d /s /c "python3 ..."` and runs mod `$.process.run` commands
  through `cmd.exe`, which also truncates the command line at the first
  newline (e71684683 AI Context).
- The Pi extension does not probe; it uses its own `process.pid`.

### Confirmed Decisions

- Keep the one-level Windows `cmd.exe` skip (558807689, e71684683) until a
  replacement from this research lands (owner, 2026-11-13, (07) c).

### Proposals

- Identity comparison against the hosted row's `backend_session_id`.

### Open Questions

- Which per-session value each harness exposes to its children, if any.

### Rejected Alternatives

- Walking up the ancestor chain until a harness image (`claude.exe`) is
  found: needs a per-harness image list (an npm-installed Claude runs as
  `node.exe`; Codex and Pi differ), and skipping shells would let a CLI run
  from the hosted session's shell claim the hosted identity ((07) option b).
