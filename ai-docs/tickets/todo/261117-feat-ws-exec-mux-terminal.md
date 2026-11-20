---
title: "ws/exec mux: 렌더링된 화면 읽기를 지원하는 PTY 기반 터미널 세션"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-exec-tools: "prerequisite; owns exec.shell, exec.write and the job model this builds on"
  261117-feat-ws-hosted-child-session-entry: "same live-view direction"
---

# ws/exec mux: PTY terminal sessions

## Background

Split out of `261117-feat-ws-exec-tools` by (51)=a: interactive full-screen
programs (REPLs, TUIs, debuggers) need a terminal, which pipe-based `exec.run`
and `exec.shell` cannot give. The daemon provides one itself.

## Decisions

1. **Surface (51)=a.** `exec.shell(cmd, mux: true, ...)` runs the command under a
   PTY. `exec.mux.read(exec_id)` returns the rendered current screen text.
   `exec.write(exec_id, data)` (from the exec tools ticket) sends input, keys as
   escape sequences.
2. **Daemon-embedded PTY plus VT emulator.** `openpty` on Unix, ConPTY on
   Windows 10 1809+, and a Go VT emulator library that renders the screen
   buffer. Rejected: tmux. It is absent on native Windows, needs WSL/MSYS2 there,
   native console programs misbehave under MSYS2 tmux, and corporate PCs with
   heavy security software block extra installs.
3. **Later direction, not a decision:** a dashboard live view/takeover of the
   screen buffer (same direction as `261117-feat-ws-hosted-child-session-entry`).

## Constraints

- Go tests run on remote w5090 only, never local `go test`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`.
- Release-gate file-touch rule (owner, 2026-11-17): the screen buffer lives in
  memory; no per-output file writes.

## Compatibility

New `exec.shell` argument and `exec.mux.read` tool are additive (T0). The
concrete verdict cites source lines per `ai-docs/manuals/release-compatibility.md`,
recorded in Phase 1's Result.

## Phases

### Phase 1: PTY mux sessions with screen read

Implement `mux: true` on `exec.shell` over an embedded PTY (openpty / ConPTY) with
a Go VT emulator, `exec.mux.read` returning the rendered screen text, and
`exec.write` input including key escapes; the screen buffer stays in memory.

Deferred: dashboard live view/takeover.

Verification: Go tests on remote w5090 for PTY spawn, screen render after output
and cursor/escape handling, write of key escapes, and no per-output file writes;
a Windows ConPTY check (Windows 10 1809+) running a console program and reading
its screen.
