---
title: "Dashboard daemon: narrow the locks that make unrelated requests wait on slow I/O"
sage-review-design: completed
sage-review-completeness: completed
related:
  261110-bug-ws-dashboard-daemon-read-amplification: "sibling: that ticket shrinks the work done under these locks; this one shrinks what the locks cover. Neither depends on the other."
sage-review-design-reviewed: dc17df7d7704a982
sage-review-completeness-reviewed: dc17df7d7704a982
---

# Dashboard daemon lock scope

## Background

The user reported that the dashboard daemon sometimes answers very slowly,
as if stuck on a blocking call, and that Enter in the support-agent chat
sometimes gets no response or a very late one (whiteboard, topic
"데몬 응답 지연 / 서포트 채팅 무응답"). A read-only investigation on the
Windows daemon (0.48.15, about 28 registered sessions) sampled for 10
minutes without catching a stall: lock hold times were at most about
150 ms. It found locks whose scope turns any slow disk read, process spawn,
or git fetch into a wait for unrelated requests, and the support send path
runs through them:

- `Index.rereadMu` (`internal/wsdashboard/index.go`) is one daemon-wide
  mutex. `Refresh` (the 30 s poll) holds it across every session's
  `ReadSession` disk I/O and across `notify`; `Nudge` takes it too. Its
  synchronous callers are `handleNudge` (`daemon.go`), `inboxAnswered`
  (`inbox_routes.go`, session-page message/cancel/steer/interrupt, plus the
  worktree cleanup route `handleWorktreeCleanup`,
  agents-plugin-tool/internal/wsdashboard/worktrees.go#L233-L251), and
  `agentChanged` (`daemon.go`), the agent runner's write hook, which calls
  `broker.wake` only after the nudge returns. Queueing is unbounded: the
  ws-mcp client gives up after 1 s (`dashboardNudgeTimeout`) and nudges
  again while the server goroutine keeps waiting.
- The agent runner's conversation loop (`internal/wsagent/runner.go`) calls
  `c.r.notify` for every write, including each live streamed event, so each
  event runs a synchronous `Index.Nudge` under `rereadMu`; `startConv`
  writes the registration and notifies while holding the runner-wide
  `r.mu`, so every `conv`, `Get`, `List`, and `act` call waits behind it.
- `Support.mu` (`internal/wsagent/support.go`) is one mutex for all
  stores. `Support.Send` holds it across `current`, `open`
  (`Runner.Create`, which takes `r.mu` and nudges), and `Runner.Send`,
  whose `act` waits on `conv.do` for the loop to be free and then runs
  `kick()`, which after an idle timeout runs `startProcess` (profile
  resolve, key ensure, PATH lookup, process spawn) inside the HTTP request.
- `DashboardSessionSource.mu` (`internal/mcp/dashboard_sessions.go`) is
  held across the `listKeys` rescan of every key directory when a reply-id
  misses, blocking every render's `lookupRecord` and every `ReadSession`.
- Block and mutex profiles are never enabled, so `/debug/pprof/block` and
  `/debug/pprof/mutex` record nothing and the next stall cannot be
  attributed.

## Decisions

1. **Agent writes wake the page without waiting on the index re-read.**
   `agentChanged` marks the conversation dirty and returns; the coalescing
   re-read runs off the caller, so neither the conversation loop nor `r.mu`
   waits on `rereadMu`, and repeated writes to one conversation while a
   re-read is pending collapse into one. Coalescing is per session: at most
   one re-read in flight per session, sessions in parallel, so a stalled
   `ReadSession` for one session never delays another's re-read or its
   page wake (one goroutine draining every dirty session in turn would). `broker.wake` is not ordered
   behind the re-read: the page's stream reads the conversation's own live
   files, so it may wake at once, and the index's change wakes it again
   when the re-read lands. `startConv` writes the registration and notifies
   after releasing `r.mu`.
2. **The index serializes re-reads per session, not daemon-wide.** The
   invariant "an older read of a session never lands after a newer one"
   holds per reply-id (a per-session lock or sequence check in `apply`).
   `Refresh` takes it per session instead of across the sweep. The "read"
   the invariant covers includes the registration read and the removal
   decision, not just `ReadSession`: today `Refresh` lists every
   registration once at sweep start and removes sessions missing from that
   listing, so without `rereadMu` a session registered (or re-registered by
   a resume or end) and nudged mid-sweep would be removed or rolled back by
   the stale listing. The implementer either re-reads the registration and
   confirms a removal under the per-session lock, or takes the sequence
   stamp before the registry listing. `handleNudge` and `inboxAnswered`
   hand the session to the same per-session coalescing and return, so a
   stall no longer piles up one goroutine per nudge; whatever their callers
   need back from the nudge today keeps its meaning (`handleNudge`'s 404
   needs its registration lookup and project check to stay synchronous; the
   implementer checks `inboxAnswered`'s callers before changing their
   return timing). `ReadSession` now runs concurrently for different
   sessions, so the read caches it touches (`DashboardSessionSource`,
   `agentSessions`, the read-amplification `FileCache`s) must be safe for
   that; the implementer confirms it under `-race`.
3. **The support lock covers only the store-to-conversation map.**
   `Support.mu` becomes per store, held for `current`, `open`, and the
   now-fast `Runner.Send` so a send cannot race `New` or a harness switch
   closing its conversation; a harness switch keeps creating the new
   conversation before closing the old one (496d8de1). Rejected: a global
   lock released before `Runner.Send`, which opens that race (a 409, or a
   200 for a message the close then drops). `Runner.Send` returns once the
   message is written to the conversation's inbox (its doc comment: it
   holds the text and reports whether the message is still held): the
   inbox write runs outside `conv.do`, with the closed check serialized
   against `close()` (for example under `c.mu`) so a send racing
   `CloseConversation` never reports success for a message nobody reads,
   and a non-blocking signal tells the loop to `kick`; `kick` and
   `startProcess` run in the loop, never inside the request. A start
   failure keeps reaching the page as today, through `lastErr` in the
   conversation's `State.Error` (Get and the store answer); the send answer
   no longer carries it, and no snapshot field is added.
4. **The session source rescans keys outside its lock.** The `listKeys`
   rescan on a reply-id miss runs without holding `src.mu`, with concurrent
   misses sharing one rescan (singleflight or equivalent); the lock guards
   only the map swap.
5. **Diagnostics name the next stall.** When `WS_MCP_DASHBOARD_PPROF` is on,
   the daemon sets a mutex profile fraction and a block profile rate, so
   `/debug/pprof/mutex` and `/debug/pprof/block` attribute waits.
6. **Tracker wiki and attachment reads wait a bounded time for the store
   lock (user, whiteboard (633) a: split the lock later in
   `261110-bug-ws-tracker-store-lock-network-split`, bound these reads
   meanwhile).** A tracker store's `mu`
   (`internal/wstrackerhost/host.go`, `with`) is held across `git fetch`
   and sync for up to `networkTimeout`, and the wiki read routes
   (`wiki.go`) and attachment reads (`attachments.go`) wait on it without
   a bound (view GETs are already bounded by `ViewWithin`). Until that
   split lands, those dashboard read routes wait at most 500 ms for
   the lock and otherwise answer a retryable busy status (503 with
   `Retry-After`). `st.mu` is a `sync.Mutex`, so the bounded wait needs an
   acquisition that can time out (a channel lock or a `TryLock` poll), and
   it applies only to the dashboard adapter's routes, since the MCP wiki
   tools (`internal/mcp/wiki_tools.go`) call the same `Host` methods. The
   page retry does not exist today and is built here: the three wiki JSON
   loads (`wkLoadList`, `wkLoadPages`, `wkLoadDoc` in `tracker.js`)
   re-fetch after the `Retry-After` delay on a 503 instead of rendering the
   error, and server-rendered wiki and attachment `<img>` elements reload
   on error a bounded number of times, since a script never sees an
   image's status. Writes and the MCP tool paths keep today's waits. Rejected: splitting the lock in this ticket, because the ledger's
   consistency rules make it the riskier change and it is off the support
   send path.

## Constraints

- Out of scope here, each in its own ticket (user, whiteboard (632) a and
  (633) a): the browser connection limit
  (`261110-bug-ws-dashboard-sse-connection-budget`), and splitting the
  tracker store lock held across `git fetch`
  (`261110-bug-ws-tracker-store-lock-network-split`).
- Concurrency tests are deterministic: block the path under test with a
  hook or channel and assert the other path completes, rather than
  asserting wall-clock durations under `-race`.
- Read caching under these locks belongs to
  `261110-bug-ws-dashboard-daemon-read-amplification`; do not duplicate it.
- Ordering and coalescing are the correctness risk: a test for each of
  Decisions 1-4 shows the old wait is gone (a blocked re-read or rescan
  does not block the other path) and that the per-session ordering
  invariant still holds under concurrent nudges and refresh.
- Never run go build/test/vet on the lead machine; run them remotely:
  `~/.local/bin/ws-rgo -C <checkout>/agents-plugin-tool -m 30 -- test
  <packages> -count=1 -race -timeout 30m`.
- Never touch the user's dashboard daemons on ports 29417 / 45743.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for
  agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for
  agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261110-bug-ws-dashboard-daemon-read-storm (2026-11-10, Decisions): "Scope is A+B+C+D below; E (pausing tracker reloads in hidden tabs) is out." (touches internal/wsdashboard/index.go and internal/mcp/dashboard_sessions.go) — bearing: constrains
- ee0520fa (2026-11-06, commit): "The daemon maps reply-id to key by scanning the key store's file names (rescanned when the directory changes or after 5 s)... Nudges go through a bounded queue drained by one goroutine that folds duplicates" — bearing: supports
- 7d9d2fba8 / 261109-bug-ws-tracker-external-write-refresh (2026-11-10, Result): "Host.ViewWithin runs the reload in the background (one per store...), waits at most wait, and otherwise serves the last folded snapshot" — bearing: supports
- 55e7a3c5 (2026-11-08, commit): "MarkSeen takes only a host-level mutex, not the store's mu or ledger: a store's mu is held across network calls up to a minute, and a seen mark must not wait for one." — bearing: supports
- 496d8de1 / 261108-feat-ws-tracker-support-agent-chat (2026-11-08, commit): "A harness switch creates the new conversation first and then closes the old one, so a failed create leaves the old conversation open." — bearing: constrains
- 85b9da5f (2026-11-08, commit): "The runner lives in its own package and never imports internal/mcp (which imports wsdashboard); cmd/ws-mcp wires the key store and the ws-mcp command in" — bearing: constrains
- 6431890d / 261109-bug-ws-dashboard-stream-wake-rerender-cpu (2026-11-09, commit): "kept behind the existing pprof opt-in rather than as temporary code, so ... a later investigation can reuse them" — bearing: supports
- 62055a21 / 261106-feat-ws-dashboard-browser-interaction (2026-11-06, commit): "The daemon serializes its own sends and cancels with one mutex; races with the adapter's drain are settled by the inbox's rename claims." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | internal/wsdashboard/index.go, daemon.go, inbox_routes.go, pprof_route.go or cmd/ws-mcp/dashboard.go; internal/wsagent/runner.go, support.go; internal/mcp/dashboard_sessions.go; internal/wstrackerhost/host.go, wiki.go, attachments.go; internal/wsdashboard/tracker_routes.go; assets/tracker.js (wiki load retry, image reload) |
| scope.surface | cross-module | wsdashboard Index, wsagent Runner and Support, mcp DashboardSessionSource; Runner.Send and Support.Send return timing changes across packages |
| scope.new_public_symbol | no | coalescing worker and per-session lock can stay unexported; none required by the Decisions |
| scope.new_type_contract | no | no signature change named; Runner.Send keeps State, error |
| scope.test_surface | existing | internal/wsdashboard/index_test.go, daemon_test.go, inbox_routes_test.go, internal/wsagent/runner_test.go, support_test.go, internal/mcp/dashboard_sessions_test.go, internal/wstrackerhost tests, wsdashboard tracker route and node-harness asset tests |
| complexity.reuse_points | unconfirmed | singleflight is not in go.mod and no singleflight use exists in the tree; ViewWithin background-reload pattern in wstrackerhost is a candidate model, not read |
| complexity.side_effect_risk | high | async kick and coalesced re-reads change when page wakes, index revisions, and start errors become visible |
| risk.correctness | high | per-session ordering invariant, coalescing, and async startProcess are concurrency changes across three locks |
| risk.fit | moderate | Decisions 2 and 3 leave the per-store vs release-before-Send and lock vs sequence choices to the implementer |
| risk.test | high | tests must prove absence of blocking and ordering under concurrency with -race, prone to timing flakes |
| risk.security_or_contract | moderate | handleNudge 204/404 answer and Runner.Send held-message meaning must keep their caller-visible semantics |

## Phases

### Phase 1: lock scope

Implement Decisions 1-6.

Verification:

- Tests: a re-read blocked for one session does not delay a nudge, an
  agent write, or a page wake for another session; repeated nudges for one
  session while its re-read runs coalesce into one more read; an older read
  never overwrites a newer one under concurrent `Refresh` and `Nudge`,
  including a session registered or re-registered during a `Refresh`
  sweep (not removed or rolled back by the sweep's stale listing);
  `Get`, `List`, and `act` on one conversation are not blocked while
  another conversation's `startConv` writes its registration.
- Tests: `Support.Send` returns while the conversation loop is busy or
  starting a process, and a second store's send does not wait on the
  first's; a send racing `CloseConversation` either lands or fails, never
  reports success for a dropped message; a start failure still shows on
  the conversation's `State.Error`.
- Tests: a reply-id miss rescan does not block `lookupRecord` for a known
  key.
- Tests: with a store lock held (a fetch in flight), a dashboard wiki or
  attachment read answers the busy status after the bounded wait, and the
  same read succeeds once the lock is free; the MCP wiki tool path still
  waits; the tracker page re-fetches a busy wiki JSON load and reloads a
  failed wiki or attachment image (asset test or scripted check); the pprof
  opt-in sets the mutex and block profile rates.
- The wsdashboard, wsagent, wstrackerhost, and mcp dashboard tests pass
  remotely through `ws-rgo` with `-race`.
