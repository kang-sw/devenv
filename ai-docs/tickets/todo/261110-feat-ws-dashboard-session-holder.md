---
title: "Dashboard agent host: a Python session holder that owns agent processes and PTYs across daemon restarts"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-mcp-daemon-hosted-proxy-launcher: "the daemon now hosts every MCP server and is replaced on version change; its Decision 7 (replacement drain, transparent proxy reconnect) is the restart behavior a holder must survive"
  261110-research-ws-dashboard-agent-host-foundations: "evidence for the archived terminal (no process survival) and the python3 prerequisite"
---

# Dashboard agent host: session holder

## Background

The user runs very long sessions and expects to update the dashboard daemon
often. A daemon restart must not end a daemon-hosted agent or terminal
(user, 2026-11-10 whiteboard, (03) a, (06) a).

The archived ws-dashboard did not provide this. Its `TerminalRegistry` lived
in daemon memory and the daemon owned each PTY child, and
`260523-feat-ws-dashboard-terminal-tab-restore` accepted that "live terminal
processes do not need to survive daemon death": restored tabs were new
shells (`git show archive/ws-dashboard:ws-dashboard/crates/daemon/src/terminal.rs`).
This ticket builds the missing piece: a small per-session holder process that
owns the child process and outlives the daemon.

The holder is shared by both epic tracks: track A
(`261110-feat-ws-dashboard-headless-lead-sessions`) puts pipe-based agent
processes in it, track B (`261110-feat-ws-dashboard-terminal`) puts PTYs in
it.

## Decisions

1. **Process survival, not only conversation survival (user, (06) a).** A
   holder owns its child (an agent process over pipes, or a shell over a
   PTY) and keeps it running while the daemon exits and restarts; the daemon
   reattaches afterwards, so an in-flight turn or shell continues.
   Rejected: conversation-only survival, where the daemon's exit ends the
   process and the daemon resumes through the backend session ID ((06) b),
   and staging that first ((06) c). The user's reason: very long sessions are
   common.
2. **The holder is a Python script using the standard library only
   (user, (06) note; technical, (09)).** python3 is already a prerequisite on
   every host: Claude and Codex start ws through
   `python3 ./bin/ws-mcp-launcher.py` (`agents-plugin/.mcp.json`), Pi spawns
   the same launcher (`agents-plugin-pi/src/mcp-stdio-client.ts`), and
   Windows requires `python3` on PATH (`ai-docs/manuals/ws-mcp.md`). A `.py`
   file can be replaced while a holder runs it, so the Windows lock on a
   running executable does not block updates. No third-party package: the
   plugin distribution has no pip install step.
   Rejected: a holder that is the ws-mcp executable (a running `.exe` cannot
   be overwritten on Windows, and each new executable run triggers the
   endpoint protector that `261110-feat-ws-mcp-daemon-hosted-proxy-launcher`
   exists to avoid); a separate native holder binary; pywinpty or any other
   pip dependency.
3. **PTY: `pty` on POSIX, ConPTY through `ctypes` on Windows (technical,
   (09)).** The standard library `pty` module is POSIX-only; on Windows the
   holder calls `CreatePseudoConsole` and its companions through `ctypes`.
   Uncertainty: ctypes ConPTY behavior across Windows versions; verify with
   `ai-docs/manuals/windows-dogfood.md`.
4. **Holder protocol version (technical, (10)).** On attach the holder
   reports its protocol version. The daemon supports the current and the
   previous version (N-1). An older holder is reported to the user with a
   prompt to restart that session when it is idle. Rejected: no negotiation
   (a daemon update could fail to reattach to existing sessions, which
   defeats Decision 1).
5. **Discovery and orphan handling (technical, (25)).** Each holder registers
   its PID, its attach socket, and its session descriptor in a state
   directory. On start the daemon scans the directory and reattaches. The
   dashboard lists holders no daemon is attached to, with a stop button. A
   holder whose child has exited, and to which nobody attaches for a set
   period, exits on its own. The daemon never kills a live holder
   automatically: losing a very long session costs more than an idle
   process. Rejected: automatic reaping of detached holders.

## Constraints

- The holder must outlive the process that spawned it: it is detached from
  the daemon's process group and session on POSIX, and from any job object
  that kills its members when closed on Windows.
- The holder only owns processes and relays bytes. Approval, mailbox
  delivery, workspace binding, and rendering belong to the children
  `261110-feat-ws-dashboard-headless-lead-sessions`,
  `261110-feat-ws-dashboard-workspaces`, and `261110-feat-ws-dashboard-terminal`.
- An agent process inside a holder reaches ws MCP through the launcher proxy,
  which reconnects on its own when the daemon is replaced
  (`261110-feat-ws-mcp-daemon-hosted-proxy-launcher` Decision 7). Cite that
  ticket's Decisions, not the current code, for restart behavior; its proxy
  design was still being corrected on 2026-11-10.
- To settle at promotion: the attach transport (Unix socket, Windows named
  pipe, or loopback TCP with a token); output buffering for reattach
  (scrollback size, and replay of stream-json output produced while
  detached); the state directory location under the ws cache root; the idle
  period for self-exit; Python version floor.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (the holder ships to
  downstream projects), `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/windows-dogfood.md`.

## Phases

### Phase 1: holder process, attach protocol, discovery

Scope and verification are settled at promotion, after the Constraints'
open items. Verification must include a daemon restart with a live child
on Linux, macOS, and native Windows.
