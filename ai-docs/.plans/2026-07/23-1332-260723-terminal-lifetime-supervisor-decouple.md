# Plan: 260723-feat-dashboard-terminal-lifetime-daemon-decouple — Phase 1: Server-side per-terminal supervisor decoupling

## Relevant Ticket Contract

- Completion: a live terminal survives a daemon restart and reattaches via
  the existing frontend resume-by-id path, verified by an acceptance test;
  plus a test proving reconcile rows 3 and 5 (identity-mismatch / PID-reuse)
  never kill an unverified process (drop-entry-only must be exercised, not
  just asserted in prose).
- **Decision A (pinned, binding):** IPC framing = NDJSON over a dedicated
  helper-facing message enum (separate Rust types from the browser-facing WS
  types), transport = Unix domain socket / Windows named pipe, reusing the
  `codex_app_server.rs` `AsyncBufReadExt::lines()` + `serde_json` pattern
  (zero new framing crate).
- **Decision B (pinned, binding):** both Unix and Windows land in Phase 1
  (no Phase-2 split). Unix (setsid/double-fork detach, pidfd-gated kill) is
  E2E-verified in this session. Windows (helper-owned Job Object +
  `CREATE_BREAKAWAY_FROM_JOB` + stable OpenProcess-handle kill) is
  implemented, cross-compile-checked, and unit-tested in this session; live
  native-Windows E2E is deferred to the user's dogfooding host. Both legs sit
  behind the existing `TerminalPlatform` abstraction.
- Helper owns a bounded output ring (NOT pure stdio forwarding); helper is
  the authoritative output buffer, daemon is a thin proxy; cursor becomes a
  helper-owned monotonic sequence. Memory-only, no on-disk text persistence.
- Registry is a directory, one file per terminal:
  `<runtime-dir>/terminals/<termid>.json`, mode 0600, atomic writes
  (temp-rename). Helper owns create-on-spawn/delete-on-exit of its own
  entry; daemon only prunes entries it has positively confirmed dead.
- Identity = OS-queryable PID + start-time ONLY (precondition for kill); a
  nonce is explicitly rejected as a kill-gate (unverifiable when IPC is
  dead, which is exactly when the kill decision is made).
- 2-tier kill: prefer graceful IPC request (helper `child.kill()`s its shell
  and exits); fall back to verified-PID kill only when IPC is unreachable.
  Fallback captures a stable OS handle (Linux `pidfd`, Windows process
  handle) at verification time and kills through that handle, not by
  re-resolving the PID (closes the verify-to-kill TOCTOU race). Windows:
  helper-owned Job Object (not daemon-owned), shell spawned with
  breakaway-from-job semantics.
- Registry-write ordering: helper spawns the shell only AFTER its own
  registry entry is durably written (atomic temp-rename) AND its PID has
  been handshaked back to the daemon over IPC — closes the orphan-leak
  window.
- Keep `MAX_TERMINAL_SESSIONS = 16`, enforced after reconcile counts adopted
  live sessions.
- 6-row boot reconcile table (IPC reachable / PID+identity match / shell
  alive) — see ticket `## Boot reconcile policy`; three-line invariant:
  Adopt = IPC-reachable && identity-ours; Kill = identity-verified-ours &&
  IPC-dead; unverified identity → NEVER kill, drop entry only.
- Reconcile completes BEFORE accepting new terminal opens and BEFORE serving
  the session list to clients. Per-entry IPC connect timeout ~250-500ms
  (degrades to row 4 on timeout). Duplicate entries: keep the one passing
  handshake, drop the rest. Whole-file parse failure → start fresh with a
  loud warning; single-entry parse failure → skip that entry only.
- Grace-reattach (row 2): hold the ring ~30s or until one reattach, deliver
  last output + exit code, then self-exit. Both the WS attach `GONE` gate
  (`terminal.rs` `terminal_websocket`, `is_live()` check) and the list
  filter (`TerminalRegistry::list_for_work_root`,
  `.filter(... && is_live())`) must be relaxed so an in-grace exited session
  stays visible/attachable.
- Non-goals: no on-disk text persistence; frontend already has the
  resume-by-id path (no frontend changes needed); no plugin version bump
  (downstream application code).

## Out of Scope

- Frontend changes — `terminals.ts`/`terminalPaneBody.tsx` resume-by-id path
  already exists and is unmodified by this phase.
- Live native-Windows E2E acceptance — explicitly deferred to the user's
  dogfooding host per Decision B; this phase only needs cross-compile-check
  + unit tests for the Windows leg.
- On-disk scrollback/text persistence (explicit non-goal).
- Spec document edits (`ws-web-dashboard/index.md` anchors
  `260523-ws-dashboard-terminal-tab-restore`,
  `260516-ws-web-dashboard-terminal-registry-pty-spawn`) — deferred to
  implementation's doc pre-pass per the ticket's Spec Impact section; do not
  edit specs in this phase's execution either without going through that
  pre-pass.
- Exact grace-window tuning beyond a reasonable default (~30s) — ticket
  leaves this as an open value, not a blocking decision.
- Any Phase-2 split — explicitly not taken (Decision B).

## Codebase Findings

**Cut boundary / current owner of PTY lifetime**
- `ws-dashboard/crates/daemon/src/terminal.rs:195-214` — `TerminalSession`
  (id, work_root_id, title, cwd_hint) / `TerminalSessionInner` (status,
  columns, rows, `output: VecDeque<TerminalOutputChunk>`, `next_sequence`,
  `writer_tx`, `master: Option<Box<dyn MasterPty>>`,
  `child: Option<Box<dyn Child>>`) — this is the struct whose PTY-owning
  fields (`master`, `child`, `writer_tx`, reader thread, `output` ring) move
  into the helper process; `TerminalSession`'s metadata fields
  (`id`/`work_root_id`/`title`/`cwd_hint`/`created_at_ms`) plus a new
  identity+IPC-handle stay daemon-side.
- `terminal.rs:521-581` (`TerminalSession::spawn`) — current in-process
  `openpty` + `spawn_command` + reader/writer thread wiring; becomes the
  helper-side spawn logic (moved into the new helper binary path), replaced
  daemon-side by "launch detached helper process, wait for handshake".
- `terminal.rs:944-959` (`spawn_reader`) — reader thread pushing into
  `TerminalSessionInner.output` via `append_output`; moves into the helper
  (helper owns the ring per Decided design).
- `terminal.rs:695-770` (`terminate`/`mark_error`/`mark_exited`) — the
  documented kill/wait-before-drop-channel ordering constraint stays
  *inside the helper* (the helper is now the process doing `child.kill()`);
  the daemon-side equivalent becomes "send IPC graceful-kill, else
  verified-PID kill via stable handle."

**IPC framing precedent (Decision A reuse target)**
- `ws-dashboard/crates/daemon/src/codex_app_server.rs:360-368` (reader loop)
  and `:247-262` (`write_message`) — exact NDJSON pattern to mirror:
  `BufReader::new(reader).lines()` / `next_line()` on the read half,
  `serde_json::to_string(&msg)` + `\n` + `write_all` + `flush` (under an
  `AsyncMutex`) on the write half. Zero new framing crate; this is the
  explicitly-cited reuse target.
- `codex_app_server.rs:71-131` (`CodexIncoming`/`classify_incoming`) is a
  *different* concern (JSON-RPC three-way classification) — do NOT reuse
  this shape; the ticket mandates a dedicated, separate helper message enum
  (not a fork of `TerminalWebSocketServerMessage`/`ClientMessage`, and not
  the JSON-RPC shape either — a plain tagged enum is enough since the
  helper protocol is a private 1:1 control channel, not JSON-RPC).
- `codex_app_server.rs:765-795` (`spawn_connection`) — precedent for
  `tokio::process::Command` with piped stdio + `kill_on_drop(true)`; NOT
  reusable as-is for the helper spawn, because the helper must be
  **detached** (survive daemon exit) — the opposite of `kill_on_drop`. Cite
  only for the general `Command`-building shape, not the lifecycle
  semantics.

**Registry-file identity model / atomic writes**
- `ws-dashboard/crates/daemon/src/persistent_state.rs:433-450`
  (`write_state_json`) — exact atomic temp-rename pattern to reuse:
  `fs::write(path.with_extension("json.tmp"), raw)` then
  `fs::rename(temp_path, path)`. Directly reusable for the per-terminal
  registry file writer (add 0600 permission-setting on Unix, which this
  existing helper does not currently do).
- `persistent_state.rs:485-509` (`default_state_file`/`default_state_dir`)
  — existing resolution order (`WS_DASHBOARD_STATE_FILE` >
  `WS_DASHBOARD_STATE_HOME` > `XDG_STATE_HOME` > `HOME` >
  `LOCALAPPDATA`-on-Windows). The natural `<runtime-dir>/terminals/` the
  ticket calls for is `default_state_dir()?.join("terminals")`; add a
  sibling accessor (mirrors how `logging.rs:109-110` already reuses
  `default_state_dir()` for `logs/`) rather than inventing a second
  resolution order.
- `persistent_state.rs:19-42` (`DashboardStateStore::at_path`/
  `default_local`) — pattern for an injectable-path constructor so tests
  don't need process-global env-var mutation; the new terminal registry
  directory handle should offer the same `at_path`-style override for test
  isolation instead of relying on `WS_DASHBOARD_STATE_HOME` (which
  `persistent_state.rs:692-718`'s existing tests show is process-global and
  needs careful save/restore — avoid adding more callers of that pattern).

**Existing `TerminalPlatform` abstraction (Decision B's stated home)**
- `terminal.rs:35-39` (`TerminalPlatform` enum, `Unix`/`Windows`) and
  `terminal.rs:1020-1029` (`default_shell()`, `#[cfg(windows)]`/
  `#[cfg(not(windows))]` dispatch) — this is the exact "existing
  `TerminalPlatform` abstraction" the ticket says both detach/kill legs must
  sit behind. Today it only selects a shell; extend it with the
  spawn-detached/verify-identity/kill-through-handle operations using the
  same cfg-gated-module-behind-a-platform-enum shape (pure/testable core,
  syscalls isolated behind `#[cfg(...)]`).

**Boot sequence — where reconcile must run**
- `ws-dashboard/crates/daemon/src/server.rs:75-102` —
  `run_with_shutdown_and_grace` currently does `TerminalRegistry::default()`
  (empty) then `build_router`. This is precisely where boot reconcile must
  run and complete *before* `build_router`/`axum::serve` starts accepting
  connections, matching the ticket's ordering requirement. `opened_work_roots`
  is already loaded from `dashboard_state` at this exact point (lines
  76-86) — the terminal registry reconcile should follow the same
  "load-then-construct-AppState" shape.

**Gates that must relax for grace-reattach (row 2)**
- `terminal.rs:143-151` (`TerminalRegistry::list_for_work_root`) —
  `.filter(|s| ... && session.is_live())` excludes anything not `Running`;
  must additionally admit an in-grace exited session.
- `terminal.rs:496-498` (`terminal_websocket` upgrade gate) —
  `if !session.is_live() { return ... GONE }` — same relaxation needed for
  one final reattach.
- `TerminalStatus` (`terminal.rs:323-330`, `Running`/`Exited`/`Terminated`/
  `Error`) has no "in grace" state today; either add one or track grace via
  a separate timestamp/flag alongside `status` so `is_live()` semantics for
  *ordinary* (non-grace) exited/terminated/error sessions do not change
  everywhere else `is_live()` is used (`write_input`, `resize`,
  `create_terminal`'s eviction check via `insert`'s `retain(is_live)`).

**Risk signal — `remove_for_work_roots` cleanup relies on Drop, which
breaks under detachment (not called out in the ticket, found by tracing
call sites)**
- `terminal.rs:181-192` (`TerminalRegistry::remove_for_work_roots`) only
  removes entries from the in-memory map; it does **not** call
  `terminate()` on the removed sessions. Today this "works" only because
  dropping the last `Arc<TerminalSession>` drops `TerminalSessionInner.master`
  (`Box<dyn MasterPty>`), and closing the PTY master fd delivers SIGHUP to
  the still-running shell child — the same drop-closes-master mechanism
  this ticket's Problem section identifies as the daemon-restart bug, just
  triggered here by workspace/worktree removal instead of process exit.
  Call sites: `git_worktree.rs:581` (worktree remove-submit route, async
  handler), `resources.rs:36` (`local_dashboard_resources_view`, async,
  called from the canonical resources route on every poll when work roots
  get pruned), `root_picker.rs:367` (workspace remove route, async
  handler). **After this refactor, dropping the daemon-side proxy struct
  does nothing to a detached helper** — the helper still owns the PTY
  master and will keep running orphaned. `remove_for_work_roots` must
  change from "drop and rely on implicit close" to "explicitly request
  kill (IPC graceful, falling back to verified-PID) for each removed
  session," e.g. by returning the removed `Arc<TerminalSession>`s (or
  draining a `Vec`) and having each of the three async call sites
  `tokio::spawn` a best-effort kill task per removed session so route
  latency is unaffected. This is a **shortcut risk**: an implementation
  that ports `remove_for_work_roots` unchanged (relying on Drop) will
  silently leak live shells on every workspace/worktree removal once the
  PTY moves out-of-process.

**Risk signal — helper binary path resolution must not hardcode
`current_exe()` for testability**
- No existing precedent in this repo for spawning
  `std::env::current_exe()` as a detached subprocess. `main.rs:1-19` shows
  the single `[[bin]]` target is `ws-dashboard`
  (`crates/daemon/Cargo.toml:8-9`, `name = "ws-dashboard"`). Inside
  `crates/daemon/tests/routes.rs` (the existing integration test binary),
  `std::env::current_exe()` returns the *test* binary, not `ws-dashboard`
  — so daemon code must resolve the helper executable through an
  overridable path (e.g. an env var the daemon reads, defaulting to
  `current_exe()`), not a bare `current_exe()` call. Cargo automatically
  sets `CARGO_BIN_EXE_ws-dashboard` at test-binary build time for
  integration tests in a crate with a matching `[[bin]]` target (confirmed
  present: `crates/daemon/Cargo.toml` has exactly that `[[bin]] name =
  "ws-dashboard"` target) — this is the mechanism the acceptance test
  needs to point the daemon-under-test at the real compiled binary for
  `terminal-helper` re-exec, and it is also the cleanest way to run the
  "survive daemon restart" acceptance test as two genuinely separate
  `ws-dashboard serve` OS processes (kill process 1 for real, start process
  2 pointed at the same state dir) rather than only dropping in-process
  daemon state, which would not prove the real "OS actually tore down the
  parent process" scenario the ticket cares about.
- `cli.rs:18-21` (`Command::Serve`) is the only subcommand today — the
  hidden helper entrypoint (e.g. `Command::TerminalHelper(..)`, not
  documented in `--help`/the remote-deployment guide) is a new variant
  here, dispatched from `main.rs` before/alongside the existing `serve`
  path.

**Dependencies available for the platform-specific detach/kill legs**
- `Cargo.lock` shows `nix 0.25.1` and `libc 0.2.186` as *transitive* deps
  only (via `portable-pty`/`mio`), not direct dependencies of
  `crates/daemon`. `nix 0.25` predates its `PidFd` wrapper (added later),
  so either bump to a direct `nix` dependency new enough to have safe
  `pidfd` wrappers, or add `libc` as a direct dependency and call
  `libc::syscall(libc::SYS_pidfd_open, pid, 0)` /
  `libc::syscall(libc::SYS_pidfd_send_signal, fd, sig, ...)` directly (both
  syscall-number constants are present in `libc` 0.2.186 for Linux/glibc
  targets). Either is a small, contained addition — flag for the executor
  to pick one, not a strategy fork.
- `windows-sys` is present transitively at multiple versions
  (`Cargo.lock:1997,2006,2015`) but not a direct dependency; the Windows
  leg needs a direct `[target.'cfg(windows)'.dependencies] windows-sys`
  (or `windows`) entry with `Win32_System_JobObjects`,
  `Win32_System_Threading`, `Win32_Foundation` features for
  `CreateJobObjectW`/`SetInformationJobObject`/`AssignProcessToJobObject`/
  `OpenProcess`. Breakaway-from-job process creation itself does **not**
  need raw `CreateProcessW`: `std::os::windows::process::CommandExt::
  creation_flags(CREATE_BREAKAWAY_FROM_JOB)` on `std::process::Command` is
  stable and sufficient.

**Wire-contract stability (must NOT change)**
- `terminal.rs:287-321` (`TerminalWebSocketServerMessage`/
  `TerminalWebSocketClientMessage`) and `terminal.rs:252-280`
  (`TerminalSessionView`/`TerminalOutputChunk`) are the browser-facing
  contract; Decision A explicitly requires the helper protocol to be
  *separate* Rust types so browser wire changes cannot bleed in.
  Linked-server forwarding (`servers.rs` `server_scoped_terminal_*`,
  `linked_server_terminal_websocket_relays_real_two_daemon_io` test at
  `crates/daemon/tests/routes.rs:4649`) proxies these same routes/types
  unmodified, so it should be transparent to this phase as long as the
  browser-facing types and route behavior are preserved.

**Mental-model doc staleness (flag only, do not edit)**
- `ai-docs/mental-model/ws-web-dashboard.md:141` ("Terminal tab restore is
  browser descriptor replay, not daemon session resume... daemon restarts
  only allow new sessions from safe workRoot-relative cwd hints") and the
  matching Common Mistake at `:207` both encode the pre-Phase-1 invariant
  that a daemon restart never leaves a resumable PTY. This phase makes the
  "adopt" reconcile row (row 1/2) a real resumption path, contradicting
  both lines. Per the render task's instruction, this is a doc-amendment
  target for the implementation's doc pre-pass — do not edit here.

## Implementation Plan

**Stage 1 — helper process + NDJSON IPC + registry-file identity + 6-row
reconcile + Unix detach (E2E-verified in this session)**

1. Add a hidden `Command::TerminalHelper` variant to `cli.rs` (args:
   registry dir, terminal id, work-root id, cwd, columns/rows,
   socket/pipe path) and dispatch it from `main.rs` before the normal
   `serve` path; keep it undocumented (no help text) since it's an
   internal re-exec target, mirror `ServeArgs`' `#[arg]` style.
2. New module(s) for the helper-facing protocol, e.g.
   `crates/daemon/src/terminal_helper_protocol.rs`: dedicated
   `HelperToDaemonMessage` (`Handshake{pid, start_time}`,
   `Output{sequence, data}`, `Status{...}`, `Exit{...}`) and
   `DaemonToHelperMessage` (`Input{data}`, `Resize{columns,rows}`,
   `RequestBackfill{after}`, `GracefulShutdown`) tagged enums, following
   `codex_app_server.rs:287-321`'s `#[serde(tag = "type", rename_all =
   "camelCase")]` shape but as separate types per Decision A.
3. New module `crates/daemon/src/terminal_helper_ipc.rs` (or fold into the
   protocol module): NDJSON reader/writer over `tokio::net::UnixStream`
   (Unix) reusing the exact `BufReader::new(...).lines()` /
   `write_all(line + "\n")` shape from `codex_app_server.rs:360-368,247-262`;
   keep the transport generic over `AsyncRead + AsyncWrite` so the same
   code serves both the Unix-socket and (Stage 2) named-pipe transports.
4. New module `crates/daemon/src/terminal_helper_process.rs` — the helper
   binary's own runtime: `openpty`/`spawn_command` (move from
   `terminal.rs:521-581`), the reader thread → ring buffer (move
   `terminal.rs:944-959`'s reader logic; bound with the existing
   `MAX_OUTPUT_CHUNKS`), the writer thread (move `terminal.rs:230-250`),
   and an IPC server loop that: (a) on Unix, calls `setsid()`/double-fork
   inside spawn setup (daemon side, see step 6) so this helper process runs
   already detached by the time it reaches this code; (b) writes its own
   registry file entry (atomic temp-rename, 0600) with its **real** PID
   (post double-fork) + start-time BEFORE opening the PTY/spawning the
   shell; (c) binds the IPC listener and only after a client (the daemon)
   connects and is handshaked, spawns the shell; (d) on shell exit, holds
   the grace window (~30s or one reattach) delivering buffered output +
   exit code, then deletes its registry file and exits.
5. New module `crates/daemon/src/terminal_registry_file.rs`: the
   `<state_dir>/terminals/<termid>.json` entry type (`terminal_id`,
   `work_root_id`, `pid`, `start_time`, `socket_path`, `created_at_ms`,
   `title`, `cwd_hint`, `columns`, `rows`) plus atomic write (reuse
   `persistent_state.rs:433-450`'s temp-rename shape, add 0600 via
   `std::os::unix::fs::PermissionsExt` on Unix) and a directory-scan reader
   that tolerates single-entry parse failures (skip, keep the rest) and
   whole-directory-unreadable failure (start fresh, loud warning).
   Directory path resolves via a new sibling to `persistent_state.rs:508`
   `default_state_dir()` (e.g. `default_state_dir()?.join("terminals")`),
   overridable the same injectable-path way `DashboardStateStore::at_path`
   is, for test isolation without global env-var mutation.
6. Daemon-side spawn path (replaces `TerminalSession::spawn`,
   `terminal.rs:521-581`): build the detached helper `Command` — Unix leg
   under `#[cfg(unix)]` in `terminal_platform/unix.rs` — using
   `std::os::unix::process::CommandExt::pre_exec` to call `libc::setsid()`
   (plus the double-fork technique so the `Command::spawn()`-returned PID
   is a short-lived middle process, not the long-lived helper — the daemon
   must NOT trust `Child::id()` as final identity), pointed at the
   resolved helper binary path (`current_exe()` in production, overridable
   via env var for tests — see Codebase Findings), then wait on the IPC
   handshake (bounded timeout) to learn the helper's real PID + start-time
   before returning success to `create_terminal`.
7. Rewrite `TerminalSession`/`TerminalSessionInner` (`terminal.rs:195-214`)
   into a thin daemon-side proxy: keep `id`/`work_root_id`/`title`/
   `cwd_hint`/`created_at_ms`; replace `master`/`child`/`writer_tx`/
   `output` with `pid`, `start_time`, `ipc: <connection handle>`, and a
   small local mirror of the most recent status/next_sequence for
   `view()`/`is_live()` without needing an IPC round-trip per HTTP call.
   `write_input`/`resize`/`terminal_output` (`terminal.rs:435-476,614-630`)
   become IPC request/response calls instead of local ring/PTY operations;
   `terminal_socket_task` (`terminal.rs:773-842`) keeps its existing shape
   but sources output from IPC-forwarded `Output` messages instead of the
   local `output_signal` watch (or keeps a local `watch` fed by an
   IPC-reader background task — preserves the existing WS task
   structure/tests as much as possible).
8. Kill path: graceful IPC `GracefulShutdown` request first; on
   unreachable/timeout, verified-PID kill through a captured `pidfd` (Unix
   leg: `libc::syscall(SYS_pidfd_open, ...)` then
   `SYS_pidfd_send_signal`) — implement in `terminal_platform/unix.rs`,
   gated by an identity check (PID + start-time match the registry entry)
   performed immediately before capturing the pidfd, per the TOCTOU
   constraint.
9. Fix the `remove_for_work_roots` Drop-reliance risk signal: change
   `TerminalRegistry::remove_for_work_roots` (`terminal.rs:181-192`) to
   return the removed `Arc<TerminalSession>`s (or equivalent), and update
   the three async call sites (`git_worktree.rs:581`, `resources.rs:36`,
   `root_picker.rs:367`) to `tokio::spawn` a best-effort kill task per
   removed session using the same graceful-then-verified-kill path as
   `close_terminal`.
10. Boot reconcile: new function (e.g.
    `TerminalRegistry::boot_reconcile(registry_dir, platform) -> TerminalRegistry`)
    called from `server.rs` (`run_with_shutdown_and_grace`, replacing the
    bare `TerminalRegistry::default()` at line 93) BEFORE `build_router` —
    scans the registry directory, applies the 6-row table per entry with a
    bounded ~250-500ms IPC connect timeout, adopts rows 1/2 into the live
    `TerminalRegistry`, kills-then-drops row 4, drops-only rows 3/5/6, and
    handles duplicate-entry / malformed-file cases per the ticket's
    ordering rules. `MAX_TERMINAL_SESSIONS` enforcement in `insert`
    (`terminal.rs:161-172`) needs no change since it already counts
    current registry size at insert time — reconcile just needs to insert
    adopted sessions before any `create_terminal` call can race it
    (already guaranteed by "reconcile completes before build_router").
11. Relax the two `is_live()` gates for grace-reattach (row 2) —
    `terminal.rs:143-151` list filter and `terminal.rs:496-498` WS upgrade
    gate — by introducing an explicit "in grace" predicate distinct from
    `is_live()`'s existing `Running`-only check, so other `is_live()`
    call sites (`write_input`, `resize`, eviction `retain`) keep today's
    stricter semantics.
12. Extend the `TerminalPlatform` enum's existing cfg-dispatch shape
    (`terminal.rs:35-39,1020-1029`) with the new spawn-detached /
    verify-identity / kill-through-handle operations so both platform legs
    share one abstraction boundary, per the ticket's explicit instruction.

**Stage 2 — Windows detach leg (implemented + cross-compile-checked + unit
tested here; live E2E deferred to user dogfood)**

13. Add `[target.'cfg(windows)'.dependencies] windows-sys = { ... }` (or
    `windows`) to `crates/daemon/Cargo.toml` with `Win32_System_JobObjects`,
    `Win32_System_Threading`, `Win32_Foundation` features.
14. `terminal_platform/windows.rs`: helper-owned Job Object creation
    (`CreateJobObjectW`), `SetInformationJobObject` with
    `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` so killing the helper reliably
    takes the shell subtree, `AssignProcessToJobObject` for the shell
    child, and spawn the shell via `std::process::Command` +
    `CommandExt::creation_flags(CREATE_BREAKAWAY_FROM_JOB)` so the shell
    process detaches from any job the *helper* itself might be in (keeping
    the helper-owned Job Object as the only one that matters).
15. Windows kill path: `OpenProcess(PROCESS_TERMINATE | SYNCHRONIZE, ...)`
    captured at verification time (mirrors the pidfd TOCTOU-closing
    approach), `TerminateProcess` through that handle.
16. Windows helper detach spawn (daemon side): no setsid equivalent needed;
    rely on the Job Object breakaway design instead, spawned via
    `tokio::net::windows::named_pipe` for the IPC transport (already
    available through the existing `tokio` `net` feature, no extra crate).
17. `cargo check --target x86_64-pc-windows-gnu` (or whichever Windows
    target toolchain is available in this session) across the daemon crate
    to prove the cfg-gated Windows code compiles; unit tests for the pure
    logic (registry entry parsing, reconcile row classification, message
    (de)serialization) already run cross-platform since Stage 1's core is
    platform-agnostic — only the syscall-touching leaves are Windows-cfg'd.

## Verification Plan

- `cargo test -p ws-dashboard-daemon` (unit tests for
  `terminal_registry_file`, reconcile row classification, NDJSON message
  (de)serialization, platform-neutral proxy logic).
- New Unix E2E acceptance test in `crates/daemon/tests/routes.rs` (or a new
  `crates/daemon/tests/terminal_lifetime.rs`): boot a first daemon instance
  (in-process `server::run_with_shutdown_and_grace`, pointed at an isolated
  temp state dir via the injectable registry-dir override), create a
  terminal, write/read a marker string through it to prove the shell is
  live, drop/kill the first daemon instance **without** invoking graceful
  terminal close (simulating a real restart, not a clean shutdown), boot a
  second daemon instance pointed at the same state dir, assert: (a) the
  terminal appears in `list_terminals` for the work root, (b) a WS reattach
  with the prior cursor delivers continuity (no duplicate/missing chunks),
  (c) writing a new marker through the reattached session still reaches the
  live shell. Use `env!("CARGO_BIN_EXE_ws-dashboard")` (Cargo-provided,
  confirmed available given `crates/daemon/Cargo.toml`'s `[[bin]]` target)
  to point the daemon-under-test's helper-binary resolution at the real
  compiled binary, since `std::env::current_exe()` inside the test binary
  would otherwise resolve to the test binary itself.
- Reconcile row 3/5 test (identity-mismatch / PID-reuse never kills):
  construct a registry entry whose PID+start-time do not match any process
  the helper would have started (e.g. point it at a real but foreign PID
  with a start-time that cannot match), run reconcile, assert the entry is
  dropped from the registry AND assert (via a process-liveness check on
  that foreign PID, or a spy/mock kill-handle in a unit-level version of
  reconcile) that no kill syscall was attempted — this must be a real
  exercised assertion per the ticket's completion bar, not only a prose
  invariant.
- `cargo check --target <windows target>` for the Stage 2 Windows leg, plus
  targeted unit tests for the Windows-specific pure logic that can run
  without a live Windows host (message framing, registry entry model,
  reconcile classification are already platform-neutral from Stage 1).
- Manual/dogfood-only: live native-Windows daemon-restart survival, to be
  completed on the user's Windows dogfooding host per Decision B (out of
  this session's verification boundary).

## Escalations

- None.
