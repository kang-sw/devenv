---
title: Agent callback base URL uses the bound interface instead of loopback
sage-review-design: completed
spec:
  - 260516-ws-web-dashboard-token-free-pairing-landing
sage-review-completeness: completed
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

### The topology, which settles the decision

Mapped against source, because the option weighting depends entirely on it:

- **A public LAN bind has real, non-browser clients.**
  `/api/dashboard/servers/{server_route}/…` resolves through
  `resolve_server_scoped_forwarding`: `server-local` dispatches in-process, any
  other `serverId` is proxied over HTTP to *another daemon*, and a peer daemon
  authenticates at `POST /api/dashboard/link-auth`. So the public socket serves
  browsers **and** peer daemons.

  **Do not overstate this** — an earlier draft called a LAN bind "the
  precondition for the linked-server gateway", and the shipped `--remote-guide`
  says the opposite: `browser → local daemon → SSH tunnel → remote daemon`,
  "prefer binding the remote daemon to remote loopback", "do not expose the
  remote daemon on a public interface for the MVP path", codified as
  `{#260525-ws-dashboard-remote-deployment-guide}`. The tunnel path even rejects
  non-loopback endpoints outright (`remote_loopback_port`). A LAN-bound remote is
  *reachable* — `normalize_dashboard_endpoint` accepts any http/https host — but
  it is the alternative path, not the blessed one. This wording must not be
  carried into the spec.
- **A helper is always co-host with its owning daemon.** The spawn path is a
  plain local `std::process::Command` on `current_exe()`; there are zero `ssh`
  references anywhere in `crates/daemon/src/terminal*.rs` (SSH lives only in
  `servers.rs`). No daemon ever manages a helper across the wire.
- **Therefore the hook is always co-host with the daemon it POSTs to — in every
  topology, including the linked one.** The gateway forwards over HTTP and the
  request terminates at the remote daemon's *unscoped* handler, so a remote
  terminal's hook targets that remote daemon's own `base_url` and never traverses
  the tunnel.

So the callback is local by construction, and the bound interface is the wrong
source for its URL in a way that has nothing to do with which interface is bound.
Under `--host <LAN ip>` the hook currently hairpins out to
`http://<LAN ip>:<port>` and back in — needless before it is unsafe.

**Nothing in the spec or the mental models states where the callback must be
reachable from.** The nearest statement is a code comment that assumes the
opposite of what the code does: `terminal_notify.rs` sizes `CONNECT_TIMEOUT` for
"a same-host TCP connect (normally sub-millisecond, even over `--bind-mode
public` on a slow network)". That divergence is itself worth recording.

### Decision: (c)

The options were:

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

**(c) is chosen. The topology above eliminates the other two.**

- **(b) is out.** It breaks turn-state notification on an explicit LAN bind.
  `--bind-mode public` with a non-loopback host is spec-supported, the user runs
  the dashboard that way, and a phone or LAN browser keeps the case live
  regardless. Silently degrading a supported deployment to close a token exposure
  is a worse trade than the exposure.
- **(a) is out on weighting, not on correctness.** It is safe and cheap and would
  have been right if LAN binds were vanishingly rare. They are not — see above —
  so (a) leaves the cleartext token on the wire in a deployment that is actually
  used.
- **(c) matches the actual requirement.** The callback is a local-only concern
  that today borrows a socket built for remote browsers and peer daemons. Giving
  it its own loopback socket keeps the LAN bind fully functional and keeps the
  token on the host, with no capability traded away.

(c) is also a strict superset of (a): it does everything (a) does and additionally
keeps the token local on an explicit LAN bind. So choosing it costs nothing that
(a) would have preserved. The decision is settled.

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
| `127.0.0.2:P` | **none needed** | still loopback — but the URL must keep `127.0.0.2`, not be rewritten to `127.0.0.1` |

That last row is not pedantry. `--host 127.0.0.2` is accepted by
`parse_bind_host` and passes `validate_bind_guard`, and a rule written as
"loopback primary → emit `127.0.0.1`" would emit a URL with **no listener behind
it** — the silent total-callback-loss mode that disqualifies option (b),
reintroduced by the fix meant to prevent it. The rule is: a loopback primary
keeps its own address.

**Failure policy for the second bind.** The collision argument above is about the
daemon's own primary socket; nothing stops an unrelated process from already
holding `127.0.0.1:P`. Decide and state what happens then — fail startup loudly,
or fall back to (a) semantics for that run with a warning. What must not happen is
a silently unbound listener with a loopback URL published anyway.

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

Existing coverage worth knowing about:
`tests/terminal_notify_end_to_end.rs` asserts `callback.json`'s `baseUrl` equals
the daemon's own bound URL. Its harness spawns with `--bind-mode local` and the
default host, so the emitted URL stays `http://127.0.0.1:<port>` and the test
stays green under (c). An earlier draft warned it would break; it will not. Check
it rather than pre-emptively rewriting it.

A consumer of `display_addr` this ticket must **not** touch: `startup_info`'s
`pairing_url` and `direct_dashboard_url`. Those are for a human with a browser
and must keep the primary bound address. Only the callback URL becomes
loopback-derived.

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
`{#260516-ws-web-dashboard-token-free-pairing-landing}` — **the only anchor this
ticket edits.**

The gateway anchors the Decisions section leans on
(`{#260525-ws-dashboard-linked-server-registry-gateway-skeleton}`,
`{#remote-terminal-http-lifecycle}`, `{#remote-terminal-websocket-gatewaying}`)
are **context, not targets**. Nothing about the linked-server contract changes:
the gateway forwards HTTP exactly as it does today, and this fix touches only
where the *local* callback URL points. Deliberately out of scope, recorded here
so a reader does not go looking for edits that should not exist.

That stem already carves the per-terminal turn-state callback route out of the
browser-facing auth rule as "a non-browser, token-authed exception". It says
nothing about where the callback token travels. This change makes that explicit
and caller-visible.

Under the chosen option (c) the spec states the strong form:

- The callback base URL is loopback-derived, so the per-terminal token never
  leaves the host regardless of `--bind-mode` or `--host`, and a LAN bind keeps
  serving browsers and peer gateway daemons unchanged.
- The hook is required to run on the same host as the daemon it reports to. Today
  that is an unstated assumption the code happens to satisfy; after this change
  it becomes a contract the URL construction enforces. Worth stating explicitly
  that this holds in the **linked-server topology too** — a remote terminal's
  hook targets the remote daemon directly and never traverses the gateway tunnel,
  which is not obvious from the route table.

The `0.0.0.0` case is worth naming in the spec rather than only in code, because
its current failure is platform-split (accepted as loopback on Linux/macOS,
rejected on Windows) and that asymmetry is exactly the kind of thing a future
reader will otherwise rediscover the hard way.

While here, reconcile the code comment that already assumes this contract:
`terminal_notify.rs` sizes `CONNECT_TIMEOUT` for "a same-host TCP connect … even
over `--bind-mode public` on a slow network", which reasons about locality the
code does not currently enforce. After Phase 1 the comment becomes true; adjust
its parenthetical so it stops implying the connect may cross a network.

## Phases

### Phase 1: Give the callback its own loopback listener

Implement option (c) per Decisions — settled, not open. Keep the port from the
actual bind, and cover both `base_url` consumers: the per-terminal
`callback.json` and the global `bound-base-url.json`.

Bind the second listener **only** when the primary bind is an explicit
non-loopback address; a wildcard or loopback primary already accepts loopback and
a second bind there is the `EADDRINUSE` case. Discharge all four obligations in
Decisions — they are not optional.

Verification — the point of this phase is that the failure mode *cannot regress
silently*, so a manual check does not discharge it:

Split the coverage in two, because the hermetic half and the half that needs a
real interface have different failure modes.

**Pure decision function — always runs, no sockets.** Extract the "given the
primary `SocketAddr`, what is the callback URL and is a second listener needed?"
decision and table-test it: LAN-style → `127.0.0.1:<same-port>` + listener
needed; `0.0.0.0:P` and `[::]:P` → loopback URL, **no** listener (this is the
`EADDRINUSE` case); `127.0.0.1:P` → itself, no listener; `127.0.0.2:P` → **itself,
not `127.0.0.1`**, no listener; v6 primary → `[::1]`. Reverting the fix must turn
the LAN-style row red.

**Integration — needs a routable non-loopback local address.** That address is
not guaranteed to exist, and this is the item that separates (c) from "a URL
pointing at nothing", so it must not degrade into a silent skip. Enumerate local
interfaces for a non-loopback address; if none exists, the test **fails loudly as
unrunnable** rather than passing. With one: bind the primary there, then assert
the loopback listener is really bound and the turn-state route answers a real
request on it — not merely that the emitted string is loopback.

Two obligations the list above cannot catch, so assert them directly:

- **`mark_socket_non_inheritable` on the new listener.** A leaked or inheritable
  second listener reproduces the exact `260723` Windows port-pinning regression —
  on a socket this fix introduces. Assert the call, or assert the observable
  (a spawned child does not inherit the handle).
- **Both `axum::serve` futures inside the one graceful-shutdown `select!`.**
  Assert that shutdown actually closes the loopback listener; a second server
  wired alongside rather than into the shutdown path leaves the port held after
  the daemon is told to stop.
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
