# Plan: 260726-refactor-ws-dashboard-long-uptime-leak-hardening — Phase 3: Bounded-map + half-open cleanup (DocumentWriteLocks, WS heartbeat, reqwest timeout)

## Relevant Ticket Contract

- Add eviction to `DocumentWriteLocks.locks`. **Prefer pruning on work-root
  unregister**; the strong-count-returns-to-1 alternative is only to be used
  if pruning-on-unregister is rejected, and if used must check+remove under
  the outer `locks` mutex (same one `lock_for()` takes) to avoid a
  concurrent `lock_for()` cloning between check and removal.
- Add a server-initiated heartbeat/ping to the terminal WebSocket so idle
  half-open connections are detected.
- Add a `.timeout(...)` to the SSE-forward reqwest clients (and consider
  reusing a shared client).
- Verification boundary: `DocumentWriteLocks` does not grow monotonically
  across repeated writes to many files; an idle half-open terminal WS is
  detected and torn down; a hung upstream SSE forward times out.
- Any daemon-initiated kill of a registry entry must route through
  `terminal_platform::kill_verified` (not applicable to this phase's items —
  no new kill paths — but stays binding background context).
- The 30s attach-grace contract (`terminal.rs:41-45`) governs eviction of
  `Exited` terminal sessions; this phase's WS heartbeat is a distinct concern
  (detecting a dead *browser* connection on a still-admits-attach session,
  not evicting the session itself) and must not interact with that grace
  window.

## Out of Scope

- Phases 1 and 2 (terminal reaper, git invocation hardening) — already
  landed, not touched.
- Phase 1's two deferred Minor findings (`SharedState::transition`
  child-reap/`kill_shell_if_running` ordering; unbounded `write_half` lock
  await in `close_ipc_connection`) — explicitly deferred to a future phase.
- Phase 2's accepted residual gaps (`credential.helper` blocking,
  unkillable-child kill/wait, `git_worktree.rs`'s direct `git` spawns
  outside the `git_exec` seam) — already dispositioned, not re-opened.
- The strong-count-returns-to-1 `DocumentWriteLocks` eviction strategy — the
  ticket states a preference for pruning-on-unregister; not implementing the
  alternate strategy.
- `OpenedWorkRoots::unregister` call sites that only roll back a
  registration that just failed to persist (`git_worktree.rs:279`,
  `root_picker.rs:276`) — these unregister an entry moments after it was
  registered in the same request, before any write route could have reached
  it, so no `DocumentWriteLocks` entries can exist for that id yet; pruning
  there is a no-op and is skipped for surgical minimalism.
- Cancelling in-flight `terminal_socket_task` connections on daemon
  shutdown — unrelated to adding a heartbeat; existing shutdown behavior for
  the browser-facing WS loop is unchanged by this phase.
- Any reqwest usage outside `servers.rs:2065/2117/2426/2469` (the four call
  sites the ticket's Findings section names) — e.g. no change to
  `codex_app_server.rs`/`claude_cli.rs` if they build their own HTTP
  clients.

## Codebase Findings

- `ws-dashboard/crates/daemon/src/work_root_files.rs:66-79` —
  `DocumentWriteLocks { locks: Arc<Mutex<HashMap<String, Arc<Mutex<()>>>>> }`
  and `lock_for()`. Key format is `format!("{}\0{}", work_root_id.as_str(),
  path)` (line 73) — a per-work-root prefix scan (`key.starts_with(&format!("{}\0",
  work_root_id.as_str()))`) is sufficient to evict every lock belonging to
  one work root. `locks` is a `tokio::sync::Mutex` (imported line 17), so
  eviction is `async`.
- `ws-dashboard/crates/daemon/src/resources.rs:30-55`
  (`local_dashboard_resources_view`) — **existing pattern to reuse.** This is
  the one funnel every `state`-bearing unregister-adjacent path already
  converges on for pruned-work-root cleanup: it takes the
  `pruned_work_root_ids` returned by `live_dashboard_resources_with_sync`
  (which internally calls `opened.unregister(...)` at `resources.rs:92`) and
  runs `state.terminals.remove_for_work_roots(&pruned)` +
  `.terminate().await`, `state.codex_sessions.remove_for_work_roots(&pruned)`,
  `state.claude_sessions.remove_for_work_roots(&pruned)` (lines 42-53). Add
  `state.document_write_locks.evict_for_work_root(id)` in the same block.
- `ws-dashboard/crates/daemon/src/git_worktree.rs:610-644`
  (worktree-remove handler) — unregisters `work_root_id` at line 611
  unconditionally (even keeps the removal on persist failure, since disk is
  already gone — comment at 617-627), then runs the same
  terminals/codex/claude cleanup pattern at lines 630-644. Add the
  `document_write_locks` eviction call alongside that block (after line 644).
- `ws-dashboard/crates/daemon/src/root_picker.rs:328-384`
  (`remove_workspace`) — unregisters every work-root id in the workspace at
  line 347-355, but only on the **success** path (persist failure
  re-registers the removed entries at lines 361-365 — rollback, unlike
  `git_worktree.rs`'s worktree-remove). The same terminals/codex/claude
  cleanup pattern runs at lines 377-381, gated behind the success path. Add
  the `document_write_locks` eviction call in that same block (iterate
  `work_root_ids`), so a failed persist does not evict locks for entries that
  get resurrected.
- `ws-dashboard/crates/daemon/src/terminal.rs:1468-1537`
  (`terminal_socket_task`) — the per-connection `tokio::select!` loop has no
  ticker arm; it only reacts to inbound `receiver.next()` frames (including
  client-initiated `Message::Ping`/`Message::Pong`, lines 1517-1520 — the
  server currently only *replies* to a client ping, never initiates one) and
  `output_signal.changed()`. `Instant` is already imported (line 4);
  `tokio::time::interval` is not yet imported in this file.
- `ws-dashboard/crates/daemon/src/terminal.rs:33-75` — existing timing
  constants (`DAEMON_GRACE_WINDOW_MS = 30_000`, `DEFAULT_CONNECT_TIMEOUT`,
  `EVICTION_BACKSTOP_GRACE`, `STALE_ENTRY_SWEEP_MARGIN`) — add the new
  heartbeat-interval/idle-timeout constants alongside these, following the
  file's existing convention of named `const` timing values with a comment.
- `ws-dashboard/crates/daemon/src/servers.rs:2049-2153` — `request_remote_sse`
  (line 2060, the actual SSE-forward path, builds client at 2065) returns a
  `RemoteSseStream` (`Pin<Box<dyn Stream<Item = Result<Bytes,
  reqwest::Error>>>>`) via `response.bytes_stream()` (line 2106) — this
  stream is consumed for the lifetime of the forwarded SSE connection, which
  is meant to run indefinitely (the source side, `document_events` in
  `work_root_files.rs:461-489`, already uses axum's
  `Sse::keep_alive(KeepAlive::default())`). `request_remote_dashboard_operation`
  (line 2110, client at 2117) reads a single bounded response body
  (`response.bytes()`, non-streaming).
- `ws-dashboard/crates/daemon/src/servers.rs:2422-2489` —
  `request_remote_link_token` (client at 2426) and `request_remote_resources`
  (client at 2469) are both single-shot, non-streaming JSON calls.
- **Risk signal (verified against vendored source, not a guess):**
  `~/.cargo/registry/src/.../reqwest-0.12.28/src/async_impl/request.rs:286-291`
  and `.../client.rs:1434-1454`. `RequestBuilder::timeout` /
  `ClientBuilder::timeout` doc: "The timeout is applied from when the
  request starts connecting **until the response body has finished**." A
  literal `.timeout(...)` on `request_remote_sse`'s client (the actual
  SSE-forward call) would therefore forcibly kill a healthy, long-lived
  forwarded stream once the deadline elapses, not just a hung one — a
  functional regression for the streaming path the ticket's Background
  section is trying to hold onto (the `document_events` SSE keep-alive fix
  is one of the two "immediate hotfixes" the ticket's Background explicitly
  calls out as already shipped and out of scope here). `ClientBuilder`
  additionally exposes `read_timeout` (`client.rs:1446-1454`): "The timeout
  applies to each read operation, and resets after a successful read. This
  is more appropriate for detecting stalled connections when the size isn't
  known beforehand" — i.e. exactly the SSE half-open case the finding
  describes, without capping total stream duration. Recommend building the
  shared client with `connect_timeout` + `read_timeout` (no total
  `.timeout()`) and using that same client for all four call sites — the
  three non-streaming calls are short-lived enough that `read_timeout`
  bounds them just as well as a total timeout would, and this avoids needing
  a second client construction just for the SSE path. `git_exec.rs:360`
  (`static GIT_TIMEOUT: OnceLock<Duration>`) is the file's existing
  process-wide-lazy-static convention to mirror for the new shared client
  (`servers.rs` does not yet import `std::sync::OnceLock`).
- `ws-dashboard/crates/daemon/Cargo.toml:28` /
  `ws-dashboard/Cargo.toml:33` — `reqwest = { version = "0.12", ...,
  features = ["json", "rustls-tls", "stream"] }`; `stream` feature is already
  enabled (needed for `bytes_stream()`, already in use).
- `ws-dashboard/crates/daemon/tests/terminal_lifetime.rs` — existing
  real-process WS integration tests use `tokio_tungstenite::connect_async`
  (lines 303, 501, 743, 984) against a real daemon+helper; this is the
  established pattern for a heartbeat-observation test (assert a `Ping`
  frame arrives within the new interval). Note: `tokio-tungstenite`'s
  `WebSocketStream` auto-replies to an inbound `Ping` with `Pong` at the
  protocol layer when its `.next()` is still being polled, so a true
  "idle half-open, never reads" client can't be simulated by simply not
  calling `.send()` — the test can only assert the ping is *sent*
  (observable via receiving `Message::Ping` frames on a client that stops
  reading further), not exercise the full idle-detection teardown
  deterministically without a lower-level socket that stops polling
  entirely (e.g. holding the raw `TcpStream` and never reading).

## Implementation Plan

1. **`DocumentWriteLocks` eviction** — `work_root_files.rs:66-80`: add
   `pub async fn evict_for_work_root(&self, work_root_id: &WorkRootId)` next
   to `lock_for`, locking `self.locks` and `retain`-ing out every key with
   prefix `format!("{}\0", work_root_id.as_str())`.
2. Call it from the three real-removal sites:
   - `resources.rs:42-53` (`local_dashboard_resources_view`): inside the
     `if !pruned_work_root_ids.is_empty()` block, alongside the
     terminals/codex/claude cleanup, `for id in &pruned {
     state.document_write_locks.evict_for_work_root(id).await; }`.
   - `git_worktree.rs:630-644` (worktree-remove handler): alongside the
     existing `ids`-scoped terminals/codex/claude cleanup, add
     `state.document_write_locks.evict_for_work_root(&work_root_id).await;`.
   - `root_picker.rs:377-381` (`remove_workspace`, success path only): add
     `for work_root_id in &work_root_ids {
     state.document_write_locks.evict_for_work_root(work_root_id).await; }`
     alongside the existing cleanup block.
3. **Terminal WS heartbeat** — `terminal.rs`:
   - Add `use tokio::time::interval;` to the imports (Duration/Instant
     already imported).
   - Add two constants near line 46-75 (e.g.
     `const WS_HEARTBEAT_INTERVAL: Duration = Duration::from_secs(15);` and
     `const WS_HEARTBEAT_IDLE_TIMEOUT: Duration = Duration::from_secs(45);`
     — a tolerance of ~2-3 missed beats; adjust to taste, no ticket-mandated
     number).
   - In `terminal_socket_task` (line 1468 onward): before the loop, create
     `let mut heartbeat = interval(WS_HEARTBEAT_INTERVAL);` and
     `let mut last_activity = Instant::now();`. Add a new `tokio::select!`
     arm: on tick, if `last_activity.elapsed() > WS_HEARTBEAT_IDLE_TIMEOUT`,
     `break` (idle half-open, tear down); otherwise
     `sender.send(Message::Ping(Vec::new().into())).await` and `break` on
     send error. Update `last_activity = Instant::now()` at the top of the
     existing `maybe_message = receiver.next() => { ... }` arm (any inbound
     frame counts as activity, including the existing `Message::Pong(_) =>
     {}` no-op arm).
4. **Shared reqwest client + timeouts** — `servers.rs`:
   - Add `use std::sync::OnceLock;`.
   - Add a module-level `static SHARED_HTTP_CLIENT: OnceLock<reqwest::Client>
     = OnceLock::new();` plus a `fn shared_http_client() -> &'static
     reqwest::Client` that lazily builds one via
     `reqwest::Client::builder().connect_timeout(...).read_timeout(...).build()`
     (mirroring `git_exec.rs:360`'s `OnceLock` convention), with no total
     `.timeout(...)` (see risk-signal finding above — a total timeout would
     kill the legitimate long-lived SSE forward).
   - Replace the four `reqwest::Client::new()` call sites
     (`servers.rs:2065, 2117, 2426, 2469`) with `shared_http_client()`.
5. Re-read the four modified call sites after the edit to confirm none of
   them relied on `reqwest::Client::new()`'s per-call default (none observed
   during survey — all four are stateless `.get`/`.post`/`.request` builder
   chains).

## Verification Plan

- `cargo build -p ws-dashboard-daemon`
- `cargo test -p ws-dashboard-daemon --lib work_root_files` (new
  `evict_for_work_root` unit test: register two work roots, `lock_for` a
  path under each, evict one, assert only that one's locks are gone via a
  small test-only accessor or by asserting `lock_for` on the evicted id
  yields a fresh `Arc` — i.e. old callers' clones are no longer reachable
  from the map).
- `cargo test -p ws-dashboard-daemon --lib terminal` plus a
  `tests/terminal_lifetime.rs` real-process test asserting a `Message::Ping`
  frame arrives on an attached WS within `WS_HEARTBEAT_INTERVAL` (bound the
  test's own wait, not the production interval, to keep it fast — consider
  a `#[cfg(test)]`-only override of the constant or an injectable interval
  if the fixed 15s is too slow for a unit test budget).
- `cargo test -p ws-dashboard-daemon --lib servers` or `--test routes` for
  the SSE-forward path — at minimum a unit test on whichever pure
  timeout/client-construction logic gets extracted (mirroring Phase 2's
  `build_ssh_command` pure-helper pattern for testability), plus a
  route-level check that `request_remote_sse`/`request_remote_dashboard_operation`
  still forward successfully with the shared client in place.
- `cargo clippy -p ws-dashboard-daemon --tests` — no new warnings.
- Manual/documented boundary if a deterministic idle-half-open WS test proves
  impractical (per the tokio-tungstenite auto-Pong finding above): document
  in the PR/commit which half of "detected and torn down" is covered by an
  automated test (ping-sent) versus asserted only by code inspection
  (idle-timeout branch).

## Escalations

- None.
