---
title: "Dogfood the SSH-tunnel and localhost-forwarding linked-server paths across a real WSL/Windows boundary"
related:
  260525-feat-ws-dashboard-server-scoped-operation-forwarding: this plan closes the ticket's outstanding live-dogfood gap
spec:
  - 260525-ws-dashboard-ssh-tunnel-reconnect
  - 260525-ws-dashboard-endpoint-linked-server-add
  - 260515-ws-web-daemon-foundation
sage-review-design: completed
sage-review-completeness: completed
sage-review: completed
---

# Dogfood the SSH-tunnel and localhost-forwarding linked-server paths across a real WSL/Windows boundary

## Background

A live cross-environment dogfood attempt (2026-07-07, see
`ai-docs/_index.local.md` run log — gitignored, machine-local) tried linking a
WSL-side gateway daemon to a native-Windows remote daemon bound non-loopback
(`--bind-mode public`) via the direct-endpoint linked-server path
(`260525-ws-dashboard-endpoint-linked-server-add`). It failed: the remote's
own `entrypoint_headers_allowed`/`is_allowed_host` check
(`ws-dashboard/crates/daemon/src/auth.rs:274-335`) rejects any Host header
that isn't `localhost`/loopback, and `--bind-mode public` deliberately does
not relax that check —
`ai-docs/mental-model/ws-web-dashboard.md:60` ("Relaxing Host/Origin or route
auth is not part of public bind enablement") and the Daemon Foundation spec
section both confirm this is an intentional invariant, not a bug. A ticket
that proposed relaxing it (`260707-bug-dashboard-public-bind-host-check-rejects-own-address`)
was dropped for that reason.

The correct way to reach a genuinely separate remote host without violating
the invariant is the existing SSH-tunnel linked-server path
(`260525-ws-dashboard-ssh-tunnel-reconnect`, `ssh_target`/
`remote_endpoint_hint` in `servers.rs`): the gateway opens a local loopback
forward to the remote's own loopback-bound port via `ssh -N -L
127.0.0.1:<local>:127.0.0.1:<remote> <ssh_target>`
(`servers.rs:2144-2172`), so every request the remote daemon receives arrives
with a loopback-compatible Host header regardless of which two machines are
actually involved. This mechanism has no OS-specific branching and has never
been exercised end-to-end in any session, on any platform combination.

Two additional facts, confirmed empirically during this investigation and
recorded in `ai-docs/_index.local.md`, narrow what this plan needs to check
rather than guess:

- This WSL2 install uses default NAT networking (no `networkingMode=mirrored`
  in `/etc/wsl.conf`): WSL cannot reach a Windows-loopback-bound service via
  `localhost`/`127.0.0.1`, confirmed by testing a known-firewall-approved
  daemon binary bound to `127.0.0.1` on the Windows side and curling it from
  WSL by hostname and by the WSL2 gateway IP — both failed to connect.
- The reverse direction works out of the box: a WSL-loopback-bound listener
  (`127.0.0.1:18081`) was reachable from Windows via plain
  `http://localhost:18081/` (`200 OK`), because WSL2's default localhost
  port-forwarding already covers Windows-to-WSL. This means a
  **Windows-hosted gateway linking to a WSL-hosted remote via the
  direct-endpoint path, using `http://localhost:<port>` as the endpoint, may
  already work today with no SSH tunnel and no `--bind-mode public`** — the
  remote never sees a non-loopback Host header because the Windows client's
  request Host is literally `localhost`. This is an untested but
  architecturally sound alternate/comparison path, in the opposite topology
  direction from the failed 2026-07-07 attempt.

SSH connectivity between the two hosts for this project's purposes is
unconfirmed: the user recalls having registered the WSL side's SSH public key
in the Windows host's `authenticated_keys` at some point, but this was never
verified in-session and may be stale, missing an SSH server on the Windows
side, or otherwise not currently working.

## Constraints

- Do not weaken `entrypoint_headers_allowed`/`is_allowed_host` or any
  Host/Origin/bind-mode invariant while investigating — that direction was
  already considered and rejected (see Background).
- Treat pairing tokens, link passphrases, and bearer tokens as credentials:
  never echo or persist them to a file or transcript; capture into a shell
  variable and consume in the same command chain, per the pattern recorded in
  `ai-docs/_index.local.md`.
- The current `LinkedServerModal` frontend UI (`App.tsx`) only exposes the
  direct-endpoint add-server fields (label, endpoint, passphrase); it has no
  input for `ssh_target`. The SSH-tunnel path's HTTP API exists in
  `servers.rs` (`connect_dashboard_server_tunnel` and related routes) but
  must be exercised directly (e.g. via `curl`) rather than through the
  browser UI until/unless a UI gap ticket is separately filed.

## Phases

### Phase 1: Confirm SSH connectivity, then dogfood both linked-server paths

1. **Confirm SSH connectivity first, before any dashboard-specific work**:
   from WSL, attempt a non-interactive SSH connection to the Windows host
   (`ssh -o BatchMode=yes -o ConnectTimeout=5 <user>@<windows-host> true`,
   using the current WSL2 gateway IP re-derived via `ip route show default`
   — do not reuse a previously recorded IP). If this fails, diagnose whether
   an SSH server is running/reachable on the Windows side (e.g. the
   "OpenSSH Server" optional Windows feature) and whether the WSL public key
   is actually present in the expected Windows `authorized_keys` location,
   before attempting any tunnel-based linking. Record whatever is found;
   fixing a broken SSH setup is in scope for this phase if it's a
   configuration gap, but do not invest in provisioning a new SSH server if
   one was never intended to exist.
2. **If SSH connectivity is confirmed**, dogfood the SSH-tunnel linked-server
   path end to end:
   - Launch the remote Windows daemon bound to loopback only
     (`--host 127.0.0.1`, default `local` bind mode, normal owner-auth — no
     `--bind-mode public` needed for this path).
   - From the WSL-side gateway daemon (normal owner-auth), drive the
     SSH-tunnel connect/reconnect API directly (`curl`, since the UI has no
     field for this yet) with the remote's `ssh_target` and a remote startup
     command, per `260525-ws-dashboard-ssh-tunnel-reconnect`.
   - Verify the full forwarded-operation set: resources listing, root
     picker/open workRoot, file read/write, Git status/branches, terminal
     create, and the terminal WebSocket relay (typed input round-trips
     through the relayed connection).
3. **Regardless of the SSH result**, dogfood the reversed-topology
   direct-endpoint variant as a comparison/fallback: Windows-hosted gateway
   daemon (normal owner-auth) linking to a WSL-hosted remote daemon bound to
   `127.0.0.1` (default `local` bind mode, normal owner-auth), added via the
   endpoint field as `http://localhost:<port>`. Confirm whether linking and
   the same forwarded-operation set (resources, files, Git, terminal,
   WebSocket relay) succeed without any Host-check failure.
4. **Record the outcome** as a dated append to
   `260525-feat-ws-dashboard-server-scoped-operation-forwarding` (do not edit
   its frozen `### Result` text in place) — this is the ticket whose
   "live dogfood against a real remote Windows daemon" gap this phase exists
   to close. Note explicitly which of the two paths (SSH-tunnel,
   reversed-topology direct-endpoint) actually succeeded, and whether either
   surfaced a genuine implementation bug (as opposed to the already-settled
   non-bug from the dropped ticket).
5. If step 2 or step 3 surfaces a genuine bug (not another Host/Origin
   misdiagnosis), file it as its own ticket rather than folding a code fix
   into this verification-only ticket.
6. **Tear down all test artifacts** once the outcome is recorded: kill every
   spawned daemon process (remote and gateway, on both hosts), close any
   active SSH tunnel process, and remove any linked-server entries created
   solely for this dogfood run from each daemon's persisted state.

### Result

Partially executed on `implement/phase-1-ssh-tunnel-dogfood` (commits
`6faff2fa`, `1d2372f3`). Outcome recorded in full as a dated append to
`260525-feat-ws-dashboard-server-scoped-operation-forwarding`'s ticket.

- Step 1 (SSH connectivity probe): **not run**. The harness's own auto-mode
  classifier denied the non-interactive `ssh ... true` probe as "Credential
  Exploration" before any SSH attempt executed. `sshd` was independently
  confirmed running on the Windows host (`Get-Service sshd` -> Running), but
  connectivity itself remains unconfirmed. This is a session-tooling
  limitation, not a daemon or plan defect.
- Step 2 (SSH-tunnel leg): **not exercised**, blocked by step 1 never
  running.
- Step 3 (reversed-topology direct-endpoint leg): link handshake succeeded
  (`200 connected`, confirming the Host-check non-bug does not resurface for
  `http://localhost:<port>`), but every subsequent forwarded call 404'd. Root
  cause is a genuine new bug, not another Host/Origin misdiagnosis: filed as
  `260707-bug-dashboard-windows-daemon-state-persistence-silently-noop`
  (`todo/`).
- Step 4 (record outcome in 260525): done.
- Step 5 (file genuine bug separately): done — see above.
- Step 6 (teardown): done — all daemons killed, scratch state dirs removed,
  no SSH tunnel process existed (leg never ran), no pre-existing persisted
  state touched on either host.

Left open rather than closed: the SSH-tunnel leg (this ticket's primary,
more production-representative topology) is still unverified, and the
reversed leg cannot be re-walked past the link step until the new state-
persistence bug is fixed. Re-run this phase (or open a Phase 2) once (a) SSH
connectivity can be probed in a session without the auto-mode block, and (b)
`260707-bug-dashboard-windows-daemon-state-persistence-silently-noop` lands a
fix.

### Phase 2: Re-walk the reversed-topology leg now that the state-persistence bug has a fix

Depends on `260707-bug-dashboard-windows-daemon-state-persistence-silently-noop`
(the fix is merged to `ws-dashboard-dev`; its own Phase 2 HTTP repro may still
be in flight — this phase can proceed in parallel, it only needs the code
fix present, not that ticket's full closure). Does **not** depend on SSH
connectivity — this phase covers only Phase 1's step 3 leg, not steps 1/2.

1. Launch the reversed topology from Phase 1 step 3 again: Windows-hosted
   gateway daemon (normal owner-auth) linking a WSL-hosted remote daemon
   bound to `127.0.0.1` (default `local` bind mode, normal owner-auth), added
   via the endpoint field as `http://localhost:<port>`.
2. Confirm the link handshake still succeeds (`200 connected`), then walk the
   **full** forwarded-operation set that Phase 1 step 3 never reached because
   of the bug: resources listing, root picker, work-roots/open, file
   read/write, Git status/branches, terminal create, and the terminal
   WebSocket relay (typed input round-trips).
3. Confirm none of the above 404 the way the original Phase 1 attempt did —
   this is the direct end-to-end confirmation that the state-persistence fix
   resolves the originally discovered symptom, not just the unit-test-level
   verification already recorded on the bug ticket.
4. Record the outcome as a further dated append to
   `260525-feat-ws-dashboard-server-scoped-operation-forwarding` (same target
   as Phase 1 step 4), noting this is the reversed-topology leg's first full
   forwarded-operation confirmation.
5. If anything beyond the already-fixed 404 symptom surfaces, file it as its
   own ticket rather than folding a code fix into this verification-only
   ticket (same rule as Phase 1 step 5).
6. Tear down all test artifacts per Phase 1 step 6's discipline.

The SSH-tunnel leg (Phase 1 steps 1-2) remains blocked on the escalation
below and is out of scope for this phase.

### Result

Executed on `implement/reversed-topology-rewalk` (commit range after
`55eba9ed`). Full outcome recorded as a dated append to
`260525-feat-ws-dashboard-server-scoped-operation-forwarding` (see
"Verification note - 2026-07-10 (reversed-topology full forwarded-op
re-walk)").

- Step 1 (relaunch the reversed topology): done. Native Windows gateway
  daemon (WSL-interop build recipe, disposable detached worktree) linked to
  a WSL-hosted remote daemon via the direct-endpoint field
  (`http://localhost:<port>`), both `--bind-mode local` with normal
  owner-auth, both on a scratch `WS_DASHBOARD_STATE_HOME` for this run.
- Step 2 (confirm link handshake, walk the full forwarded-operation set):
  done. Link `200 connected`; every route in `router.rs:94-215` walked
  (resources, root-picker, work-roots/open, files read/write, git
  status/branches, terminal create) returned `200`; the terminal WebSocket
  relay connected with the same owner cookie and the input/output round
  trip was observed directly (`echo dogfood-ws-relay-ok` sent, echoed output
  received back through the relay).
- Step 3 (confirm no 404 recurrence): done. None of the forwarded calls
  reproduced the original `404 "unknown server"` symptom; the Windows
  gateway's log had no `"no dashboard state file could be resolved"`
  warning before the walk began, confirming the state-persistence fix was
  in effect for this build.
- Step 4 (record outcome in 260525): done — see the dated append above.
- Step 5 (file a new ticket if a genuine gap surfaces): not needed. The only
  anomaly was a client-side `ClientWebSocket.CloseAsync().Wait()` exception
  in the Windows PowerShell 5.1 test harness itself, thrown *after* the
  functional round trip already succeeded — a test-tooling artifact, not a
  daemon-side finding, so no new ticket was filed.
- Step 6 (teardown): done — both daemon processes stopped, both scratch
  state directories removed, and the disposable
  `D:\scratch-reversed-rewalk` worktree removed.

Net effect: the reversed-topology leg's full forwarded-operation set is now
confirmed end-to-end with the state-persistence fix in place, closing this
ticket's remaining gap for that leg specifically. The SSH-tunnel leg (Phase
1 steps 1-2) is unchanged by this phase and remains blocked on the
escalation below.

## Escalations

- SSH connectivity (Phase 1, step 1) could not be probed in this session:
  the auto-mode classifier blocks a bare non-interactive `ssh ... true`
  liveness check as "Credential Exploration." Needs either an explicit
  human-run probe outside this harness, or a narrower probe shape that
  doesn't trip the classifier, before the SSH-tunnel leg can be attempted.

#### Finding (2026-07-07): plain-TCP relay is a viable SSH-tunnel substitute

Prompted by the user's observation that the SSH-tunnel mechanism's only real
job is making the daemon's own outbound Host header loopback-compatible, not
providing the transport itself — tested a plain TCP relay in place of SSH as
a comparison path, separate from (and not blocked by) the still-open SSH
escalation above.

Setup: Windows daemon bound `192.168.208.1:<port>` (`--bind-mode public`,
normal owner-auth, same shape as the already-settled non-bug reproduction) +
a small Python `asyncio` TCP proxy on the WSL side listening on
`127.0.0.1:18099` and forwarding raw bytes to `192.168.208.1:<port>` (no
`socat`/`ncat` available in this WSL install; wrote a ~30-line asyncio
listen-and-pipe script instead) + the WSL gateway daemon (`127.0.0.1`,
`local` bind mode, normal owner-auth) linking the Windows remote via the
already-proven direct-endpoint path (`POST /api/dashboard/servers/link`,
`endpoint: "http://127.0.0.1:18099"`).

Result: link handshake returned `200 connected`; the forwarded
`GET .../resources` call returned `200` with a real, correctly-shaped body.
No Host-check `403` — confirms the mechanism generalizes: any loopback-bound
local forwarder (SSH tunnel, plain TCP relay, or otherwise) satisfies
`is_allowed_host` identically, because the check only inspects the Host
header the daemon's own outbound HTTP client sends, which is always
loopback-literal when the target URL is `127.0.0.1:<port>`.

Caveat: unlike SSH, a plain TCP relay has no transport-level encryption or
authentication — only the dashboard's own owner-auth (cookie/bearer) protects
the hop. Acceptable for this local WSL<->Windows-host virtual network
(same trust boundary as loopback in practice), not a general SSH
replacement for a genuinely untrusted network path.

Scope: this finding is a comparison/fallback data point, not a completed
walk of the ticket's SSH-tunnel leg (step 2) — only the link + one
forwarded-resources call were exercised, not the full forwarded-operation
set (files, Git, terminal, WebSocket relay). The SSH-tunnel leg itself
remains blocked on the escalation above; this finding does not resolve it,
it documents an available fallback if SSH access continues to be
unreachable in-session.
