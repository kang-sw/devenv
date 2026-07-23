---
title: Decouple dashboard terminal (PTY) lifetime from daemon process
related-mental-model:
  - ws-web-dashboard
---

# idea: Decouple dashboard terminal (PTY) lifetime from daemon process

> **Drain priority — TOP.** Designated by the user (2026-07-23) as the
> highest-priority pickup for the active `/lead-drain-ready-queue` run:
> select this ahead of the default FIFO ordering. Rationale: it removes the
> "daemon restart kills all live terminals" pain that currently blocks
> efficient multi-workspace Windows dogfooding.

## Problem

Dashboard daemon restart kills all live terminals. During multi-workspace
Windows dogfooding there are many live terminals across workspaces, so
bringing the daemon down/up is very costly. We want terminal (shell) process
lifetime decoupled from the daemon so terminals survive a daemon restart and
reattach.

## Current architecture & the seam

- Shell is spawned in-process by the daemon via the `portable-pty` crate; the
  child shell is a direct child of the daemon and the daemon holds the PTY
  master fd. `ws-dashboard/crates/daemon/src/terminal.rs:489-509` (`spawn`:
  openpty + spawn_command), `:498` (`default_shell()`).
- Session identity is opaque `term_<18 alnum>` (`terminal.rs:1236-1243`).
  Live PTY state (`status`, `writer`, `master`, `child`, output ring buffer)
  lives in `TerminalSessionInner` (`terminal.rs:205-214`) behind a Mutex.
- Registry is purely in-memory `Arc<RwLock<HashMap<String, Arc<TerminalSession>>>>`
  (`terminal.rs:137-193`), on `AppState.terminals` (`router.rs:84`). No
  cross-restart persistence. `MAX_TERMINAL_SESSIONS = 16` (`terminal.rs:26,167`).
- On daemon exit, the PTY master closes and the shell receives SIGHUP and
  dies. Graceful teardown kills children via `terminate()` (`terminal.rs:630-643`).
- **Key fact:** the frontend ALREADY reconnects resume-by-id with cursor
  backfill — WS URL carries `terminalId + after` cursor (`terminals.ts:149-168`),
  socket effect keyed on `[terminalId]` reattaches instead of minting a new
  session (`terminalPaneBody.tsx:537,633`), daemon backfills missed output
  from the cursor (`terminal.rs:796-826`). So the missing half is ONLY
  server-side PTY-lifetime survival.
- Natural cut boundary: `TerminalSessionInner` (`terminal.rs:205-214`) — the
  single struct owning PTY master+writer+child — plus `spawn` (`:479-539`)
  and `spawn_reader` (`:864-879`). These move into a per-terminal process;
  the daemon-side session retains metadata + IPC handle.

## Decided design

- Per-terminal **detached supervisor (helper) process** owns the PTY master +
  shell child; lifetime independent of the daemon (detached: own
  session/setsid on Unix; survives daemon exit and IPC drop).
- **Helper owns a bounded output ring** — NOT pure stdio forwarding.
  Rationale: output produced during the daemon-restart window must not be
  lost, and the existing cursor-backfill contract must stay continuous
  across restart. Memory only; NO on-disk text persistence. This makes the
  helper the authoritative output buffer and the daemon a thin proxy; the
  cursor becomes a helper-owned monotonic sequence.
- **Native IPC** (Unix domain socket / Windows named pipe) preferred over
  TCP loopback (avoids port allocation + loopback exposure). If TCP is ever
  used, mandate 127.0.0.1 + per-helper token.
- **Registry as a directory**, one file per terminal:
  `<runtime-dir>/terminals/<termid>.json`, mode 0600. Atomic writes
  (temp-rename). The **helper owns** create-on-spawn / delete-on-exit of its
  own entry (so a daemon crash cannot orphan the file); the **daemon
  prunes** only entries it has positively confirmed dead. Per-terminal
  files avoid multi-writer races.
- Identity = **PID + starttime (or nonce)**; this is the PRECONDITION for
  kill.
- **2-tier kill**: prefer graceful IPC request to have the helper
  `child.kill()` its shell and exit; fall back to PID SIGKILL only when IPC
  is unreachable. Use a **Windows Job Object** so killing the helper
  reliably takes the shell subtree.
- Keep `MAX_TERMINAL_SESSIONS = 16`, enforced after reconcile counts
  adopted live sessions.

## Boot reconcile policy

Three axes: IPC reachable? / PID alive & identity matches ours? / shell
child alive?

| # | IPC | PID/identity | Shell | Action |
|---|-----|--------------|-------|--------|
| 1 | reachable | matches, alive | alive | **Adopt** — reconnect proxy, resume streaming (happy path) |
| 2 | reachable | matches | already exited | **Adopt-then-reap** — deliver remaining buffered output + exit code to the reattaching client, then shut helper down and remove entry |
| 3 | reachable | **identity mismatch** | — | **Drop entry only.** Do NOT kill (not ours). Mark stale, warn |
| 4 | unreachable | matches, alive | unknown | Short bounded retry → if still unreachable, **kill by verified PID + drop entry** |
| 5 | unreachable | **identity mismatch** (PID reused) | — | **Drop entry only. NEVER kill** (would kill an innocent process). Warn: leaked shell |
| 6 | unreachable | PID gone (dead) | (died with helper) | **Drop entry only** — the "file present but process already dead" case |

**Invariant (three lines):**

- Adopt = IPC-reachable && identity is ours.
- Kill = identity is verified-ours && unrecoverable (IPC dead).
- If identity is unverified, NEVER kill — drop the entry only.

**Ordering/timeouts:** reconcile completes BEFORE accepting new terminal
opens and BEFORE serving the session list to clients (clients never see a
half-reconciled view). Per-entry IPC connect timeout (~250-500ms) so a hung
helper cannot stall boot; a timeout degrades to row 4. Duplicate entries
(same termid / same socket): keep the one passing the handshake, drop the
rest. Whole-file parse failure → back up and start fresh with a loud
warning; single-entry parse failure → skip that entry, keep the rest.

## Grace-reattach (confirmed decision)

For a helper whose shell has already exited (row 2), hold the ring for a
short grace window (e.g. 30s, or until one reattach) and deliver the last
output + exit code before self-exiting — so a pane shows WHY it ended
instead of silently vanishing.

## Non-goals / notes

- No on-disk text persistence (scrollback is memory-only in the helper
  ring).
- Scope is server-side; the frontend resume-by-id path already exists.
- ws-dashboard is downstream application code → this change does NOT bump
  the plugin version.

## Open questions

- IPC framing/protocol choice (length-prefixed frames? line protocol?
  existing schema reuse?).
- Exact grace window value.
- Cross-platform detach mechanics (setsid/double-fork on Unix vs. Job
  Object + detached process on Windows).

## Phases

### Phase 1: Server-side per-terminal supervisor decoupling

- Completion: a live terminal survives a daemon restart and reattaches via
  the existing frontend resume-by-id path, verified by an acceptance test.
