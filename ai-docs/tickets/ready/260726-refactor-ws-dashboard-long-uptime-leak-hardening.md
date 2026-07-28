---
title: Long-uptime resource-leak hardening (terminal reaper, git guards, bounded maps)
sage-review-design: completed
related-mental-model:
  - ws-web-dashboard
sage-review-completeness: completed
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
- **[MED] git shell-outs have no `GIT_TERMINAL_PROMPT=0` / non-interactive
  credential handling** — `git_toolbar.rs` `git_text`/`run_git` (`:797`/`:812`)
  and mutations `mutate_no_body` -> `run_git` (`:389-435`). As of the scope
  update below, every call already routes through `git_exec::capture` with a
  bounded timeout, so the remaining exposure is narrower than originally
  scoped: a credential-prompting remote still blocks for the full
  `WS_DASHBOARD_GIT_TIMEOUT_MS` budget (default 10s) instead of failing fast,
  and repeated stalls still tie up `spawn_blocking` worker threads for that
  window.
- **[MED, slow] `DocumentWriteLocks.locks` grows unbounded** —
  `work_root_files.rs:66-80`. `lock_for()` inserts one `Arc<Mutex<()>>` per
  distinct `(work-root, path)` and there is **no removal path** anywhere (no
  eviction on file close, work-root unregister, or worktree removal). Grows
  monotonically with unique files ever written. Not a per-poll leak (writes only),
  so slow — but genuinely unbounded.
- ~~**[LOW/conditional] No single-flight on concurrent polls**~~ — already
  covered: `GitStateCache` (`git_state_cache.rs:196-251`) releases the map lock
  before taking a per-key `Arc<Mutex<GitCacheSlot>>`, so a concurrent miss on
  the same key serializes behind the in-flight fill instead of launching a
  second git child (pinned by
  `refs_slot_single_flights_concurrent_misses_for_one_key`, `:374`). Struck
  from scope; Phase 2's "optional single-flight" sub-item is removed for the
  same reason.
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
  daemon never does proactively for an `Exited` session.
- **Periodic sweep is a backstop, not the sole mechanism — narrower fixes land
  first.** Design review (2026-07-28) noted two strictly narrower fixes the
  code already admits: (1) on a handshake-failure return, the daemon already
  knows the terminal id and registry dir, so it can read `<id>.json` for
  pid+start_time and kill immediately — race-free, no scanner needed; (2) the
  orphan only persists because `serve_connections` waits on
  `IDLE_ACCEPT_POLL` forever when no handshake ever completed
  (`terminal_helper_process.rs:224-241`) — a helper-side "no handshake within
  N seconds -> self-exit" timeout closes that leak from inside the helper,
  independent of the daemon. Also note: an orphan of this kind has not
  spawned a shell (the shell only spawns on `HandshakeAck`), so it is one idle
  process + socket + json file, not a shell/PTY leak. Phase 1 below adopts the
  narrow fixes as primary and keeps a periodic reconcile as a backstop for
  whatever they miss (e.g. a daemon crash between registry-write and
  handshake), rather than standing up the scanner as the only mechanism.
- **Any daemon-initiated kill of a registry entry MUST route through
  `terminal_platform::kill_verified`** (never a bare pid kill), per the
  `ws-web-dashboard` terminal mental model's invariant that an unverified
  identity is never killed. This applies to both the narrow handshake-failure
  kill and the backstop sweep.
- **The 30s attach-grace contract (`{#260723-terminal-attach-grace-window}`,
  `terminal.rs:41-45`) is authoritative for when an `Exited` session may be
  evicted.** Neither the lazy `retain(is_live)` path nor the new reconcile
  task may drop a session, or tear down its IPC connection, before
  `admits_attach()` goes false — doing so early is itself a bug (it already
  looks latent in the lazy path today) and Phase 1 must not inherit it.
- **git policy is uniform.** Apply `GIT_TERMINAL_PROMPT=0` (+ disable interactive
  askpass) at the `git_text`/`run_git` seam so every call site inherits it,
  rather than per-call. (The bounded-wait half of this decision already
  shipped — see Phase 2's scope update.)

## Phases

### Phase 1: Kill the handshake-failure orphan, unpark exited sessions on their own grace, backstop with a periodic sweep

Three sub-fixes, in order — the first two are the primary mechanism, the third
is a backstop:

1. **Immediate kill on handshake failure.** When `TerminalSession::spawn`'s
   `connect_and_handshake` returns `None` (`terminal.rs:874-881`), read the
   just-written `<id>.json` for pid+start_time and kill it via
   `terminal_platform::kill_verified` before returning `Err` — no scanner
   needed, the daemon already has everything it needs at that call site.
   Remove the now-stale `.sock`/`.json` registry files.
2. **Helper-side no-handshake self-timeout.** In `serve_connections`
   (`terminal_helper_process.rs:224-241`), bound how long the accept loop
   waits for a first successful handshake before self-exiting, independent of
   whether the daemon ever comes back. This closes the leak even when the
   daemon crashes or is killed between the registry write and the handshake
   attempt (the case sub-fix 1 cannot cover).
3. **Proactive teardown of `Exited` sessions, gated by the attach-grace
   contract, as a periodic backstop.** Once `admits_attach()` goes false for a
   session (i.e. its `grace_until_ms` has elapsed — never before), the
   reconcile task must (a) actually close the IPC connection — abort or
   signal `spawn_ipc_reader_task`'s task and shut down `write_half`, since
   dropping the session out of `TerminalRegistry.sessions` alone leaves that
   task's own `Arc` clone (and the fd) alive and the helper still parked in
   `handle_connection`'s select loop; closing the connection is what lets the
   helper's own `DAEMON_GRACE_WINDOW_MS` self-exit fire; (b) confirm the
   shell child was `wait()`ed (no zombie) — already true for the Windows
   reaper path, and something the Unix EOF `transition(Exited)` path still
   needs, per the existing finding. This same sweep also re-scans the
   registry dir for `<id>.json`/`.sock` with no live session as a backstop for
   sub-fix 1/2 gaps, but must exclude an entry younger than the handshake
   timeout (an in-flight `create_terminal` looks identical to an abandoned
   one until then) and must not resurrect an entry via `boot_reconcile`'s
   adopt path — a runtime sweep only kills, it never adopts.

Interval and shutdown safety: pick and state a concrete sweep interval (the
verification boundary is meaningless without one). The sweep task must be
cancelled as part of (or strictly before) the daemon's own shutdown path
(`server.rs:145`) — a sweep that runs against a partially torn-down
`sessions` map would misclassify live helpers as orphans and kill them,
breaking the mental model's "daemon exit must NOT kill terminal helpers"
invariant.

Verification boundary: after inducing a handshake timeout, no orphaned helper
process / fd / `.json`/`.sock` survives past the connect timeout (sub-fix 1) or
past the helper's own no-handshake timeout (sub-fix 2) even with the daemon
down; a terminal whose shell exits keeps its attach grace for the full 30s,
then is reaped (connection closed, no zombie, fd released) within one sweep
interval after grace expires, without needing a new `create_terminal`; open
terminals, and terminals mid-creation, are unaffected by the sweep.

### Phase 2: git invocation hardening (no-prompt + descendant-process containment)

At the `git_text`/`run_git` seam set `GIT_TERMINAL_PROMPT=0` and disable
interactive credential prompting so a credential-required fetch fails fast
instead of blocking for the full timeout budget.

**Scope update (2026-07-26, from `260726-refactor-ws-dashboard-git-fs-watch-invalidation`
Phase 1, commit `0c48065a`).** The bounded-timeout half of this phase is
delivered: `git_exec::capture` now wraps every toolbar/discovery/Activity git
invocation with a deadline, kills the child on expiry, and drains both pipes
concurrently. `WS_DASHBOARD_GIT_TIMEOUT_MS` sets the budget (default 10 000;
`0` restores unbounded waiting). What remains of this phase as originally
written is `GIT_TERMINAL_PROMPT=0` / non-interactive credential handling,
plus the two items below that the seam introduced or left standing. The
"optional single-flight" item is struck — `GitStateCache` already
single-flights concurrent misses per key (see Findings).

Two additions this phase should now own:

- **Detached reader threads on timeout — name and pick a containment
  mechanism.** When the budget expires, `capture` kills and reaps the direct
  child but does not join its two reader threads, because a descendant that
  inherited the pipes keeps them open and joining would be unbounded again.
  An immortal descendant (an ssh master with `ControlPersist`,
  `git-credential-cache--daemon`) makes this leak permanent — `GIT_TERMINAL_PROMPT=0`
  reduces how often such descendants exist, it does not close the leak
  itself. Pick one: (a) launch the child in its own process group
  (`setsid`/`killpg` on Unix, a Job Object on Windows) and tear down the
  whole group on timeout, so the descendant dies with the parent instead of
  inheriting orphaned pipes; or (b) cap the number of outstanding detached
  reader threads and refuse/queue new git calls past the cap; or (c)
  explicitly accept the leak as documented behavior (`git_exec.rs:575-600`
  already frames this as a deliberate tradeoff) and close this item as
  "won't fix, tracked". A boundary phrased as "counts return to baseline once
  the descendant exits" is true by construction today and must not be reused
  verbatim — the boundary has to demonstrate the chosen mechanism actually
  bounds an *immortal* descendant, not merely observe an ordinary one
  finishing.
- **`kill()`/`wait()` are themselves unbounded against an unkillable child.** A
  git process wedged in uninterruptible I/O — a disconnected 9p/NFS/CIFS mount,
  which is a live risk for a daemon that runs under WSL and over network shares —
  does not die on kill, so the reaping call blocks. The seam's bound therefore
  means "bounded except an unkillable child". Decide whether that case needs
  its own detection (a deeper timeout cannot help) or is explicitly documented
  as out of reach.

Verification boundary: a git call against a hung/prompting remote returns an
error within the timeout instead of blocking indefinitely, and a
credential-required fetch fails fast rather than hanging (no-prompt half);
for the process-group/cap mechanism chosen above, an *immortal* descendant
left behind by N induced timeouts is either terminated with its parent or
capped rather than accumulating without bound — or the leak is explicitly
documented as accepted with no code change; an unkillable-child case is
either detected and reported or explicitly documented as out of reach.

### Phase 3: Bounded-map + half-open cleanup (DocumentWriteLocks, WS heartbeat, reqwest timeout)

Add eviction to `DocumentWriteLocks.locks`. Prefer pruning on work-root
unregister; if instead evicting on strong-count-returns-to-1, the count check
and the removal must happen while the outer `locks` map mutex is held (the
same mutex `lock_for()`, `work_root_files.rs:72-79`, takes to hand out a new
clone) — checking outside that lock lets a concurrent `lock_for()` clone the
entry between check and removal, leaving two writers on the same
`(work-root, path)` holding *different* mutexes and losing the serialization
the map exists for (`{#260524-ws-dashboard-document-edit-save-fanout}`). Add
a server-initiated heartbeat/ping to the terminal WebSocket so idle
half-open connections are detected. Add a `.timeout(...)` to the SSE-forward
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
