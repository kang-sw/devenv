---
title: Agent callback base URL uses the bound interface instead of loopback
sage-review-design: required
---

# Agent callback base URL uses the bound interface instead of loopback

## Background

Found by code review of PR #4 (`goal/ws-dashboard-dev/velvet-arbor-quill`, merged
as `1b41a37b`).

`server.rs` builds the agent callback base URL as
`format!("http://{}", display_addr(bound_addr))`. That string is written into
every token-bearing `callback.json` (`agent_callback.rs`, `terminal.rs`). The hook
always runs on the same host as the daemon, so loopback would always suffice —
using the bound interface buys nothing and costs two things under
`serve --bind-mode public`:

- **`--host <LAN ip>`**: the per-terminal bearer token is POSTed in cleartext over
  the network on every turn boundary (`terminal_notify.rs`). A sniffer gets a
  token valid for that terminal's entire lifetime — the token never rotates — and
  can forge turn states for it.
- **`--host 0.0.0.0`**: `display_addr` yields literally `http://0.0.0.0:PORT`.
  Linux and macOS accept that as loopback; Windows does not, so every hook fire
  fails there.

Everything else about the token design reviewed clean: ~190 bits of CSPRNG
entropy, a genuine constant-time comparison with a non-vacuity test, `0600` at
file creation with no chmod-after window, no token in argv/env/logs, cross-agent
replay blocked, and byte-identical 401s for unknown-terminal vs wrong-token. This
is the one place the token leaves the machine.

## Decisions

Build the callback base URL from `127.0.0.1:<bound_addr.port()>` (or `[::1]` when
the daemon bound v6-only) rather than from `bound_addr.ip()`.

Confirm at design review that no supported deployment runs the hook on a
different host from the daemon. If one exists, this becomes a transport-security
question instead of a URL-construction fix, and the ticket should be rescoped.

## Phases

### Phase 1: Derive the callback base URL from loopback

Change the construction site, keep the port from the actual bind, and cover the
`0.0.0.0` case explicitly so the Windows failure mode cannot regress silently.

While in this code, consider `.redirect(Policy::none())` on the
`terminal_notify.rs` client — it currently carries a bearer token under reqwest's
default 10-hop redirect policy. Not reachable today (the base URL comes from a
`0600` daemon-written file), but it costs nothing.
