---
title: Long-uptime resource-leak hardening (terminal reaper, git guards, bounded maps)
sage-review-design: required
related-mental-model:
  - ws-web-dashboard
---

# Long-uptime resource-leak hardening (terminal reaper, git guards, bounded maps)

## Background

A four-surface handle/process/subscriber-leak audit of the daemon (crates/daemon,
crates/core) — prompted by a dogfood host that had been up for days under heavy
concurrent load — found no memory bloat (footprint bounded <100MB, confirmed) and
**no git zombies** (every `git` shell-out uses `.output()`, which reaps). The real
long-uptime accumulation is in handles/processes/subscribers.

Two findings were trivial one-liners and are handled as **immediate hotfixes,
out of scope for this ticket** (see `git log` on `ws-dashboard-dev`):
- `document_events` SSE keep-alive (broadcast-receiver + socket-FD leak per
  ungraceful reconnect) — `fix(dashboard): keep-alive on document_events SSE ...`.
- codex-session auto-prune parity (orphaned live `codex app-server` children) —
  `fix(dashboard): prune codex sessions on work-root auto-prune`.

This ticket covers the remaining, non-trivial items that need design (a new
periodic reconcile task, git-invocation policy, map eviction).

## Findings (remaining)

- **[MED, unbounded on recurrence] Orphaned helper on handshake failure** —
  `terminal.rs:874-881` (`TerminalSession::spawn`). The helper is spawned
  detached (daemon holds no `Child`, learns the real pid only via the IPC
  handshake). If `connect_and_handshake` returns `None` (connect never succeeds,
  or handshake/status not received within `connect_timeout`, default 3s), spawn
  just `?`-returns `Err` and **nothing kills the helper** — the daemon cannot even
  signal it (pid unknown), it was never inserted into `sessions`, and its socket
  id is random. It has already written `<id>.json` + bound `<id>.sock`
  (`terminal_helper_process.rs:193-196`) and loops on `accept()` forever. The only
  reaper is `boot_reconcile`, which runs solely at startup. Every transient
  handshake timeout therefore leaks a permanent process + fd + registry files
  until the next daemon restart.
- **[MED] Terminal lazy-only pruning defeats the 30s grace + retains fds** —
  the daemon never proactively drops an `Exited` session; pruning is piggybacked
  on the next `create_terminal` via `insert`'s `retain(is_live)`
  (`terminal.rs:295`). While a dead session sits in the map, the daemon keeps the
  IPC socket connected, so the helper stays parked in `handle_connection`'s select
  loop and never re-evaluates its own `DAEMON_GRACE_WINDOW_MS` self-exit
  (`terminal_helper_process.rs:226-230`), directly contradicting the grace
  contract at `terminal.rs:41-45`. On the Unix EOF path `transition(Exited)`
  (`terminal_helper_process.rs:150-160`) drops the master/writer but does not
  `wait()` the shell child, so it stays a **zombie** until `kill_shell_if_running`
  runs at helper exit — now deferred to whenever the daemon finally prunes. The
  dead session also keeps its `write_half` socket fd (`terminal.rs:450`) open.
  Bounded by `MAX_TERMINAL_SESSIONS` (~16) but persists for days if terminal
  creation stops.
- **[MED] git shell-outs have no timeout and no `GIT_TERMINAL_PROMPT=0`** —
  `git_toolbar.rs` `git_text`/`run_git` (`:566`/`:579`) and mutations
  `mutate_no_body` -> `run_git` (`:265-302`). `.output()` waits forever. A stalled
  read poll (slow/hung FS, huge repo) or a mutation that hits a credential prompt
  or a hung network fetch pins the `spawn_blocking` worker thread that launched it;
  repeated over long uptime this accumulates stuck blocking-pool threads (default
  cap 512) until new blocking git calls — and thus polls — queue and stall.
- **[MED, slow] `DocumentWriteLocks.locks` grows unbounded** —
  `work_root_files.rs:66-80`. `lock_for()` inserts one `Arc<Mutex<()>>` per
  distinct `(work-root, path)` and there is **no removal path** anywhere (no
  eviction on file close, work-root unregister, or worktree removal). Grows
  monotonically with unique files ever written. Not a per-poll leak (writes only),
  so slow — but genuinely unbounded.
- **[LOW/conditional] No single-flight on concurrent polls** — the 300ms-debounced
  and 3s polls are independent HTTP requests each spawning its own
  `spawn_blocking(status_for_path)` git; with no in-flight de-dup, a slow git lets
  a new poll's child launch before the previous returns. Benign alone; amplifies
  the git-no-timeout finding into blocking-pool accumulation.
- **[LOW] Terminal WebSocket idle half-open** — `terminal.rs:1158-1227`
  (`terminal_socket_task`) sends no server-initiated ping; an idle half-open-dead
  client parks in the `select!` holding a `watch::Receiver` + socket fd. Bounded
  (terminals are opened deliberately) and self-heals on the next output send-fail,
  but prompt detection is missing.
- **[LOW] Per-request reqwest client, no timeout in SSE forwarding** —
  `servers.rs:2065,2117,2426,2469` build a fresh `reqwest::Client::new()` per call
  and `.send()` with no `.timeout(...)`. Per-request waste, not a persistent leak;
  a forwarded no-heartbeat upstream stream inherits half-open parking (doubled).

## Cross-Child Decisions

- **Root cause tying the two terminal findings: no periodic in-daemon
  reconciliation.** Pruning is purely lazy (piggybacked on `create_terminal`), and
  helper self-exit is gated on the daemon closing the IPC connection, which the
  daemon never does proactively for an `Exited` session. A single periodic
  reconcile/reaper task addresses both the orphaned-helper leak and the
  missed-grace/zombie/fd retention — prefer that over per-path patches.
- **git policy is uniform.** Apply `GIT_TERMINAL_PROMPT=0` (+ disable interactive
  askpass) and a bounded wait (spawn + `wait_timeout`/kill on expiry) at the
  `git_text`/`run_git` seam so every call site inherits it, rather than per-call.

## Phases

### Phase 1: Periodic terminal reconcile/reaper (orphan helpers + grace + fds)

Introduce a periodic in-daemon reconciliation task (not just `boot_reconcile` at
startup) that: reaps orphaned helpers left by handshake/connect failures
(scan the registry dir for `<id>.json`/`.sock` with no live session and terminate
them), proactively drops `Exited` sessions so the helper honors its
`DAEMON_GRACE_WINDOW_MS` self-exit and its shell child is `wait()`ed (no zombie),
and releases the retained `write_half` fd. Reconcile with the existing lazy
`retain(is_live)` path so the two do not double-act.

Verification boundary: after inducing a handshake timeout, no orphaned helper
process / fd / `.json`/`.sock` survives past one reconcile interval; a terminal
whose shell exits is reaped (no zombie, fd released) within the grace window
without needing a new `create_terminal`; open terminals are unaffected.

### Phase 2: git invocation hardening (timeout + no-prompt) + optional single-flight

At the `git_text`/`run_git` seam set `GIT_TERMINAL_PROMPT=0` and disable
interactive credential prompting, and add a bounded timeout (spawn + wait-with-
timeout + kill) so no git call can pin a blocking-pool thread forever. Optionally
coalesce concurrent identical polls (single-flight) to cap stacked children.

Verification boundary: a git call against a hung/prompting remote returns an error
within the timeout instead of blocking indefinitely; blocking-pool threads do not
accumulate under repeated stalled polls; a credential-required fetch fails fast
rather than hanging.

**Scope update (2026-07-26, from `260726-refactor-ws-dashboard-git-fs-watch-invalidation`
Phase 1, commit `0c48065a`).** The bounded-timeout half of this phase is
delivered: `git_exec::capture` now wraps every toolbar/discovery/Activity git
invocation with a deadline, kills the child on expiry, and drains both pipes
concurrently. `WS_DASHBOARD_GIT_TIMEOUT_MS` sets the budget (default 10 000;
`0` restores unbounded waiting). What remains of this phase as originally written
is `GIT_TERMINAL_PROMPT=0` / non-interactive credential handling and the optional
single-flight, plus the two items below that the seam introduced or left standing.

Two additions this phase should now own:

- **Detached reader threads on timeout.** When the budget expires, `capture`
  kills and reaps the direct child but does **not** join its two reader threads,
  because a descendant that inherited the pipes keeps them open and joining
  would be unbounded again. Each timeout therefore detaches two threads plus
  two pipe read handles, which end only when the pipe finally closes. Bounded by
  timeout frequency in normal operation — but an immortal descendant (an ssh
  master with `ControlPersist`, `git-credential-cache--daemon`) makes the leak
  permanent, which is exactly this ticket's failure mode. `GIT_TERMINAL_PROMPT=0`
  and non-interactive credential handling reduce how often those descendants
  exist at all, so the two items are related, not merely adjacent.
- **`kill()`/`wait()` are themselves unbounded against an unkillable child.** A
  git process wedged in uninterruptible I/O — a disconnected 9p/NFS/CIFS mount,
  which is a live risk for a daemon that runs under WSL and over network shares —
  does not die on kill, so the reaping call blocks. The seam's bound therefore
  means "bounded except an unkillable child". Deciding whether that case needs
  its own detection (rather than a deeper timeout, which cannot help) belongs
  here.

Verification boundary for the additions: after N induced timeouts against a
descendant-holding child, thread and file-descriptor counts return to baseline
once the descendant exits; an unkillable-child case is either detected and
reported or explicitly documented as out of reach.

### Phase 3: Bounded-map + half-open cleanup (DocumentWriteLocks, WS heartbeat, reqwest timeout)

Add eviction to `DocumentWriteLocks.locks` (e.g. drop the entry when its `Arc`
strong count returns to 1 after the guard releases, or prune on work-root
unregister). Add a server-initiated heartbeat/ping to the terminal WebSocket so
idle half-open connections are detected. Add a `.timeout(...)` to the SSE-forward
reqwest clients (and consider reusing a shared client).

Verification boundary: `DocumentWriteLocks` does not grow monotonically across
repeated writes to many files; an idle half-open terminal WS is detected and torn
down; a hung upstream SSE forward times out.

## Spec Impact

Target spec area: none in the workflow spec set — downstream ws-dashboard daemon
hardening, no workflow-system contract. A periodic-reconcile note may warrant a
line in the `ws-web-dashboard` mental-model/spec on landing (the terminal grace
contract at `terminal.rs:41-45` is the source of truth to reconcile with).

Contract-first spec: no.
