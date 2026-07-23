---
title: Decouple dashboard terminal (PTY) lifetime from daemon process
related-mental-model:
  - ws-web-dashboard
sage-review-design: completed
sage-review-completeness: completed
---

# idea: Decouple dashboard terminal (PTY) lifetime from daemon process

> **Drain priority — TOP.** Designated by the user (2026-07-23) as the
> highest-priority pickup for the active `/lead-drain-ready-queue` run:
> select this ahead of the default FIFO ordering. Rationale: it removes the
> "daemon restart kills all live terminals" pain that currently blocks
> efficient multi-workspace Windows dogfooding.

## Spec Impact

New observable behavior: terminal (shell) sessions now SURVIVE a dashboard
daemon restart and reattach via the existing frontend resume-by-id path.
Previously a daemon restart killed all live terminals (SIGHUP). This is a
new caller-visible guarantee, not merely an internal refactor, so it
qualifies for spec addressing.

Target spec: `ws-web-dashboard/index.md`, anchors
`260523-ws-dashboard-terminal-tab-restore` ("Terminal Tab Restore" — its
current text assumes a daemon restart always leaves no daemon-alive
terminal to reattach to, which this change invalidates for the boot-reconcile
"adopt" case) and `260516-ws-web-dashboard-terminal-registry-pty-spawn`
("Terminal Registry And PTY Spawn" — daemon ownership/persistence framing).
Both anchors exist today and are the most likely amendment targets; no new
spec file is needed.

Deferral note: the actual spec-document edit is performed during
implementation's doc pre-pass (lead-update-spec judge), not here; this
section only declares the impact for the ready-gate.

Not in scope for the spec update: on-disk persistence remains a non-goal,
and scrollback stays memory-only in the helper ring, so the spec impact is
about session survival/reattach, not durable history.

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
- Identity = **OS-queryable PID + start-time ONLY**; this is the
  PRECONDITION for kill. A nonce is explicitly NOT a kill-gate option: a
  nonce is verifiable only over IPC, and IPC is dead exactly when the
  IPC-unreachable kill decision (reconcile rows 4/5) is being made, so it
  cannot gate the kill path.
- **2-tier kill**: prefer graceful IPC request to have the helper
  `child.kill()` its shell and exit; fall back to a verified-PID kill only
  when IPC is unreachable. To close the verify-to-kill TOCTOU race (a PID
  reused between start-time verification and the SIGKILL call), the
  fallback path captures the process via a **stable OS handle** at
  verification time — Linux `pidfd`, Windows process handle — and issues
  the kill THROUGH that handle, not by re-resolving the PID.
  Windows: the **helper-owned** Job Object (NOT daemon-owned) so killing
  the helper reliably takes the shell subtree; the helper spawns the shell
  detached with **breakaway-from-job** semantics. Rationale: a
  daemon-owned Job Object would close when the daemon exits and re-kill the
  shell, silently reintroducing the exact bug this ticket fixes on the
  primary Windows dogfooding platform.
- **Registry-write ordering**: the helper spawns the shell only AFTER its
  own registry entry (PID + start-time) is durably written (atomic
  temp-rename) AND the helper's PID has been handshaked back to the
  daemon. This closes the orphan-leak window where a daemon crash mid-spawn
  would otherwise leave a live shell with no registry entry to reconcile
  against.
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

**Implementation note:** the current daemon-side WS attach guard rejects
`!is_live()` sessions with `StatusCode::GONE` (`terminal.rs:454`), and
session listing filters to live sessions only (`terminal.rs:148`,
`list_for_work_root`'s `.filter(... && session.is_live())`). BOTH gates must
be relaxed so an in-grace exited session (reconcile row 2) is still visible
to the client and accepts one final reattach to deliver its buffered output
and exit code, instead of being treated as already gone.

## Non-goals / notes

- No on-disk text persistence (scrollback is memory-only in the helper
  ring).
- Scope is server-side; the frontend resume-by-id path already exists.
- ws-dashboard is downstream application code → this change does NOT bump
  the plugin version.

## Open questions

- **IPC framing/protocol choice — resolution checkpoint.** Pin the choice
  (ad hoc length-prefixed frames vs. reusing the existing terminal message
  schema) at Phase 1 kickoff; not a design-time blocker, but must be decided
  before implementation starts.
- Exact grace window value.
- Cross-platform detach mechanics — largely DECIDED: Unix uses
  setsid/double-fork; Windows uses a helper-owned Job Object + detached
  breakaway spawn (see Decided design). What remains open here is residual
  specifics only (e.g. exact Windows API calls / crate choice), not a fork
  in the approach.

## Phases

### Phase 1: Server-side per-terminal supervisor decoupling

- Completion: a live terminal survives a daemon restart and reattaches via
  the existing frontend resume-by-id path, verified by an acceptance test.
- Completion additionally requires a test proving reconcile rows 3 and 5
  (identity-mismatch / PID-reuse) NEVER kill an unverified process — the
  drop-entry-only behavior must be exercised, not just asserted in prose.
- Platform scope: the Unix path (setsid/double-fork detach, pidfd-gated
  kill) is verified end-to-end in Phase 1. Windows detach + the
  helper-owned Job Object breakaway are also IN Phase 1 scope, because
  Windows is the motivating dogfooding platform and a Unix-only result
  would not solve the pain this ticket exists for. Exception: this MAY
  split into a Phase 2 if the Job-Object breakaway work proves
  independently large — decide at Phase 1 kickoff, not mid-phase.

#### Edition (kickoff) — 2026-07-23

Phase 1 kickoff open questions resolved (autonomous goal run; user confirmed autonomous proceed and the platform direction on 2026-07-23):

- **Decision A — IPC framing: NDJSON (newline-delimited JSON) carrying a dedicated helper-facing message enum.** Reuses the in-repo precedent in `crates/daemon/src/codex_app_server.rs` (`AsyncBufReadExt::lines()` + `serde_json`, zero new crate deps). Message shapes borrow from `TerminalWebSocketServerMessage` / `TerminalWebSocketClientMessage` (Output/Status/Exit ↔ Input/Resize) but are declared as SEPARATE Rust types so browser-facing WS field changes cannot bleed into the helper wire contract. Transport stays as already decided: Unix domain socket / Windows named pipe (tokio "net" feature, already present). Rejected: ad-hoc length-prefixed binary frames — no in-repo precedent, the codex precedent explicitly rejects Content-Length-style framing, higher Phase-1 cost.
- **Decision B — platform scope: BOTH Unix and Windows land in this phase; the optional Phase-2 split is NOT taken.** The user confirmed the daemon's primary deployment is native Windows, so the Windows Job-Object breakaway detach is pulled into scope rather than deferred. Verification is split by capability: the Unix path (setsid/double-fork detach + pidfd-gated kill) is verified end-to-end IN this session; the Windows path (helper-owned Job Object + `CREATE_BREAKAWAY_FROM_JOB` + stable OpenProcess-handle kill) is implemented, cross-compile-checked, and unit-tested in this session, with LIVE native-Windows end-to-end acceptance completed on the user's native-Windows dogfooding host (this session has no native-Windows daemon host; cf. 260703-chore-windows-branch-pinned-acceptance). Both legs sit behind the existing `TerminalPlatform` abstraction (`terminal.rs`); the helper architecture, registry-file identity model (PID + start-time), IPC protocol, and the 6-row reconcile state machine are platform-independent and shared.

Implementation is staged for reviewability on a single effort: (1) helper process + NDJSON IPC + registry-file identity + 6-row reconcile + Unix detach (E2E-verified here), then (2) the Windows detach leg layered on the same abstraction (statically verified here, live-verified by user dogfood).
