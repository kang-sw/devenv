---
title: Tracker page misses writes from other processes until the poll
related:
  261108-feat-ws-tracker-support-agent-chat: the support chat whose writes do not show
  261108-feat-ws-tracker-drawer-ui: recorded minor - View waits behind the network lock
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 16f46749ccaf97a9
sage-review-completeness-reviewed: 16f46749ccaf97a9
---

# Tracker page misses writes from other processes until the poll

## Background

User report (board, 2026-11-09, dogfooding the support agent): the support
chat created a topic with three tasks, but the tracker's Items panel stayed
empty; the page did not refresh.

The support agent's `tracker_update` runs in a child
`ws-mcp serve --stdio --root <store>` process (`newAgentRunner` in
`agents-plugin-tool/cmd/ws-mcp/dashboard.go`), which writes the store
directly. The daemon bumps its tracker revision (`Daemon.TrackerChanged`,
`internal/wsdashboard/tracker_routes.go`) only from its own write routes and
from the background poll's round callback (`startTrackerPoll`, default
interval 3 minutes, `internal/wstrackerhost`). A write from any other process
(the support agent's ws-mcp child, a lead session's `tracker_update`, another
session) therefore reaches the page only on the next poll, up to about three
minutes later.

A second cause makes a refresh look empty even when it is triggered: the
host's `View` takes the store lock (`st.mu`, `Host.with`) that a remote fetch
(`attempt`, up to 60 s) also holds, so a re-read can wait behind an in-flight
fetch. This was already recorded as a minor on
`261108-feat-ws-tracker-drawer-ui`.

## Decisions

1. **Both notification paths (board 138 c).**
   - *Runner hook:* when the support chat runner sees a `tracker.update` tool
     result (the same place it parses `tracker.show` results into cards,
     `cardEvent` / `CardTool` in `internal/wsagent/stream.go`), it notifies the
     daemon, which calls `TrackerChanged`. Support-chat writes show at once.
   - *Store watch:* each registered store directory's ledger state (journal
     and ledger refs only) is checked on a short interval of a few seconds,
     and `TrackerChanged` is called when it changes. Every other external
     writer shows within seconds. The watch lives in `wstrackerhost` (a
     `StartPoll`-shaped method, e.g. `Host.StartWatch(interval, onChange)`)
     and is wired in `cmd/ws-mcp` next to `startTrackerPoll` with
     `daemon.TrackerChanged`, because wsdashboard stays tracker-free (prior
     decision of `261108-feat-ws-tracker-drawer-ui`).
   Rejected: the store watch alone (a; leaves the chat the user is in a few
   seconds behind); the runner hook alone (b; misses lead-session and
   other-session writes).
2. **Lock hardening in the same ticket (board 139), contained in
   wstrackerhost (board 140 b).** The real wait is not `st.mu` alone:
   `Ledger.Fetch` / `Sync` run inside `wsledger.(*Ledger).withLock`, which
   holds the in-process `l.mu` and the cross-process `<store>/lock` flock
   across the network call, and `View`'s reload goes through the same
   `withLock`. So `View` stops waiting without changing wsledger: it waits for
   the store's reload only for a short bound (sub-second), and when the reload
   does not finish in time - the daemon's own fetch or another process's
   push/fetch holds the lock - it serves that store's last folded state. At
   most one reload per store is in flight; when a reload that outlived the
   bound completes, the host signals a change (the same callback the store
   watch uses, so `TrackerChanged`) and the page re-reads fresh state. A store
   with no folded state yet (the first read) waits as today. Only the
   daemon's page read path is bounded (board 142 a): a Host option set where
   the daemon creates its Host, or a separate bounded method called from
   `dashboardTracker.View` (`cmd/ws-mcp/tracker_host.go`); the agent-facing
   MCP reads (`tracker_query`, `tracker_show` in lead-session and
   support-agent `ws-mcp` processes) keep waiting for fresh state as today,
   because only the daemon has a callback that re-reads after a late reload.
   The page read thus promises briefly stale state instead of blocking. A
   multi-store read
   (`tkReadAll`) also renders per store rather than after the slowest one
   (`tkReadAll` is page JavaScript, not daemon Go: it calls `done()` only
   after every store's view loads,
   `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L2772-L2792`).
   Rejected: a separate ticket (139; an immediate notification whose re-read
   then blocks up to 60 s shows the same empty panel, and 140 c deferred it
   again); releasing only `st.mu` around the fetch (the design review showed
   the wait stays in wsledger's lock); splitting wsledger's network phase out
   of its lock (140 a; changes the cross-process locking contract of
   a6f97847 for every process to fix a display problem); bounding every
   `View` caller (142 b; an MCP process has no re-read, so an agent could get
   an arbitrarily old snapshot, even one missing its own earlier write).
3. **Runner-to-daemon signal shape (technical, from fact population).** The
   backend parsers emit a new `wsagent.Event` field (for example
   `TrackerWrite bool`) on a successful tracker update tool result, next to
   `Card`; the runner's event switch (`runner.go`, the `ev.Card` case) turns
   it into a call of a tracker-changed callback the daemon side supplies -
   an `Options` field or a second `Start` argument, whichever fits the
   construction order in `cmd/ws-mcp/dashboard.go` - wired to
   `Daemon.TrackerChanged`. wsagent stays unaware of wsdashboard's tracker
   types. Rejected: reusing `notify(replyID, project)` (it re-reads one
   conversation's stores, a different meaning, and would wake conversation
   streams for a tracker change); having the backend parsers call the daemon
   directly (parsers stay pure event producers, as with `Card`).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing (path row
  for `agents-plugin-tool/`); read `ai-docs/manuals/ws-mcp.md` before editing
  `agents-plugin-tool/internal/mcp/`.
- The store watch must be cheap: a stat or ref read per store per tick, no
  ledger fold, and no network.
- The daemon's own writes still call `TrackerChanged` directly; the watch may
  see them again, and a duplicate bump is harmless.
- Never touch the user's dashboard daemons on ports 29417 / 45743; tests and
  manual checks use a dev daemon with isolated `WS_CACHE_HOME` /
  `WS_CONFIG_HOME`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- The store watch must not trigger itself: compare only the journal files and
  ledger refs, never files the re-read path writes (`host-sync.json`,
  `ui-seen.json`, and the like); an idle store with an open page causes no
  repeated bumps.
- Bump only through `Daemon.TrackerChanged`, never by calling the broker's
  wake directly. `261109-bug-ws-dashboard-stream-wake-rerender-cpu` merged
  (0f1872fb6) and changed broker wake routing; `TrackerChanged` still calls
  `trackerRev.Add`, `stats.WakeTracker`, and `broker.signalAll()`, and the
  current `tracker_routes.go` / `sse.go` are the baseline.

## Prior Decisions

- 261108-feat-ws-tracker-drawer-ui (2026-11-08, Decisions): "One Host per daemon: one wstrackerhost.Host, created in cmd/ws-mcp and handed to wsdashboard through a small interface wsdashboard defines ... wsdashboard stays tracker-free" — bearing: constrains
- 261108-feat-ws-tracker-drawer-ui (2026-11-08, Result e85b2200d): "the poll's onRound = daemon.TrackerChanged ... Minors left open: ... View waits behind a store's network attempt" — bearing: supports
- 261108-feat-ws-tracker-caller (2026-11-08, Result 9b3721bdc): "Host.DispatchWith ... holds a per-store file lock (<store>/write.lock) across the reload, the build, and the append." — bearing: constrains
- 261108-feat-ws-tracker-caller (2026-11-08, commit 9b3721bd): "Left as recorded: the unfiltered store list runs PreRead and View per store one after another (the Phase 1 \"a read waits out a running attempt\" Minor, now reached from a query)." — bearing: supports
- a6f97847 (2026-11-08, commit): "Reload is a read-side API addition ... an open Ledger reads only its own last refresh, so a long-lived process (the daemon ...) never saw another process's writes or fetches" — bearing: supports
- a6f97847 (2026-11-08, commit): "Daemon detection is a poll.lock flock held by StartPoll ... Throttled checks ... count 30s from the last attempt by any process, recorded in <store>/host-sync.json" — bearing: constrains
- fc2fb0c3 (2026-11-08, commit): "kept the branch's onRound hook and develop's immediate first round ... Otherwise the dashboard's tracker revision would not bump for the startup fetch." — bearing: supports
- 0b9f9bc9 (2026-11-09, commit): "minors recorded in the ticket Result (fetch-order transient window, mutex held across blob fetch, ...)" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/cmd/ws-mcp/dashboard.go, internal/wsdashboard/tracker_routes.go, internal/wstrackerhost/host.go and poll.go, internal/wsagent/claude.go, pi.go, backend.go, stream.go, internal/wsdashboard/assets/dashboard.js |
| scope.surface | cross-module | runner signal crosses wsagent to cmd/ws-mcp to wsdashboard Daemon.TrackerChanged; wsdashboard TrackerHost interface may gain a watch input |
| scope.new_public_symbol | yes | Decision 3: a new wsagent.Event field and a runner tracker-changed callback (Options field or Start argument); Runner.Start notify at internal/wsagent/runner.go#L100 is per-conversation |
| scope.new_type_contract | yes | Decision 3's Event field and callback; possibly a store-state accessor on wstrackerhost.Host for the watch |
| scope.test_surface | existing | internal/wstrackerhost/host_test.go, internal/wsagent/stream_test.go, internal/wsagent/claude_test.go, pi_test.go, internal/wsdashboard/tracker_routes_test.go |
| complexity.reuse_points | confirmed | CardTool recognition in internal/wsagent/claude.go#L572 and pi.go#L505, Daemon.TrackerChanged at internal/wsdashboard/tracker_routes.go#L405, poll onRound wiring at cmd/ws-mcp/dashboard.go#L92 |
| complexity.side_effect_risk | high | a bounded View reload with a per-store in-flight reload and a cached last folded state in internal/wstrackerhost/host.go#L160-L171 changes concurrency with attempt#L611, Dispatch write.lock and ledger Reload |
| risk.correctness | high | the bounded View must keep per-process sync outcome semantics, never serve a torn fold, and signal once a late reload lands; watch must see every external write |
| risk.fit | moderate | must respect one-Host-per-daemon and tracker-free wsdashboard; builds on merged 261109-bug-ws-dashboard-stream-wake-rerender-cpu (0f1872fb6) broker wake routing |
| risk.test | moderate | timing-based watch-interval and blocked-fetch tests can be flaky; needs a fake blocking remote |
| risk.security_or_contract | low | no auth or protocol change; daemon-local revision bump and store stat reads only |

## Phases

### Phase 1: Notify on external tracker writes and stop View waiting on fetch

- Runner hook: in the stream path that already recognizes `CardTool` (the
  per-backend parsers `internal/wsagent/claude.go#L572` and
  `internal/wsagent/pi.go#L505`; `stream.go` only defines `CardTool` and
  `cardEvent`), also
  recognize the tracker update tool's result and raise a tracker-changed
  signal the daemon turns into `TrackerChanged`, shaped per Decision 3. Match
  the tool by the same name forms the `CardTool` sites use for each backend
  (`claudeMCPToolName(ReservedMCPServer, ...)`, `piMCPToolName(...)`) with
  the tracker update tool's registered name, and only on a non-error result.
  `wsdashboard.StartDaemon` calls the runner's `Start` before it returns the
  `*Daemon` (`internal/wsdashboard/daemon.go`), so an `Options` callback set
  in `newAgentRunner` needs a race-safe late-bound target (for example an
  atomic pointer); a second `Start` argument instead changes wsdashboard's
  `AgentSessions` interface (`agent_sessions.go`).
- Store watch: a `wstrackerhost` ticker (per Decision 1) over the registered
  stores, re-reading the registry each tick so a store registered after
  startup is watched, that compares each store's journal and ledger-ref state
  against the last seen value and calls the change callback on a change.
  Choose the interval (a few seconds) and the compared state (journal
  size/mtime, ref values) so a write by another process is always seen; stop
  it with the daemon.
- Lock hardening (Decision 2): the daemon's page read (only that path; MCP
  `tracker_query` / `tracker_show` reads keep waiting) bounds its wait for a store's reload
  and, past the bound, serves that store's last folded state; one reload per
  store in flight, and its late completion calls the change callback so the
  page re-reads. Neither `st.mu` nor a wsledger lock may be held by `View`
  while it waits past the bound, and no wsledger API or locking change is
  made. The last folded state is a host-side snapshot (a cached view or its
  inputs, taken when the last reload completed, under its own small mutex):
  every Ledger reader (`Events`, `Pending`, `Tracker.View`'s `Refresh`) takes
  `l.mu`, so computing the stale state from the live ledger would block on
  the same lock; its time-relative fields and sync/pending block are as of
  the snapshot. The in-flight reload runs on a context not tied to the
  request (bounded by the existing network/lock timeouts, stopped by
  `Host.Close`) and refreshes the snapshot; the change callback is stored on
  the Host (set by `StartWatch` or an option), and a host without one skips
  the signal. The daemon creates its Host (`cmd/ws-mcp/dashboard.go`) before
  `StartDaemon` returns the `*Daemon`, so a callback given as an option
  needs the same late-bound target as the runner hook; setting it through
  `StartWatch`, wired after `StartDaemon` next to `startTrackerPoll`, avoids
  that. Wiki reads (`WikiList`, `WikiPages`, `WikiRead`) are out of scope
  and may still wait behind a fetch; record it as a minor in the Result. The existing per-process sync outcome semantics stay as they are (the
  throttled check timestamps in `<store>/host-sync.json`, `poll.lock` daemon
  detection, and `attempt` recording the outcome for every process; read them
  from `host.go` / `poll.go` before restructuring). A first read with no
  folded state waits as today.
- Page: `tkReadAll` (dashboard.js) renders each store as its view arrives
  instead of calling `done()` after the slowest; stores still loading keep
  their previous rows (or the loading state on first load) until they arrive.
- Tests: a Go test where a second host instance (simulating another process)
  writes a store and the daemon's revision bumps within the watch interval; a
  runner stream test where a `tracker.update` tool result raises the signal;
  a host test where `View` returns the last folded state within the bound
  while a fetch is blocked (fake remote that blocks until released) and the
  change callback fires once it is released; a host test where another
  process holds the store's `<store>/lock` flock and `View` still returns
  within the bound; a test that an MCP-path `View` (no bound) still waits and
  returns fresh state; a watch test that an idle store with repeated
  re-reads causes no repeated change callbacks; a page-side check (node test
  of the per-store merge, or the headless check below with one store's view
  delayed) that one slow store does not hold back the others.
- Verify in a headless browser against a dev daemon: create items through a
  separate `ws-mcp` process's `tracker_update`; the Items panel shows them
  within the watch interval without a reload.
