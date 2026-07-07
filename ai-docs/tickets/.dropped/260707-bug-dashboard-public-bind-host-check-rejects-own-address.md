---
title: "ws-dashboard --bind-mode public rejects every request whose Host header is the daemon's own bound address"
related:
  260525-feat-ws-dashboard-server-scoped-operation-forwarding: blocks the ticket's stated remote-Windows-daemon dogfood gap
  260620-feat-ws-dashboard-loopback-no-auth-debug-mode: sibling bind-mode/auth guard work; did not cover this runtime Host-check path
spec:
  - 260516-ws-web-dashboard-protected-frontend-shell
sage-review: required
dropped: 2026-07-07
---

## Dropped (2026-07-07)

Misdiagnosed. The behavior described below is not a bug: it is a documented,
intentional security invariant —
`ai-docs/mental-model/ws-web-dashboard.md:60` states "Relaxing Host/Origin or
route auth is not part of public bind enablement," and
`ai-docs/spec/ws-web-dashboard/index.md` (Daemon Foundation section) says the
same. `--bind-mode public`'s non-loopback bind allowance was never meant to
make the daemon directly reachable by a client presenting a non-loopback Host
header; direct-endpoint linked-server forwarding to a `--bind-mode public`
remote is consequently not a supported combination as implemented.

The correct way to reach a genuinely remote daemon without violating the
invariant is the existing SSH-tunnel-based linked-server path (`ssh_target`/
`remote_endpoint_hint`, `servers.rs`), which forwards to the remote's
loopback-bound port so the Host header the remote daemon observes is always
loopback-compatible. Follow-up test-plan ticket:
`260707-chore-dashboard-linked-server-tunnel-dogfood-plan`.

# ws-dashboard --bind-mode public rejects every request whose Host header is the daemon's own bound address

## Background

Discovered while performing a live cross-environment dogfood of
`260525-feat-ws-dashboard-server-scoped-operation-forwarding`'s Phase 7
"linked remote server" flow: a WSL-side gateway daemon (normal owner-auth,
`127.0.0.1:8787`) linked to a native-Windows daemon bound non-loopback with
`--bind-mode public` (`192.168.208.1:4101`, normal owner-auth, no `--no-auth`).

The link handshake (`POST /api/dashboard/link-auth` with the printed
daemon-lifetime passphrase) succeeded and issued a valid bearer token. But the
very next forwarded call, `GET /api/dashboard/resources` (used by
`request_remote_resources` in
`ws-dashboard/crates/daemon/src/servers.rs:2019-2042`), came back `403`, which
`ResourceForwardError`'s status mapping (`servers.rs:484-496`) turns into a
misleading `502 Bad Gateway` / `"linked server resource request failed"`
surfaced to the browser (confirmed via a driver script hitting the real
gateway + real remote daemon, not a mock).

Root-caused with a controlled `curl` A/B test against the same live remote
daemon, same valid bearer token, identical request except for one header:

- `Host: 192.168.208.1:4101` (the daemon's own real bound address) → `403`
- `Host: localhost` → `200`

The rejection is `entrypoint_headers_allowed` (`auth.rs:274-277`) via
`is_allowed_host` (`auth.rs:308-335`), which only ever allows `localhost` or a
loopback IP literal — it has no branch for "the request's Host matches this
daemon's own configured (possibly non-loopback) bind address," and is not
aware of `ServeConfig.bind_mode` at all. `validate_bind_guard`
(`config.rs:48-82`) explicitly permits and, for non-loopback binds, *requires*
`--bind-mode public` with owner-auth enabled — but nothing downstream in the
request-auth path honors that mode. The result: `--bind-mode public` is
accepted at startup and then makes the daemon's protected routes (everything
behind `require_owner_auth` — file reads/writes, terminals, Git operations,
linked-server operations, static UI serving; see
`260620-feat-ws-dashboard-loopback-no-auth-debug-mode`'s Constraints section
for the full protected-route list) unreachable via the one address a remote
client can actually use to reach it. Confirmed this is not merely the
forwarding path: navigating a browser directly at the remote daemon's own
`http://192.168.208.1:4101/` origin (bypassing the gateway/link entirely)
gets the same `403` from `entrypoint_headers_allowed` once past pairing.

This appears to have never been exercised before: no prior ticket, spec, or
mental-model doc mentions `is_allowed_host`, `entrypoint_headers_allowed`, or
`BindMode::Public` together, and `260525`'s Phase 7 Result explicitly flagged
"live dogfood against a real remote Windows daemon" as never having been
performed in any session before this one.

## Phases

### Phase 1: Make the Host/Origin entrypoint check bind-mode-aware

- Extend the allowlist so a request's Host (and Origin) header is accepted
  when it matches the daemon's own configured bind address under
  `BindMode::Public` (or more precisely: whatever address the operator
  legitimately expects clients to reach the daemon at through that mode) —
  not just `localhost`/loopback.
- Do not weaken the default (`BindMode::Local`) posture: only widen the
  allowlist when `bind_mode == Public`, since that is the mode that already
  requires (and signals) an explicit, informed choice to serve non-loopback.
- Consider whether the right fix is "allow the exact configured
  `bind_addr`" vs. a broader "allow any Host when bind_mode is Public" — the
  former is more conservative and should be preferred unless it breaks a real
  deployment shape (e.g. a reverse proxy or SSH tunnel rewriting Host).
- Reproduce locally first with the `curl` Host-header A/B test described
  above (no cross-machine setup needed — a single `--bind-mode public` daemon
  bound to a real non-loopback interface reproduces it) before attempting a
  fix, then re-verify the same A/B test flips to `200`/`200`.
- Re-run the original cross-environment scenario (WSL gateway daemon linked
  to a native-Windows `--bind-mode public` remote) end to end afterward:
  resources listing, root picker, file read/write, Git status, terminal
  create, and the terminal WebSocket relay — this ticket's fix is only
  confirmed once that full flow works, not just the isolated `curl` check.

## Escalations

- None yet.
