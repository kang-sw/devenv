---
title: "Linked-server endpoint using bare `localhost` can wedge the daemon's outbound client on WSL2 (IPv6 ::1 not forwarded)"
related:
  260525-feat-ws-dashboard-endpoint-linked-server-add: introduced the direct-endpoint linked-server path this finding affects
  260707-chore-dashboard-linked-server-tunnel-dogfood-plan: adjacent WSL2/Windows loopback-forwarding dogfooding for the same feature, opposite topology direction
---

# Linked-server endpoint using bare `localhost` can wedge the daemon's outbound client on WSL2 (IPv6 ::1 not forwarded)

## Background

Dogfooding the endpoint linked-server feature
(`POST /api/dashboard/servers/link`,
`260525-ws-dashboard-endpoint-linked-server-add`) across a WSL2 host: a
Windows-native ws-dashboard daemon (gateway/frontend) was linked to a
WSL-side daemon.

Linking with `endpoint: http://localhost:8787` made the Windows daemon's
outbound `reqwest` HTTP client hang, with no quick error surfaced back to
the caller. Root cause: `localhost` resolved to IPv6 `::1` on the Windows
side, and WSL2 does **not** forward `::1` from Windows to the WSL-hosted
service, which binds `127.0.0.1:8787` (IPv4 only). The daemon did not
fast-fail on the unreachable `::1` target — it briefly wedged instead of
surfacing a prompt connection-refused/timeout error.

Workaround that resolved it immediately: use the IPv4 literal
`endpoint: http://127.0.0.1:8787` instead of `localhost`.

This is low-severity — a trivial workaround exists once diagnosed — but a
genuine operational papercut: a bare `localhost` endpoint is a very natural
thing for a user to type when linking a server, and the failure mode (a
silent hang rather than a fast, legible error) is worse than the underlying
connectivity gap itself.

## Discussion needed (TBD)

Relevant code area to start from: `ws-dashboard/crates/daemon/src/servers.rs`
(the linked-server outbound `reqwest` client used by the direct-endpoint
path). No design is committed yet; open questions for whoever picks this up:

- Should the outbound client prefer/try IPv4 first for loopback-shaped
  endpoints (a `localhost`/happy-eyeballs-style resolution order), so
  `::1` unreachability on WSL2 doesn't surface as a hang at all?
- Alternatively (or additionally), should a connect timeout be applied to
  this outbound client so an unreachable candidate address fails fast with
  a clear error, regardless of root cause — this seems worth doing even if
  the IPv4-preference idea above isn't adopted?
- Is this WSL2-specific, or does the same wedge risk apply to any host
  where `localhost` resolves to an address family the target isn't actually
  listening on?

Not actionable until scoped; filing this as an idea ticket rather than
re-investigating live, per the dogfooding surprise this was captured from.
