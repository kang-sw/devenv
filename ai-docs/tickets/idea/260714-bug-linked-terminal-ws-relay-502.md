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
