---
title: Linked-server terminal WebSocket relay fails its first real WSL<->Windows exercise with an undiagnosable blanket 502
related:
  260716-feat-ws-dashboard-daemon-persistent-log-layer: "prerequisite — the persistent rolling-file log sink must land first so the instrumentation below is captured durably on a detached daemon"
sage-review-design: completed
sage-review-completeness: completed
---

# Linked-server terminal WebSocket relay fails its first real WSL<->Windows exercise with an undiagnosable blanket 502

> **Status (2026-07-16): WIP — not resolved.** The confirmed root cause is a
> FRONTEND active-root / selection-derivation instability, NOT a backend relay
> bug. See `## Confirmed root cause (2026-07-16)` below; it OVERTURNS the earlier
> "mid-session relay pump drop" framing (the `## Reframed findings` section),
> which is retained only as ruled-out investigation history. Diagnostic tracing
> landed (Phase 1) and a partial hotfix has landed (`af058b73`); the substantive
> two-pronged frontend/socket fix is deferred to Phase 2.

## Background

- Dogfooding topology: Windows frontend daemon at localhost:4300 (linked WSL daemon 8787 as server `wsl-daemon`, endpoint http://127.0.0.1:8787, --no-auth). Opening a linked-server work-root now mounts a terminal tab (frontend derivation refactor 260714-refactor-dashboard-active-root... Phase 1 landed and works), but the terminal WebSocket then fails:
  `ws://localhost:4300/api/dashboard/servers/wsl-daemon/terminals/term_.../socket` -> "WebSocket is closed before the connection is established" + a fetch-failed 502.
- Confirmed this is a SEPARATE subsystem from the frontend refactor: the WS URL and connection logic were untouched by that refactor.

## Findings (read-only investigation, evidence)

- The linked-server WS proxy path is real, complete code: `ws-dashboard/crates/daemon/src/servers.rs:1476` `server_scoped_terminal_websocket` resolves the linked server (`resolve_server_scoped_forwarding`, ~1749), opens an outbound WS to upstream via `connect_remote_terminal_websocket` (~1562) with URL from `remote_terminal_websocket_url` (~1594, reuses legacy unscoped `/api/dashboard/terminals/{id}/socket`, http->ws), and only on successful connect calls `upgrade.on_upgrade` + `terminal_websocket_relay` (~1617). Local path contrast: `terminal.rs:436`.
- NOT a stale-binary gap: the running Windows binary (branch git-discovery-combined-rev-parse-hotfix) and current goal HEAD are byte-identical for these four functions. Rebuilding the Windows binary alone will NOT change behavior.
- NOT the known IPv4/loopback hang (260714-idea-dashboard-linked-server-localhost-ipv6-hang): link uses the 127.0.0.1 literal and plain HTTP forwards (resources, git/status) succeeded moments earlier.
- Root cause class (d): first real end-to-end exercise of the linked-terminal WS relay across a WSL<->Windows hop. ZERO test coverage for `connect_remote_terminal_websocket`/`terminal_websocket_relay`/`remote_terminal_websocket_url` (landed 2026-07-03, commit 7c0db03c, never round-tripped over a real network boundary). The error mapping at `servers.rs:1582` collapses every non-HTTP tungstenite error (`_ => TerminalWebSocketForwardError::Unavailable`) into a blanket 502 "linked server unreachable" with NO tracing anywhere on this path, so the actual transport cause (connection refused / reset / handshake mismatch / etc.) is currently unknowable. The WSL daemon has no per-request access logging, so it can't even confirm whether the upgrade request reaches port 8787.

## Diagnosis next-steps (not yet done)

- (A) No-rebuild disambiguation: open a direct WS to the WSL daemon's own terminal endpoint (ws://127.0.0.1:8787/api/dashboard/terminals/<id>/socket) and compare against the 4300-proxied path -> isolates Windows-relay vs WSL-endpoint fault.
- (B) Add tracing::warn/error of the real error in `connect_remote_terminal_websocket`'s map_err arm (servers.rs:1582), rebuild the Windows binary, restart the Windows daemon, reproduce -> observe the actual transport error before a substantive fix. (Windows daemon restart requires owner confirmation.)
- Regardless of the specific fix: this path needs (1) error diagnosability (stop collapsing to blanket 502), and (2) test coverage for the relay.
- Prerequisite: the diagnosis instrumentation depends on `260716-feat-ws-dashboard-daemon-persistent-log-layer` landing first — a detached daemon currently discards stderr, so the per-direction teardown-reason and connect-path error logs added for this bug must be captured by a durable rolling-file sink to survive a reproduction run.

## Additional findings (read-only relay-path investigation, 2026-07-14)

Second read-only pass after the option-A probe isolated the fault to the
Windows->WSL relay leg. Ranked most-likely-cause first; each entry cites the
`servers.rs` (or dependency source) evidence actually read, not a guess.

1. **The blanket swallow at `servers.rs:1582-1587` hides more than "connection
   refused" — it collapses nearly the entire `tungstenite::Error` enum, and the
   two most likely real causes are still indistinguishable from each other
   without the tracing already scoped as next-step (B).** Full enumeration
   from `tungstenite` 0.29 (`~/.cargo/registry/src/*/tungstenite-0.29.0/src/error.rs:15-75`):
   `ConnectionClosed`, `AlreadyClosed`, `Io(io::Error)`, `Tls(TlsError)`,
   `Capacity(CapacityError)`, `Protocol(ProtocolError)`, `WriteBufferFull`,
   `Utf8`, `AttackAttempt`, `Url(UrlError)`, `HttpFormat(http::Error)` — only
   `Error::Http(Response)` is distinguished today (mapped to `Rejected`, servers.rs:1583-1585).
   Since the browser-observed failure is a 502 (i.e. the `Unavailable` arm, not
   `Rejected`), the real fault is one of the swallowed variants. The two most
   plausible, given a direct external WS probe to the same upstream endpoint
   already succeeded:
   - `Error::Io(io::Error)` — a raw OS-level failure (refused/reset/timed out)
     on the outbound `TcpStream` that `tokio_tungstenite::connect_async`
     (servers.rs:1579) opens from *the Windows daemon process itself* to
     `127.0.0.1:8787`. This socket is opened by a different code path/process
     than whatever tool performed the option-A probe, so a per-process
     WSL2-loopback-forwarding discrepancy cannot be ruled out from code alone.
   - `Error::Protocol(ProtocolError)` (variants enumerated at
     `tungstenite-0.29.0/src/error.rs:172-232`: e.g.
     `SecWebSocketAcceptKeyMismatch`, `MissingConnectionUpgradeHeader`,
     `InvalidHeader`, `HttparseError`) — a malformed or unexpected handshake
     response from the upstream, distinguishable from `Io` only by content.
   Neither can be confirmed further without the tracing add already queued as
   ticket next-step (B); this entry exists to make sure that tracing captures
   the *value*, not just the presence, of the error (the `Debug`/`Display` of
   the whole `tungstenite::Error`, not a boolean).

2. **`tokio-tungstenite` carries no TLS backend at all, unlike the sibling
   `reqwest` client used for every other forwarded operation — a latent
   defect independent of today's bug, but shaped identically (untraceable
   502).** `ws-dashboard/Cargo.toml:29` declares `tokio-tungstenite = "0.29"`
   with no feature list, and `Cargo.lock` confirms the resolved dependency set
   is only `futures-util, log, tokio, tungstenite` (no `native-tls` or
   `rustls-tls`/`rustls` present under the `tokio-tungstenite` entry). By
   contrast `ws-dashboard/Cargo.toml:32` deliberately opts `reqwest` into
   `rustls-tls`. Today's dogfooded link is plain `http://127.0.0.1:8787`, so
   this is not implicated in the current failure, but it means once the
   http-endpoint case is fixed, any linked server configured with an
   `https://` endpoint will have `connect_remote_terminal_websocket` fail
   every single time via `Error::Tls(TlsError)` — collapsed into the exact
   same blanket `Unavailable` -> "linked server unreachable" 502 documented
   above — while that same server's plain-HTTP/SSE forwards (resources, git,
   activity, etc., all via `request_remote_dashboard_operation` /
   `request_remote_sse`, servers.rs:1885-1978) would keep working. This
   asymmetry should be closed in the same pass that adds tracing/tests for
   this path.

3. **Confirmed non-causes (recorded so they are not re-litigated in the fix
   pass):**
   - Handshake header construction is correct and unremarkable. Tungstenite's
     `impl IntoClientRequest for Uri` (`tungstenite-0.29.0/src/client.rs:223-244`)
     builds `Host` (from the URL authority, port included), `Connection:
     Upgrade`, `Upgrade: websocket`, `Sec-WebSocket-Version: 13`, and a fresh
     `Sec-WebSocket-Key` exactly as any conforming client would — nothing is
     dropped or duplicated relative to a generic WS client, so this is not a
     header-construction bug.
   - `on_upgrade` ordering is correct in code. `server_scoped_terminal_websocket`
     (servers.rs:1499-1502) only calls `upgrade.on_upgrade(...)` inside the
     `Ok(upstream)` arm of `connect_remote_terminal_websocket`, upholding the
     CONTRACT comment at servers.rs:1486-1490 — there is no code path that
     accepts/half-opens the browser-side socket before the outbound connect
     resolves. (A slow-but-eventually-successful outbound connect could still
     make the *browser* give up first due to its own client-side WS connect
     timeout while the daemon holds the HTTP response open — `connect_async`
     at servers.rs:1579 has no timeout wrapper — but that is a latency/UX
     concern, not an ordering bug, and does not match a 502 being returned by
     the daemon itself.)
   - The Authorization header cannot be the fault on this specific
     `--no-auth` WSL upstream. `router.rs:97-99` plus `auth.rs:172-182` show
     that `require_owner_auth` (and therefore `entrypoint_headers_allowed`
     and the bearer-token check) is omitted for the *entire* protected router
     when `--no-auth` is set (`config.rs:37`), so whatever Authorization
     value `connect_remote_terminal_websocket` attaches (servers.rs:1572-1578)
     is inert on that leg regardless of content. The bearer token itself is
     also always hex-ASCII (`auth.rs:366-370`, `hex_encode`), so
     `.parse::<HeaderValue>()` at servers.rs:1577 cannot fail in practice
     either.

4. **Minor/low-severity latent issue:** `remote_terminal_websocket_url`
   (servers.rs:1611-1613) unconditionally clears the upstream URL's entire
   query string and re-adds only `after=<n>`. Harmless today because
   `TerminalWebSocketQuery` (terminal.rs:326-330) has exactly one field, but
   it means any future query parameter added to the terminal-socket route
   would be silently dropped specifically on this relay hop (the local path
   in `terminal.rs:436-460` and the plain-HTTP forwards in
   `request_remote_dashboard_operation`/`request_remote_sse` do not have an
   equivalent clear-and-rebuild step).

## Reframed findings: mid-session relay drop (read-only, 2026-07-14)

> **SUPERSEDED (2026-07-16).** This section's "backend relay pump drop"
> conclusion was overturned by Phase 1 tracing. The relay is correctly invoked;
> the client closes the socket. Retained only as ruled-out history — see
> `## Confirmed root cause (2026-07-16)`. The pump-loop fragility and missing TLS
> backend named below remain real latent defects, tracked in Phase 2's deferred
> list, but they are NOT this bug's root cause.

### Corrected framing

New reproduction detail from dogfooding: terminal content **rendered briefly
(live output streamed), then the WebSocket dropped immediately after**. This
is decisive and overturns the prior round's top candidate.

`connect_remote_terminal_websocket` (servers.rs:1562-1588) runs to completion
exactly once, before `upgrade.on_upgrade(...)` (servers.rs:1500-1502) is ever
called. Per the CONTRACT comment at servers.rs:1486-1490, no byte can reach
the browser unless that connect already succeeded. Since the browser is
confirmed to have received live output, the connect leg (and everything the
prior "Additional findings" section reasoned about — the swallowed
`tungstenite::Error` variants at servers.rs:1582-1587, the missing TLS
feature, the handshake/`on_upgrade`-ordering/auth non-causes) is **provably
not what is executing when the failure happens**. Those findings stay valid
as latent defects (in particular #2, the missing TLS backend, is real and
independent) but they diagnose a *different* failure mode — "never connects"
— than the one actually observed — "connects, streams, then drops mid-session".
The fault must be in `terminal_websocket_relay` (servers.rs:1617-1655) or in
whatever governs the established session's lifetime on either leg.

The browser console message `WebSocket is closed before the connection is
established` is downgraded from primary evidence to a **secondary artifact**.
That exact string is the browser's own diagnostic for calling `.close()` on a
socket still in the `CONNECTING` state — and the frontend's terminal-socket
effect cleanup does exactly that unconditionally on every teardown:
`frontend/src/App.tsx:8596-8600` (`disposed = true; ...; socket.close();`,
effect deps `[terminalId, paneVisible]`, `frontend/src/App.tsx:8601`). The
socket's own `close` handler (`frontend/src/App.tsx:8587-8594`) sets status to
`"fallback"` with no explicit reconnect-via-new-socket call visible in this
effect, so the most consistent read is: first session drops (server-side, see
below) -> some later re-render/cleanup cycle calls `.close()` on a
freshly-opened-but-not-yet-`OPEN` retry socket -> browser logs that specific
message. It is therefore a **downstream symptom of the same root drop**, not
an independent connect-time fault.

### Ranked candidate causes (pump-loop and session-lifetime code, in rank order)

1. **[Top-ranked] Any single-direction read error or EOF silently tears down
   the entire bidirectional relay, with zero tracing anywhere in this file —
   servers.rs:1621-1655, decisive lines 1624 and 1638.**

   ```rust
   browser_message = browser_receiver.next() => {
       let Some(Ok(message)) = browser_message else { break; };   // servers.rs:1624
       ...
   }
   upstream_message = upstream_receiver.next() => {
       let Some(Ok(message)) = upstream_message else { break; };  // servers.rs:1638
       ...
   }
   ```

   Either branch's `else { break; }` fires on **any** of: the stream ending
   (`None`) or **any** `tungstenite::Error` variant surfacing from
   `WebSocketStream::poll_next` (`tokio-tungstenite-0.29.0/src/lib.rs:291-315`
   maps every non-`AlreadyClosed`/`ConnectionClosed` error straight through as
   `Some(Err(e))`) — the same full `tungstenite::Error` enum already
   enumerated in this ticket's prior round (`Io`, `Protocol`, `Capacity`,
   `Utf8`, `AttackAttempt`, etc.). `break` exits the single shared `loop`, and
   the two lines immediately after it (servers.rs:1653-1654) then send a
   `Close` to **both** legs unconditionally — i.e. a hiccup on the upstream
   leg's *read* side kills the browser's side too, with no distinction between
   "the peer legitimately closed" and "the read glitched." `grep -n
   "tracing::" ws-dashboard/crates/daemon/src/servers.rs` returns **zero**
   matches in the entire 2569-line file, even though `tracing` is a workspace
   dependency used elsewhere in the same crate (`git_worktree.rs`,
   `persistent_state.rs`, `root_picker.rs`, `server.rs`) — so if this branch
   fires today, it fires invisibly.

   This exactly reproduces the reported shape: connect succeeds, bytes flow
   (the burst that renders), then one read on the upstream leg (the
   Windows-daemon-process -> WSL-daemon TCP hop, crossing the WSL2 NAT/vEthernet
   boundary) errors or EOFs, and the whole session dies silently. The local
   path has the structurally identical "abort on any error" shape
   (`terminal.rs:722`, `let Some(Ok(message)) = maybe_message else { break; };`)
   — so this is not a *different* code shape between the two paths, it is the
   *same fragile shape*, but only the relay path has a plausible source of a
   spurious read error (an extra cross-process, cross-network TCP hop with a
   from-scratch `tokio_tungstenite::connect_async` client that has never
   round-tripped over a real WSL<->Windows boundary before this dogfood
   session, per the "root cause class (d)" note in this ticket's original
   Findings section). This is why the *pump-loop* cause now outranks every
   connect-time candidate from the prior round: it is the only mechanism in
   the code that can explain data flowing first and the drop coming after,
   and it shares the exact "swallowed-error, no tracing" texture that already
   made the connect-time path undiagnosable — just one function down.

2. **Ping/Pong keepalive mechanics: examined in depth, confirmed NOT the
   asymmetry the initial hypothesis expected — reclassified from "top
   candidate" to "confirmed structurally sound, with one residual latency
   caveat."**
   - Axum's `WebSocket` (used for the local path's single socket AND the
     relay's browser-facing leg) is a direct pass-through wrapper over
     `tokio_tungstenite::WebSocketStream` (`axum-0.8.9/src/extract/ws.rs:546-618`);
     its own doc comment states plainly: "Ping messages will be automatically
     responded to by the server... Pong messages will be automatically sent to
     the client if a ping message is received" (`axum-0.8.9/src/extract/ws.rs:781-791`).
     The relay's upstream leg uses that same `tokio_tungstenite::WebSocketStream`
     type directly (servers.rs:1553-1554). Both legs are `.split()` (servers.rs:1618-1619;
     mirrored in the local path at `terminal.rs:705`) via `futures_util`'s
     `BiLock`-backed `SplitStream`/`SplitSink`, which share one underlying
     connection object — identical mechanism on every leg in both paths, not
     something the relay uniquely breaks.
   - Tungstenite's `read()` (`tungstenite-0.29.0/src/protocol/mod.rs:449-479`)
     flushes any auto-queued Pong reply at the **top of its own loop**, before
     attempting to read the next frame (line 457: `if self.additional_send.is_some()
     ... { self.flush(stream)? }`). Because the relay's `select!` calls
     `upstream_receiver.next()` again on literally the next loop iteration,
     the queued Pong for a received Ping is flushed on that very next poll —
     sub-millisecond in practice, not "deferred indefinitely." This refutes the
     literal form of the original hypothesis (manual frame-forwarding leaving
     a Ping permanently unanswered).
   - Residual, unconfirmed caveat: while one `select!` arm's body is
     `.await`-ing (e.g. `browser_sender.send(...)` forwarding a large burst,
     servers.rs:1643), the other arm (`upstream_receiver.next()`) is not
     polled at all until that send resolves — so a queued Pong could sit
     unflushed for the duration of one large forwarded message. This shape
     exists equally in the local path's single loop (`terminal.rs:720-766`),
     so it is not relay-specific either.
   - Neither leg proactively sends keepalive Pings today: `grep -rn
     "keep_alive|KeepAlive|ping_interval|WebSocketConfig"
     ws-dashboard/crates/daemon/src/` returns no matches; axum's
     `WebSocketUpgrade` defaults to no periodic ping unless `.keep_alive(...)`
     is called (it isn't, anywhere in this codebase), and `connect_async`
     (servers.rs:1579) passes no `WebSocketConfig`. So an "unanswered
     application-level keepalive Ping -> idle-timeout close" mechanism has no
     active trigger in this code as written. If the drop is timeout-driven at
     all, a TCP/NAT-layer idle timeout on the WSL2 vEthernet/NAT boundary is a
     more likely source than a WS-level ping/pong fault — that boundary is
     outside what this code can confirm or rule out by itself.

3. **Half-close / Close-frame handling is symmetric and matches the RFC
   intent; not a distinguishing cause.** Both `select!` arms treat a `Message::Close`
   the same way: forward it to the other leg, then break (servers.rs:1625,
   1633-1635 for the browser->upstream direction; 1642, 1646-1648 for the
   reverse). After the loop, both legs get an explicit best-effort `Close`
   send (servers.rs:1653-1654), which is arguably *more* graceful than the
   local path — `terminal.rs`'s `terminal_socket_task` never explicitly sends
   `Message::Close` on normal PTY exit (`mark_exited`/`mark_error`,
   `terminal.rs:682-696`, `666-680`); it just breaks the loop and lets the
   dropped `WebSocket` end the TCP connection without a close handshake. So
   the relay's Close-frame handling is not less correct than the local path —
   ruled out as an explanation for a *relay-specific* drop; it is subsumed by
   candidate #1 (a Close is one specific, well-handled instance of the
   `else { break; }` in candidate #1 — the bug is in the *unhandled*, silent
   instances: raw `None`/`Err`, not `Close`).

4. **Frame-type fidelity: essentially complete, one narrow and
   already-justified gap.** `axum_to_tungstenite_message`/
   `tungstenite_to_axum_message` (servers.rs:1657-1686) round-trip
   Text/Binary/Ping/Pong/Close faithfully in both directions with no
   Binary-as-Text or similar mistranslation. The only dropped variant is
   `TungsteniteMessage::Frame(_) => None` (servers.rs:1682-1684), which the
   in-code comment attributes to tungstenite having no axum equivalent for
   its raw `Frame` type — the same drop-don't-error stance axum itself takes
   for the identical reason (`axum-0.8.9/src/extract/ws.rs:840-841`, citing
   snapview/tungstenite-rs#268). Confirmed non-cause.

5. **Backpressure/`WriteBufferFull` is not reachable under this code's
   configuration.** `connect_async(request)` (servers.rs:1579) uses
   `tokio_tungstenite`'s default `WebSocketConfig` (no config passed
   anywhere in servers.rs or terminal.rs), and tungstenite's default
   `max_write_buffer_size` is `usize::MAX`
   (`tungstenite-0.29.0/src/protocol/mod.rs:99-100`), so
   `Error::WriteBufferFull` cannot fire regardless of burst size; only
   `WouldBlock` backpressure can occur, and tungstenite retries that
   internally rather than surfacing an error
   (`tokio-tungstenite-0.29.0/src/lib.rs:351-356`). Confirmed non-cause for a
   hard failure (though it is a latent unbounded-memory-backlog risk under
   sustained backpressure, unrelated to today's bug).

6. **Initial-burst-to-live-streaming transition (`?after=0`) is not a
   distinct code path anywhere in the relay.** `remote_terminal_websocket_url`
   (servers.rs:1594-1615) only shapes the initial connect URL's `after` query
   parameter; once connected, `terminal_websocket_relay` forwards every
   upstream message identically regardless of whether it is backfill or live
   output — there is no burst/live mode switch to harbor a transition bug.
   The one place such a seam could exist is the upstream WSL daemon's own
   `terminal_socket_task` (`terminal.rs:699-768`), and there both the
   connect-time backfill (712-717) and every later backfill triggered by
   `output_signal.changed()` (754-765) call the exact same
   `send_output_backfill` function against the same cursor — no separate
   "first burst" code path exists there either. Confirmed non-cause.

### Re-rank: pump-loop vs. connect-time candidates

**The pump-loop cause (ranked #1 above) now outranks every connect-time
candidate from the prior "Additional findings" round.** The user-confirmed
symptom — output rendered, then the socket dropped — is direct evidence that
`connect_remote_terminal_websocket` (servers.rs:1562-1588) already returned
`Ok(upstream)`, since no byte can reach the browser before that (CONTRACT at
servers.rs:1486-1490). The connect-time error-swallowing findings (blanket
`_ => TerminalWebSocketForwardError::Unavailable` at servers.rs:1582-1587, and
the missing TLS backend) remain accurate, real gaps, but they describe a
"never connects, blanket 502 before any upgrade" failure — a mode the current
reproduction has already ruled out by observing live output. The corrected
diagnosis is: the fault is in `terminal_websocket_relay`'s per-iteration
`else { break; }` on `browser_receiver.next()`/`upstream_receiver.next()`
(servers.rs:1624, 1638) combined with the complete absence of `tracing::` in
`servers.rs`, which converts a to-be-expected first-ever cross-network read
hiccup into an untraceable full-session teardown. The fix pass for this
ticket should treat "add tracing + tests for the relay pump loop" as at least
as urgent as the previously-queued connect-time tracing add (next-step B),
and should specifically capture, per direction, whether the loop exited via
`None` (clean EOF) or `Some(Err(_))` (and which `tungstenite::Error` variant)
so the next reproduction can distinguish a legitimate half-close from a
transport fault.

## Confirmed root cause (2026-07-16): frontend active-root / selection-derivation instability — NOT a backend relay drop

Live dogfood tracing (Phase 1) is decisive and **overturns the prior "mid-session
relay pump drop" framing**. The backend relay is correctly invoked and behaves
correctly: its logs show clean `task started -> loop exited (clean teardown)`
pairs. Those pairs are the *symptom*, not the cause — the daemon relay exits
cleanly because **the client (browser) closes the socket**. The fault is entirely
in the frontend's active-root / workbench-selection derivation, which flips the
linked sub-worktree out of view on the poll cadence and tears its terminal socket
down.

### Causal chain (file:line anchors)

1. `mergeResourcesByServer` (`ws-dashboard/frontend/src/resourceModel.ts:204-209`)
   replaces the server's resource tree with a brand-new object on every 5s poll
   (`resourceRefresh.ts` interval; poll call at `App.tsx:609-612`).
2. When that poll's freshly-fetched tree for the LINKED server momentarily omits
   the sub-worktree's (`gitLinkedWorktree` child) `workRoot` entry — while the
   tree object itself is non-null, so the Phase-1 refactor's D2 slot-level
   fallback (`resolveActiveResources`, `resourceModel.ts:236-261`) never engages
   — `resolveWorkbenchSelection` (`resourceModel.ts:441-490`) cannot return null
   (it returns null only when `resources` itself is null, `:445-447`) and
   silently falls back to the first root walked (`fallback ??= rootSelection`,
   `:474`, returned `:489`) = the primary/root worktree.
3. `workbenchSelection` recomputes (`App.tsx:806-809`) -> `selectedWorkRootStateKey`
   (`:3782-3787`) -> `effectiveActiveRootKey` (`:5985`) now points at the wrong
   root.
4. `isActiveRoot` for the sub-worktree instance becomes false (`App.tsx:5991`), so
   its wrapper gets `style={{display:"none"}}` (`:6018-6023`).
5. Within 100ms the `focusWatchdog` (`App.tsx:8439-8441`) reads
   `container.offsetParent === null` for every pane under that wrapper and sets
   `paneVisible=false`.
6. The terminal socket effect (`App.tsx:8491-8617`, deps `[terminalId, paneVisible]`)
   re-runs; its cleanup closes the by-now-OPEN socket unconditionally
   (`App.tsx:8615`; only `CONNECTING` is guarded — that guard is the `af058b73`
   hotfix).
7. Next poll the child reappears, selection flips back, and a new socket opens for
   the SAME terminalId — repeating on the ~5s poll cadence. This matches the
   observed ~4s period, same-`term_id` reappearance, and the simultaneous
   multi-pane (8-id) burst.

**Why the root worktree is unaffected:** the primary root is the fallback's
*destination*, so it is never the one evicted.

### Regression origin

The Phase-1 active-root refactor (`ddd353fe`, decision D3) DELETED the
`lastActiveRootKeyRef` / `lastActiveRootServerIdRef` + `resolveEffectiveActiveRootKey`
safety net on the theory that D1 (render-time union) + D2 (slot-level fallback)
subsume it. But D2 only covers *the slot being absent*, not *the selected root
momentarily absent from an otherwise-present tree* — which is exactly the
child-worktree gap. Cross-references:
`.done/260714-bug-dashboard-childroot-workbench-flash-hide.md` (names this
local-vs-remote distinction verbatim) and
`ready/260714-refactor-dashboard-active-root-atomic-select-pure-derivation.md`.

### Partial mitigation already landed

Hotfix `af058b73` (cleanup no longer aborts a CONNECTING socket) is PARTIAL: root
work-root terminals are now stable, but sub-worktree terminals still churn (open
-> clean-teardown loop ~every 4s) because the OPEN-socket guard (Phase 2 prong #2)
and the selection stickiness (Phase 2 prong #1) are not yet done.

## Spec Impact

No `ai-docs/spec` workflow-system spec applies: this fix is downstream
`ws-dashboard` application behavior (frontend selection stickiness in
`resourceModel.ts`/`openRootLookup.ts` and the WebSocket cleanup guard in
`App.tsx`), outside the spec system's scope per AGENTS.md Architecture Rule 1
(specs describe the ws workflow system itself). Behavior recovery is fully
specified in this ticket's Phase 2 plan below.

## Phases

### Phase 1: Diagnostic tracing instrumentation for the relay pump loop and connect path

Add `tracing` instrumentation only (NO control-flow or teardown-semantics change) so a reproduction captured on the durable rolling-file log sink (260716, landed) reveals the real mid-session-drop cause:

- In `terminal_websocket_relay` (servers.rs:1617-1655): at each loop-exit `else { break; }` (browser leg servers.rs:1624, upstream leg servers.rs:1638), emit a `tracing` event recording, per direction, whether the loop exited via `None` (clean EOF / stream end) or `Some(Err(e))`; for the error case log the full `tungstenite::Error` value (its Debug/Display and variant, not a boolean). Include a stable target/identifier for the terminal/session.
- In `connect_remote_terminal_websocket` (servers.rs:1562-1588): in the blanket `_ => TerminalWebSocketForwardError::Unavailable` map_err arm (servers.rs:1582-1587), log the actual underlying error value via `tracing::warn` before it collapses to `Unavailable`, so a connect-time failure stops being an untraceable blanket 502.

Acceptance:
- `cargo build -p ws-dashboard-daemon` and `cargo test -p ws-dashboard-daemon` pass.
- On an owner-rebuilt detached daemon, reproducing the linked-terminal drop leaves per-direction teardown-reason lines in `<state_dir>/logs/daemon.log.<date>`, distinguishing `None` (half-close) from `Some(Err(<variant>))` (transport fault).

### Result (55ee8ff4) - 2026-07-16

Tracing landed (`55ee8ff4` relay teardown paths + `d6ff5d5d` task-lifecycle /
external-cancellation events), captured on the durable rolling-file sink. The
evidence **overturned this ticket's relay-drop hypothesis**: the relay logs show
clean `task started -> loop exited (clean teardown)` pairs, proving the backend
relay is correctly invoked and exits only because the browser closes the socket.
Root cause is therefore frontend, not backend — see
`## Confirmed root cause (2026-07-16)` above. The tracing is kept as diagnostic
infrastructure (it is what proved the relay is invoked and the client closes the
socket); the pump-loop teardown-semantics change originally anticipated here is
demoted to a Phase 2 deferred latent defect, not this bug's fix.

### Phase 2 (sage-review gate passed 2026-07-20 — implementation-ready): frontend selection-stickiness + OPEN-socket guard

> The 2026-07-16 WIP handoff note ("Do NOT implement yet") is LIFTED as of
> 2026-07-20: the design and completeness sage reviews for this ticket both
> closed with all issues resolved autonomously (see the constraint
> subsections below). Phase 2 is now implementation-ready.

The confirmed root cause is frontend active-root / selection-derivation
instability (see `## Confirmed root cause (2026-07-16)`), not a backend relay
teardown. Two-pronged fix:

1. **Re-provide the deleted protection**, scoped to "the currently-selected root
   disappeared from an otherwise-present tree": in `resourceModel.ts`
   (`resolveActiveResources` / `resolveWorkbenchSelection`, ~`:251-261`,
   `:441-490`) and/or `ws-dashboard/frontend/src/workbench/openRootLookup.ts`
   `deriveWorkbenchView` (~`:130-164`), keep the last-known selected root sticky
   across a transient content-level omission (the gap D2's slot-level fallback
   does not cover).
2. **Independently harden `App.tsx:8615`** so a `paneVisible` flip does not
   `socket.close()` an OPEN socket that has only just reached OPEN — mirror the
   CONNECTING guard the `af058b73` hotfix added.

#### Prong 1 design constraint (sage-review resolution)

The stickiness MUST be modeled on the already-vetted **server-scoped D5
pattern** — `withLastNonNullResourcesByServer` / `resolveActiveResources`
(`resourceModel.ts:236-261`) — which is server-keyed, bridges exactly one
transient poll gap, and by construction cannot pin content under the wrong
server's header.

Do NOT reintroduce the unscoped `lastActiveRootKeyRef` /
`lastActiveRootServerIdRef` cross-render ref memory. The sibling ready ticket
`260714-refactor-dashboard-active-root-atomic-select-pure-derivation`
(decision D3) deliberately DELETED that shape — it was the render-time mutable
memory that only advanced when the root already happened to be mounted, so it
was stale in exactly the gap it existed to cover, and that class of ad-hoc
"sticky last-known" patch is what produced 4 regressions in one cycle (see
that ticket's `### The four failure modes`). This ticket's fix must reconcile
with D3 rather than re-derive the deleted shape under a new name: build the
sub-worktree stickiness as a server-scoped last-good cache (D5's shape,
applied to the selected-root key rather than the resources view), not as
unscoped cross-render selection memory.

The stickiness must distinguish a transient single-poll omission (bridge it)
from a genuine worktree removal/close (let selection actually change). The
distinguishing signal: the server's resource tree itself is still non-null and
still present in `resourcesByServer` (i.e. the poll succeeded and D2's
slot-level fallback does NOT engage) but the previously-selected root's entry
is absent from that tree on this one poll. A genuine removal/close is
distinguished from a transient omission by persisting across more than one
poll cycle (or by an explicit close/removal action) — the sticky cache must
expire once the omission repeats past a single poll, not hold indefinitely.

#### Prong 2 mechanism constraint (sage-review resolution)

"Mirror the CONNECTING guard" is insufficient as a spec: an OPEN socket has no
natural later completion event the way a CONNECTING socket has an open/error
event, so a naive OPEN guard (e.g. "never close if OPEN") risks leaking
connections on a genuine hide/unmount (tab closed, worktree removed, server
disconnected). The concrete teardown condition must be defined:

- Prefer keying socket teardown on **logical terminal presence** — whether the
  terminal is still present in the resource tree / still logically open —
  rather than on transient `paneVisible`. A `paneVisible=false` flip alone
  (pane hidden behind another Dockview tab, or the watchdog's
  `offsetParent === null` check firing during layout churn) must NOT tear down
  an OPEN socket for a terminal that is still logically open; only the
  terminal's actual removal from the resource tree / explicit close should.
- If a post-open grace window is used instead (e.g. "ignore a `paneVisible`
  drop within N ms of reaching OPEN"), the threshold must be stated explicitly
  and must be long enough to bridge one poll cycle's worth of selection churn
  (see the ~5s poll cadence and ~4s observed churn period in the causal
  chain above) — a value shorter than one poll interval does not defend
  against the actual observed failure mode.
- Prong 2 is defense-in-depth: once prong 1 removes the spurious selection
  flip, `paneVisible` should no longer flip for a still-open sub-worktree, so
  prong 2 should stop firing in the steady state. It exists to protect against
  any residual or other flip source (e.g. RU1 below, or an unrelated future
  regression), not as the primary fix.

Runner-up hypotheses to rule out during the fix:
- (RU1) the keep-alive / background-instance path (`App.tsx:4400-4424`) resolves
  via an *unprotected* raw lookup with no fallback cache, so a transient omission
  fully unmounts/remounts that root's Dockview + panes.
- (RU2) Phase 2 of the active-root refactor (atomic `selectRoot`, delete the
  sync-mount hack at `App.tsx:~1017-1041`) is still unlanded — a documented loose
  end in the same cluster; lower confidence for the repeating loop.

#### RU1/RU2 completion boundary (sage-review resolution)

- **RU1** (`App.tsx:4400-4424`): "ruled out" means confirming, during
  implementation, whether this raw-lookup path shares the same fallback
  protection as prong 1 (or is otherwise outside the repeating-loop path). If
  it turns out to be a live second unmount/remount source, fold a matching fix
  into this phase; otherwise document it in the Result as not-implicated —
  do not leave it unexamined.
- **RU2** (unlanded atomic `selectRoot` refactor, sibling ticket
  `260714-refactor-dashboard-active-root-atomic-select-pure-derivation`) is a
  separate refactor and is NOT required for this fix. Documenting it as
  out-of-scope for Phase 2 is sufficient — it does not need to land alongside
  this phase.
- Resolving RU1/RU2 beyond the documented check above is optional for Phase 2
  completion; the phase is complete once the two-pronged fix lands and the
  Acceptance/Verification boundary below is met, regardless of RU1/RU2
  disposition.

#### Phase 2 Acceptance / Verification boundary

- Dogfood reproduction: open a linked WSL sub-worktree terminal from the
  Windows frontend and confirm NO open/teardown churn (no ~4s socket
  recreation loop, no 8-pane bursts) over a sustained window (e.g. >= 5 min)
  in the daemon logs.
- Frontend build/typecheck passes.
- Named regression coverage: a unit test for `resolveWorkbenchSelection`
  (and/or `deriveWorkbenchView`) covering stickiness under a transient
  single-poll omission of the selected root; a test for the `App.tsx`
  OPEN-socket cleanup guard. Relay/terminal automated coverage is currently
  ~zero, so at minimum the selection-model logic (pure) must get a unit test
  even if the socket-guard behavior can only be covered by a lighter-weight
  check.

Deferred latent defects (real, but NOT this bug's root cause — kept so they are
not lost): the backend relay pump loop still tears the whole bidirectional session
down on any single-direction read error/EOF, now traced but with no
teardown-semantics fix (servers.rs:1621-1655); and `tokio-tungstenite` still
carries no TLS backend, so any `https://` linked server's terminal WS would fail
via `Error::Tls` collapsed into the same blanket 502. Add relay test coverage
(currently zero) alongside whichever of these is addressed.
