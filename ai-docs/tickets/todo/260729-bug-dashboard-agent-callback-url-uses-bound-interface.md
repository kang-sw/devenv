---
title: Agent callback base URL uses the bound interface instead of loopback
sage-review-design: blocked
spec:
  - 260516-ws-web-dashboard-token-free-pairing-landing
sage-review-completeness: blocked
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
--port)`. Verified: that is the only production `TcpListener::bind` that *serves*
— the one other production bind, `allocate_loopback_port` in `servers.rs`, is a
throwaway port probe that is dropped immediately. It is also the in-repo
precedent for allocating a loopback port, so it is worth reading under option (c).
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

**(c) is conditional, and that is not a weakness — it is why it works.** Design
review raised that `127.0.0.1:PORT` collides with a wildcard `0.0.0.0:PORT` bind
(`EADDRINUSE`), concluding (c) collapses into (a)-plus-a-socket with an
unpredictable port. The collision is real in the abstract but **never arises
here**, because the second socket is needed in exactly the case where it cannot
collide:

| primary bind | second listener | why |
| --- | --- | --- |
| `0.0.0.0:P` / `[::]:P` | **none needed** | the wildcard socket already accepts loopback; just emit a loopback URL |
| `192.168.x.y:P` | `127.0.0.1:P` | a specific-address bind reserves only that `(addr, port)` pair, so loopback on the same port is free |
| `127.0.0.1:P` | **none needed** | already loopback |

So (c) keeps the **same port** in every case, and Phase 1's
`127.0.0.1:<same-port>` assertion stays satisfiable. What the implementer must
not do is attempt the second bind unconditionally — under a wildcard primary that
is the `EADDRINUSE` the review predicted.

Obligations (c) carries that (a) and (b) do not, all visible at the existing call
site and none of them optional:

1. Apply `terminal_platform::windows::mark_socket_non_inheritable` to the new
   listener. Omitting it re-opens the Windows port-pinning regression closed by
   `260723`.
2. Wire both `axum::serve` futures into the existing single-server
   `tokio::select!` graceful-shutdown path, not alongside it.
3. Decide whether the loopback socket serves the full cloned `Router` — which
   republishes the entire dashboard and pairing surface on a second port — or a
   callback-only router. Prefer callback-only; state the choice either way.
4. **The per-terminal token check stays mandatory.** "Local-only by construction"
   bounds who can reach the socket to local users, which is not the same as
   trusting them. The turn-state route is registered outside `require_owner_auth`
   and authorized by the token inside the handler; that must not change.

Existing coverage to update, not just add:
`tests/terminal_notify_end_to_end.rs` asserts `callback.json`'s `baseUrl` equals
the daemon's own bound URL. Any option that changes the emitted URL breaks it.

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
and caller-visible.

**What the spec must state depends on which option is chosen — the three do not
share a guarantee.** Do not write the wording below before the decision is made:

- Under **(b)** or **(c)**, the spec can state the strong form: the callback base
  URL is loopback-derived, so the per-terminal token never leaves the host
  regardless of `--bind-mode` or `--host`. (b) must additionally state that an
  explicit non-loopback `--host` loses turn-state notification, and where the
  operator is told so.
- Under **(a)**, that strong form is **false** — an explicit non-loopback
  `--host` still puts the token on the wire on every turn boundary. The spec must
  instead state the conditional rule (loopback only for wildcard/loopback binds)
  and name the residual exposure as accepted, so it is not mistaken for closed.

Common to all three: state the consequence that the hook is required to run on
the same host as the daemon. Today that is an unstated assumption the code
happens to satisfy; after this change it becomes a contract the URL construction
enforces.

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

- A unit test over the URL-construction function. **The expected value differs by
  option, so write the test after the decision, not before:**
  - Under **(b)/(c)**: both a LAN-style `SocketAddr` and `0.0.0.0:PORT` yield
    `127.0.0.1:<same-port>`.
  - Under **(a)**: `0.0.0.0:PORT` yields `127.0.0.1:<same-port>`, but a LAN-style
    bind deliberately yields the bound IP unchanged. Asserting loopback there
    would encode a guarantee (a) does not make.

  Either way this is the test that would have caught the original defect, because
  the `0.0.0.0` case is common to all three.
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

## Blocked (2026-07-29)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Phase 1 self-declares a blocking (a)/(b)/(c) decision that trades security against a spec-supported deployment; not derivable by an implementer | critical | missing |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
