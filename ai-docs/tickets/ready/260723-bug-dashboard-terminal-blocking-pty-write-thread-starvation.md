---
title: dashboard terminal latency under load traces to blocking PTY write/flush starving the shared Tokio worker pool
related:
  260723-feat-dashboard-terminal-lifetime-daemon-decouple: adjacent — moving PTY ownership to a helper process would reshape this write path, but this is a distinct performance bug to fix independently regardless of the lifetime work
related-mental-model:
  - ws-web-dashboard
sage-review-design: completed
sage-review-completeness: completed
---

# dashboard terminal latency under load traces to blocking PTY write/flush starving the shared Tokio worker pool

## Symptom

During Windows dogfooding, dashboard terminals become extremely laggy
(input/output latency), and it worsens as more terminals are opened ("more
terminals open → laggier for everyone"). User suspected input buffering
and/or external Windows security/DRM/EDR software.

## Investigation result (read-only sweep)

**Input is NOT batched/buffered on the frontend** — each keystroke is sent
immediately over the WebSocket (`terminalPaneBody.tsx:201-212`,
`socket.send({type:"input",...})`); HTTP `sendTerminalInput` is only a
per-keystroke fallback when the socket is closed. So the frontend input path
is ruled out as the buffering culprit.

### Primary root-cause candidate (confidence: high)

Blocking PTY write held inside an async Tokio task with NO
`spawn_blocking`:

- `write_input` does
  `writer.write_all(input).and_then(|()| writer.flush())` synchronously at
  `terminal.rs:591-606` (write+flush at 602-605).
- Invoked synchronously from the WS message loop `terminal.rs:723-736` →
  `handle_terminal_socket_client_message` `terminal.rs:770-782`, and from the
  HTTP fallback `terminal_input` `terminal.rs:399-414`.
- The daemon runs on Tokio's default multi-thread runtime (`#[tokio::main]`,
  `crates/daemon/src/main.rs:4`, no worker-thread override), so there are
  only ~num_cpus worker threads shared by EVERY terminal's socket task and
  all other daemon endpoints.
- If a PTY's OS write buffer fills — because the shell (PowerShell/pwsh) or
  a security hook is slow to drain reads (common under Windows AV/EDR
  interception) — `write_all`/`flush` blocks the calling Tokio worker
  thread. A few stalled terminals among N open ones can exhaust the shared
  pool and delay input/output for UNRELATED terminals. This precisely
  matches the "more terminals → laggier for everyone" symptom and the
  Windows amplification.
- The codebase already offloads blocking work via
  `tokio::task::spawn_blocking` elsewhere (`git_toolbar.rs:140,156,173,213,310`,
  `resources.rs:31`, `root_picker.rs:126,227`,
  `work_root_activity.rs:125,150,174`) — this path is an overlooked
  instance of the same pattern.

**Reframe of the DRM/security hypothesis:** security software is plausibly
the TRIGGER (slow pipe drain), but the code turns a localized per-terminal
stall into a GLOBAL one by blocking a shared thread pool. The fix makes the
daemon resilient regardless of the external cause.

### Secondary N-scaling suspects

- **resize** has the same blocking-in-async pattern: `master.resize()` at
  `terminal.rs:608-628`, called from `terminal.rs:776-780` (lower frequency,
  client debounced 250ms). (confidence: medium)
- **Frontend O(N) render scan** (confidence: medium): every PTY output chunk
  from ANY terminal calls `setTerminalPanes`
  (`applyTerminalSocketMessage`/`markTerminalOutputCursor`,
  `App.tsx:5508-5524`), re-running the whole `App` render including an
  unmemoized `Object.values(terminalPanes).filter(...).map(...)` over all
  panes at `App.tsx:4833-4848`. Cost = O(N terminals × aggregate output
  events/sec).
- **HTTP short-poll O(N) per 120ms** (confidence: medium, conditional):
  `terminalOutputPollIntervalMs=120` (`App.tsx:441`), per-pane fetch loop
  `App.tsx:4859-4916`. Idle under healthy WS, but if Windows security
  software intermittently kills WebSockets, N terminals fall back to N
  independent polls every 120ms.
- **`output_after` linear scan** over up to `MAX_OUTPUT_CHUNKS=1024`
  (`terminal.rs:565-578`, via
  `plan_output_backfill`/`send_output_backfill` `terminal.rs:796-826`) on
  every output signal (confidence: low).

### Ruled out

- Input keystroke path not debounced/batched/queued.
- `TerminalRegistry` RwLock (`terminal.rs:137-193`) touched only on
  create/list/lookup/remove, not per-keystroke — not a global per-message
  bottleneck.
- `spawn_reader` (`terminal.rs:864-879`) does not hold the session Mutex
  across the blocking `reader.read()`.
- No single global writer task / shared broadcast channel serializing all
  terminals.

## Recommended fix direction (DECIDED 2026-07-23)

Wrap the blocking PTY write (and resize) off the async worker. Two options
were weighed: (a) `tokio::task::spawn_blocking` per write — simplest,
matches existing codebase pattern, but rejected due to per-keystroke
task-spawn churn under heavy input; (b) a dedicated per-session blocking
writer thread fed by a channel — more code, avoids per-keystroke spawn
churn, better under heavy input.

**Chosen: option (b)** — a dedicated per-session blocking writer thread fed
by a channel. Decided by the user on 2026-07-23.

Scope: move BOTH the blocking PTY write (`terminal.rs:591-606`) AND the
blocking resize (`terminal.rs:608-628`) off the async Tokio worker; resize
may route through the same per-session writer thread as the write path.

Out of scope for this ticket (tracked as separate follow-ups): the
frontend O(N) render scan (`App.tsx:4833-4848`) and the HTTP short-poll
fallback (`App.tsx:441`, `App.tsx:4859-4916`).

## Spec Impact

None — this is an internal concurrency fix; caller-visible terminal
behavior is unchanged (input is still delivered in order over WS and the
HTTP fallback; only the execution moves off the shared async worker
thread). Per the spec-impact rule, an internal refactor preserving all
caller-visible behavior does not qualify for spec addressing.

One nuance surfaced by design review: a mid-flight write failure now
surfaces asynchronously via the existing `output_signal`/`is_live` path
(the reader thread marks the session exited/error, which the socket task
already observes) instead of as a synchronous `Gone` from `write_input`.
The synchronous `Gone` response for an already-closed terminal is
preserved. This is an internal error-surfacing-locus nuance, not a
behavior change visible to callers.

## Phases

### Phase 1: Move blocking PTY write + resize off the async worker

Completion boundary: daemon builds clean and the existing daemon test
suite passes; `write_input` and `resize` no longer block a Tokio worker
thread (writes/resizes are handed to a per-session dedicated blocking
writer thread); terminal input/output behavior is unchanged and in-order
over both WS and HTTP fallback.

Constraints on the completion boundary:

- **Non-blocking handoff (required):** the async→writer-thread enqueue
  MUST be non-blocking — an unbounded channel, or `try_send` with a
  defined full-channel policy. A bounded channel whose `send()` blocks a
  Tokio worker thread is explicitly forbidden (it would reintroduce the
  pool starvation this ticket fixes). `MAX_INPUT_BYTES` already caps
  per-message size, so unbounded growth is human-typing-rate bounded on a
  live terminal.
- **Shutdown ordering (required):** `terminate()`/`mark_error()`/
  `mark_exited()` must FIRST unblock any stalled write (kill child / drop
  the master so `write_all` returns EIO/EPIPE), and must NEVER join the
  writer thread while holding the session mutex; detaching rather than
  joining is acceptable degradation if a pathological pipe never
  unblocks. (Avoids a teardown deadlock.)
- **Preserve synchronous fast-path:** keep the cheap `status==Running` /
  writer-present check synchronous in `write_input` so an already-closed
  terminal still returns `Gone` immediately.
- **Test requirement:** add a targeted unit/integration test for the new
  per-session writer-thread/channel path — covering in-order delivery AND
  error/session-teardown handling — since the existing daemon suite
  exercises none of `write_input`/`resize`.

## Relation

Adjacent to `260723-feat-dashboard-terminal-lifetime-daemon-decouple`
(moving PTY ownership to a helper process would reshape this write path),
but this is a DISTINCT performance bug that should be fixed independently
regardless of the lifetime work.

## Autonomy note

Behavior-preserving (internal concurrency fix), but perf fixes lack a
crisp deterministic regression test and option (a)/(b) is a real design
fork — so this is NOT an auto-drain candidate; it needs a fix-approach
decision before promotion.
