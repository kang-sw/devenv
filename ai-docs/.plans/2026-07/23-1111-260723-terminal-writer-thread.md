# Plan: 260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation — Phase 1

## Relevant Ticket Contract

- Completion boundary: daemon builds clean, existing daemon test suite passes;
  `write_input` and `resize` no longer block a Tokio worker thread (handed to
  a per-session dedicated blocking writer thread); terminal input/output stays
  unchanged and in-order over WS and HTTP fallback.
- Decided fix approach: option (b) — a dedicated per-session blocking writer
  thread fed by a channel (NOT per-call `spawn_blocking`, rejected for
  per-keystroke task-spawn churn).
- **Non-blocking handoff (required):** async→writer-thread enqueue must be
  non-blocking — unbounded channel or `try_send` with a defined full-channel
  policy; a blocking-`send()` bounded channel is explicitly forbidden.
- **Shutdown ordering (required):** `terminate()`/`mark_error()`/
  `mark_exited()` must FIRST unblock any stalled write (kill child / drop the
  master so `write_all` returns EIO/EPIPE) and must NEVER join the writer
  thread while holding the session mutex; detaching instead of joining is
  acceptable degradation.
- **Preserve synchronous fast-path:** the cheap `status==Running` /
  writer-present check in `write_input` stays synchronous so an
  already-closed terminal still returns `Gone` immediately.
- **Test requirement:** add a targeted unit/integration test for the new
  per-session writer-thread/channel path covering in-order delivery AND
  error/session-teardown handling.
- Spec impact: none (internal concurrency fix, no caller-visible behavior
  change). A mid-flight write failure may now surface asynchronously via the
  existing `output_signal`/reader-thread `mark_error`/`mark_exited` path
  instead of a synchronous `Gone` from `write_input`; the synchronous `Gone`
  for an *already-closed* terminal is preserved.

## Out of Scope

- Frontend O(N) render scan (`App.tsx:4833-4848`) and HTTP short-poll
  fallback (`App.tsx:441`, `App.tsx:4859-4916`) — tracked as separate
  follow-ups per the ticket.
- `260723-feat-dashboard-terminal-lifetime-daemon-decouple` (adjacent,
  separate ticket) — do not reshape PTY ownership toward a helper process.
- `output_after` linear scan (`terminal.rs:565-578`) — ruled-out/low-confidence
  secondary suspect, not part of this phase.
- Changing `TerminalRegistry`, WS/HTTP route auth, or shell-selection logic.

## Codebase Findings

- `ws-dashboard/crates/daemon/src/terminal.rs#L591-606` — `write_input`: the
  exact blocking call to replace (`writer.write_all(input).and_then(|()|
  writer.flush())` under the session `std::sync::Mutex`). Fast-path checks
  (`input.len() > MAX_INPUT_BYTES`, `status != Running`, `writer` presence)
  must stay synchronous ahead of the channel enqueue.
- `ws-dashboard/crates/daemon/src/terminal.rs#L608-628` — `resize`: blocking
  `master.resize(...)` under the same mutex, then synchronous
  `inner.columns`/`inner.rows` update and `self.view()` return. Callers need
  the resulting `TerminalSessionView` back synchronously (HTTP JSON response,
  WS ack semantics), unlike `write_input`.
- `ws-dashboard/crates/daemon/src/terminal.rs#L205-214` —
  `TerminalSessionInner { writer: Option<Box<dyn Write + Send>>, master:
  Option<Box<dyn MasterPty + Send>>, child: ... }`. `writer` field must become
  a channel `Sender` handle instead of the raw `Write` object — the raw
  writer moves into the new dedicated thread closure.
- `ws-dashboard/crates/daemon/src/terminal.rs#L864-879` — `spawn_reader`:
  existing precedent for the "detached `thread::spawn`, no retained
  `JoinHandle`" pattern this phase should mirror for the writer thread (no
  `.join()` anywhere in the codebase for this style of thread — satisfies the
  "never join" shutdown constraint by construction).
- `ws-dashboard/crates/daemon/src/terminal.rs#L630-696` —
  `terminate()`/`mark_error()`/`mark_exited()`: all three currently clear
  `inner.writer = None; inner.master = None;` **before** `child.kill()`/
  `child.wait()`. This order must invert: kill/wait (and clear `master`)
  FIRST, then drop the writer channel `Sender` (`inner.writer_tx = None`) —
  killing the child (or dropping the master) is what actually unblocks a
  `write_all()` stuck on a full OS pipe buffer; dropping the sender first
  does nothing for an already-blocked syscall.
- `ws-dashboard/crates/daemon/src/terminal.rs#L723-782` —
  `terminal_socket_task` and `handle_terminal_socket_client_message`: caller
  sites for both `write_input` (via `Message::Binary` and
  `TerminalWebSocketClientMessage::Input`) and `resize` (via
  `TerminalWebSocketClientMessage::Resize`). `handle_terminal_socket_client_message`
  is currently a plain sync fn taking `&TerminalSession`; the resize arm needs
  to become async (or be special-cased) to `.await` an offload, since
  `terminal_socket_task` itself is already `async`.
- `ws-dashboard/crates/daemon/src/terminal.rs#L399-414` — HTTP `terminal_input`
  fallback: calls `session.write_input(...)` synchronously; no change needed
  beyond `write_input`'s new internals (return type/error semantics for the
  already-closed case are unchanged).
- `ws-dashboard/crates/daemon/src/terminal.rs#L416-434` — HTTP
  `terminal_resize`: calls `session.resize(...)` synchronously and expects
  `Result<TerminalSessionView, TerminalError>` back to build the JSON
  response — this is the constraint that shapes the resize offload choice
  below.
- `ws-dashboard/crates/daemon/src/git_toolbar.rs:140,156,173,213,310`,
  `resources.rs:31`, `root_picker.rs:126,227`,
  `work_root_activity.rs:125,150,174` — established `tokio::task::spawn_blocking`
  idiom already used repeatedly in this daemon for occasional/low-frequency
  blocking work; reusable pattern for `resize` (see decision below).
- `ws-dashboard/crates/daemon/Cargo.toml` / workspace `Cargo.toml` — no
  `crossbeam-channel` dependency; `tokio::sync::mpsc` is already used
  elsewhere but only from async contexts (`claude_cli.rs`, `codex_app_server.rs`).
  `std::sync::mpsc::channel()` (std, unbounded, already implicitly available,
  no new dependency) is the natural fit for a plain `std::thread` consumer
  fed from both async and non-async producers — `Sender::send()` on it never
  blocks the caller, satisfying the non-blocking-handoff constraint directly.
- `ws-dashboard/crates/daemon/tests/routes.rs` — grepped for `terminal`:
  no terminal-specific integration test file exists; today's only terminal
  coverage is the inline `#[cfg(test)] mod terminal_portability_skeleton_tests`
  at `terminal.rs#L958` (confirms the ticket's claim that `write_input`/
  `resize` are currently untested). The new writer-thread test belongs in
  this same inline module, following the `fake_terminal_session()` helper
  precedent at `terminal.rs#L1114-1133` (constructs a `TerminalSession`
  without a real PTY).

### Design decision surfaced during survey: resize offload mechanism

The ticket's "Recommended fix direction" section explicitly hedges resize
with "resize **may** route through the same per-session writer thread as the
write path" (permissive, not mandatory) — the heavier machinery was chosen
for `write_input` specifically to avoid *per-keystroke* thread/task churn.
Resize is already client-debounced to ~250ms
(`ws-dashboard/frontend/src/App.tsx` resize forwarding, per the ticket's
"Secondary N-scaling suspects" note), so that churn objection does not apply
to it. Recommended approach: keep `resize()`'s existing synchronous logic
(mutex lock, `master.resize()`, `columns`/`rows` update, `view()` return)
completely unchanged, and wrap the **call site** in
`tokio::task::spawn_blocking` (mirroring `git_toolbar.rs`/`root_picker.rs`/
`resources.rs`) instead of routing resize through the new writer-thread
channel. This fully preserves resize's existing synchronous
`Result<TerminalSessionView, TerminalError>` contract (including the
`BadRequest("terminal resize failed")` error path on real OS resize
failure) with zero new error-surfacing-locus nuance, avoids inventing a
reply/oneshot mechanism back from the writer thread, and satisfies the
completion boundary's "resize no longer blocks a Tokio worker thread"
requirement. This is a routine implementation choice within the ticket's
explicitly permissive language, not an unresolved architecture fork —
flagged here for executor visibility, not as an escalation trigger.

## Implementation Plan

1. `terminal.rs` — add a `TerminalWriterCommand` enum (`Write(Vec<u8>)` is
   sufficient per the resize decision above) and a `spawn_writer_thread(mut
   writer: Box<dyn Write + Send>) -> std::sync::mpsc::Sender<TerminalWriterCommand>`
   function near `spawn_reader` (`terminal.rs#L864-879`): `std::sync::mpsc::channel()`
   (unbounded), `thread::spawn` (no retained `JoinHandle`, matching
   `spawn_reader`'s detached style) looping `while let Ok(cmd) = rx.recv()`,
   performing `writer.write_all(&data).and_then(|()| writer.flush())` and
   breaking the loop on error (no session/mark_error coupling needed — a
   write failure correlates with process death, which the existing reader
   thread already observes and reports via `mark_error`/`mark_exited`, per
   the ticket's spec-impact note).
2. `terminal.rs#L205-214` — change `TerminalSessionInner.writer` from
   `Option<Box<dyn Write + Send>>` to `writer_tx: Option<std::sync::mpsc::Sender<TerminalWriterCommand>>`.
3. `terminal.rs#L479-539` (`TerminalSession::spawn`) — after
   `pair.master.take_writer()`, call `spawn_writer_thread(writer)` and store
   the returned sender as `writer_tx` in the constructed `TerminalSessionInner`.
4. `terminal.rs#L591-606` (`write_input`) — keep the `MAX_INPUT_BYTES` and
   `status != Running` checks synchronous; replace the `writer.as_mut()` /
   blocking write with `inner.writer_tx.as_ref()` (returning `Gone` if
   `None`, preserving the synchronous fast-path) followed by
   `let _ = writer_tx.send(TerminalWriterCommand::Write(input.to_vec()));`
   — treat a send error (writer thread already exited) as best-effort, not a
   synchronous error, matching the ticket's async-error-surfacing note.
5. `terminal.rs#L630-696` (`terminate`, `mark_error`, `mark_exited`) — reorder
   each function's body so `child.kill()`/`child.wait()` and `inner.master =
   None` happen BEFORE `inner.writer_tx = None`, per the required shutdown
   ordering (unblock the stalled write before closing the channel).
6. `terminal.rs#L608-628` (`resize`) — leave the function body itself
   unchanged (still synchronous, still locks `inner`, still calls
   `master.resize(...)`).
7. `terminal.rs#L416-434` (`terminal_resize` HTTP handler) — wrap the
   `session.resize(columns, rows)` call in
   `tokio::task::spawn_blocking(move || session.resize(columns, rows)).await`
   (session is already an `Arc<TerminalSession>` from `state.terminals.get`),
   handling the `JoinError` case with a bounded internal-error response.
8. `terminal.rs#L770-782` (`handle_terminal_socket_client_message`) — make
   this fn `async` and take `Arc<TerminalSession>` (or `&Arc<TerminalSession>`)
   instead of `&TerminalSession`; keep the `Input` arm calling
   `session.write_input(...)` directly (still sync/non-blocking); change the
   `Resize` arm to `spawn_blocking` the `session.resize(...)` call the same
   way as step 7. Update the call site in `terminal_socket_task`
   (`terminal.rs#L732`) to `.await` it and pass the already-available
   `session: Arc<TerminalSession>`.
9. `terminal.rs` inline test module (`terminal_portability_skeleton_tests`,
   near `fake_terminal_session()` at `#L1109-1133`) — add the required
   writer-thread test(s): build a `TerminalSessionInner` with `writer_tx` set
   from `spawn_writer_thread` over a test `Write` impl that records received
   chunks (e.g. into a `Arc<Mutex<Vec<Vec<u8>>>>` or forwards them over a
   second `std::sync::mpsc` channel the test polls with `recv_timeout` to
   avoid real sleeps) to assert in-order delivery across multiple
   `write_input` calls; and a second case using a `Write` impl that returns
   `Err` on write to assert the writer thread stops cleanly (no panic) and
   that `write_input` on an already-`Gone`/terminated session (after
   `terminate()`/`mark_error()`) synchronously returns `Err(Gone)` without
   touching the channel.

## Verification Plan

- `cargo build -p ws-dashboard-daemon` (or `cargo build --workspace`) —
  clean build, per completion boundary.
- `cargo test -p ws-dashboard-daemon` — full existing daemon suite must still
  pass (no terminal-specific integration test file exists today; the
  existing suite is `crates/daemon/tests/routes.rs` plus inline `#[cfg(test)]`
  modules).
- `cargo test -p ws-dashboard-daemon --lib terminal` — targeted run of the
  new/updated inline terminal tests (in-order delivery, write-error
  teardown, synchronous `Gone` fast-path).
- Manual/dogfood note (not required to close Phase 1 per the completion
  boundary, but worth calling out): the original symptom was Windows-only
  latency under load with multiple open terminals; this phase's fix is not
  mechanically verifiable via automated tests for the original perf symptom
  itself — the completion boundary is intentionally scoped to build+test
  passing plus the structural non-blocking guarantee, not a perf regression
  test (ticket's own "Autonomy note" says perf fixes lack a crisp
  deterministic regression test).

## Escalations

- None.
