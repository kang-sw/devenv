---
title: Linked-server terminal WebSocket relay fails its first real WSL<->Windows exercise with an undiagnosable blanket 502
---

# Linked-server terminal WebSocket relay fails its first real WSL<->Windows exercise with an undiagnosable blanket 502

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
