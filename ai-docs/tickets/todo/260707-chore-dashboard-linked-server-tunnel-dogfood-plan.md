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

## Escalations

- None yet.
