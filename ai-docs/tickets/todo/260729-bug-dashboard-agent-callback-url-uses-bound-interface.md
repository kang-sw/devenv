---
title: Agent callback base URL uses the bound interface instead of loopback
sage-review-design: required
spec:
  - 260516-ws-web-dashboard-token-free-pairing-landing
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

**An earlier draft of this ticket prescribed unconditional loopback. That fix is
wrong and would have caused a worse defect than the one it closes.** Design
review caught it; the correction is recorded here so it is not re-proposed.

The daemon opens exactly one socket — `TcpListener::bind(config.bind_addr)` in
`server.rs`, where `bind_addr` is `SocketAddr::new(parse_bind_host(--host),
--port)`. Verified: that is the only production `TcpListener::bind` in the tree.
So the two motivating cases are *not* one case:

- **Wildcard bind (`0.0.0.0` / `::`)** — loopback is genuinely included in what
  the socket accepts, so a loopback callback URL is strictly correct and fixes
  the Windows breakage at no cost.
- **Explicit non-loopback bind (`--host 192.168.x.y`)** — the socket accepts only
  traffic destined to that IP. `http://127.0.0.1:PORT` is connection-refused.
  Unconditional loopback would convert a cleartext-token exposure into **total
  loss of turn-state callbacks**.

That loss would be silent. `terminal_notify.rs`'s module CONTRACT deliberately
never writes stdout/stderr and always exits 0, appending only to
`logs/terminal-notify.log` — the operator would see turn state simply stop
updating, with no error anywhere they look. Verified in the source.

Note also that public mode with a LAN host is a **spec-supported deployment**
(`ai-docs/spec/ws-web-dashboard/index.md`), so silently degrading it narrows a
documented capability.

The real decision, unsettled:

- **(a) Conditional loopback.** Use loopback only when
  `bound_addr.ip().is_unspecified() || is_loopback()`; keep the bound IP
  otherwise. Fixes the Windows `0.0.0.0` break with zero functional loss, but
  leaves the token on the wire in exactly the case this ticket named first.
- **(b) Unconditional loopback, degradation declared.** Accept that an explicit
  LAN bind loses turn-state notification. Only defensible if the degradation is
  stated as intended *and surfaced at startup* — discovering it from a log file
  is not acceptable for a spec-supported deployment.
- **(c) Bind an additional loopback listener** so both properties hold: the
  callback is local-only by construction, and the LAN bind keeps working.

**Recommendation: (c), with (a) as the fallback if the second listener is judged
too invasive.** The callback is a local-only concern that currently borrows a
socket built for remote browsers; giving it its own loopback socket is the fix
that matches the actual requirement rather than trading one failure for another.
(a) is safe and cheap but leaves the original security finding open, so choosing
it means consciously accepting the cleartext token on LAN binds — record that if
it is chosen.

Secondary, resolvable without asking:

- The IPv6 rule is on the address family — `[::1]` when `bound_addr.is_ipv6()`,
  `127.0.0.1` otherwise. An earlier draft said "when the daemon bound v6-only",
  which names an `IPV6_V6ONLY` socket option the code never sets and cannot
  portably read back; do not detour into probing it.
- `base_url` feeds two consumers, not one: `agent_callback::write_bound_base_url`
  (the global `bound-base-url.json`) and `TerminalRegistry::boot_reconcile`, in
  addition to the per-terminal `callback.json`. Nothing reads
  `bound-base-url.json` back in-process — its own CONTRACT forbids deriving a
  per-terminal target from it — so the effect is benign, but the spec wording
  must cover both files.

## Spec Impact

Target area: `ai-docs/spec/ws-web-dashboard/index.md`,
`{#260516-ws-web-dashboard-token-free-pairing-landing}`.

That stem already carves the per-terminal turn-state callback route out of the
browser-facing auth rule as "a non-browser, token-authed exception". It says
nothing about where the callback token travels. This change makes that explicit
and caller-visible:

- The callback base URL is loopback-derived, not bind-derived, so the
  per-terminal token never leaves the host regardless of `--bind-mode` or
  `--host`.
- State the consequence that follows: the hook is required to run on the same
  host as the daemon. Today that is an unstated assumption the code happens to
  satisfy; after this change it becomes a contract the URL construction enforces.

The `0.0.0.0` case is worth naming in the spec rather than only in code, because
its current failure is platform-split (accepted as loopback on Linux/macOS,
rejected on Windows) and that asymmetry is exactly the kind of thing a future
reader will otherwise rediscover the hard way.

## Phases

### Phase 1: Make the callback target local-only without breaking a LAN bind

**Blocked on the Decisions section's open question (a / b / c).** The three
options produce different code and different capability guarantees; do not start
until one is chosen.

Whichever is chosen, keep the port from the actual bind, and cover both
`base_url` consumers — the per-terminal `callback.json` and the global
`bound-base-url.json`.

Verification — the point of this phase is that the failure mode *cannot regress
silently*, so a manual check does not discharge it:

- A unit test over the URL-construction function asserting that a bind on a
  non-loopback address (a LAN-style `SocketAddr` and `0.0.0.0:PORT`) still yields
  a `127.0.0.1:<same-port>` base URL. This is the one that would have caught the
  original defect.
- An assertion that the port is taken from the actual bind, not a default —
  otherwise a loopback-hardcoding fix silently breaks every non-default-port
  deployment.
- A test that the string written into `callback.json` is the loopback-derived
  one, not just that the helper returns it. The defect was in what got
  *persisted*; testing only the helper leaves the wiring uncovered.

Reverting the fix must fail the first of these. If it does not, the test is not
covering the defect.

While in this code, consider `.redirect(Policy::none())` on the
`terminal_notify.rs` client — it currently carries a bearer token under reqwest's
default 10-hop redirect policy. Not reachable today (the base URL comes from a
`0600` daemon-written file), but it costs nothing.
