# Plan: 260707-chore-dashboard-linked-server-tunnel-dogfood-plan — Phase 1: Confirm SSH connectivity, then dogfood both linked-server paths

## Relevant Ticket Contract

- Step 1: confirm non-interactive SSH connectivity WSL -> Windows host first
  (`ssh -o BatchMode=yes -o ConnectTimeout=5 <user>@<windows-host> true`,
  re-derive the WSL2 gateway IP via `ip route show default` — do not reuse a
  previously recorded IP). Diagnose (not provision) if broken.
- Step 2 (if SSH confirmed): dogfood the SSH-tunnel linked-server path —
  remote Windows daemon bound `127.0.0.1`, default `local` bind mode, normal
  owner-auth; drive the tunnel connect/reconnect API via `curl` from the WSL
  gateway daemon (normal owner-auth); verify resources, root-picker/open
  workRoot, file read/write, Git status/branches, terminal create, terminal
  WebSocket relay round-trip.
- Step 3 (regardless of SSH result): dogfood the reversed-topology
  direct-endpoint variant — Windows-hosted gateway (normal owner-auth) linking
  to a WSL-hosted remote (`127.0.0.1`, `local` bind mode, normal owner-auth)
  via `http://localhost:<port>`; confirm the same forwarded-operation set
  succeeds with no Host-check failure.
- Step 4: record outcome as a dated append to
  `260525-feat-ws-dashboard-server-scoped-operation-forwarding` (do not edit
  its frozen `### Result` text).
- Step 5: if step 2/3 surfaces a genuine bug (not another Host/Origin
  misdiagnosis), file it as its own ticket — do not fix code in this ticket.
- Step 6: tear down all test artifacts (daemons, SSH tunnel process, and
  any linked-server entries persisted solely for this run).
- Constraint: do not weaken `entrypoint_headers_allowed`/`is_allowed_host` or
  any Host/Origin/bind-mode invariant — already settled non-bug (see dropped
  ticket `260707-bug-dashboard-public-bind-host-check-rejects-own-address`).
- Constraint: never echo/persist pairing tokens, link passphrases, bearer
  tokens, or session cookies to a file or transcript — capture into a shell
  variable and consume in the same command chain, per
  `ai-docs/_index.local.md`'s documented pattern.
- Constraint: the SSH-tunnel path has no UI — drive it via `curl` against
  `servers.rs`'s HTTP API directly.

## Out of Scope

- Any code fix for the settled non-bug (public-bind Host-check). Do not touch
  `auth.rs`'s `entrypoint_headers_allowed`/`is_allowed_host`.
- Filing/implementing a UI gap ticket for the missing `ssh_target` field in
  `LinkedServerModal` (`App.tsx`) — out of scope per ticket Constraints.
- Phases beyond Phase 1 (none exist yet in this ticket).
- Playwright/e2e coverage — this phase is manual/API-level dogfooding only.

## Codebase Findings

- `ws-dashboard/crates/daemon/src/router.rs:89-92` — `POST /api/dashboard/servers/ssh/start`, handler `start_ssh_dashboard_server` (`servers.rs:328-419`). Request body (`SshServerTunnelRequest`, `servers.rs:1920-1932`, camelCase): `serverId`, `label`, `sshTarget`, optional `remoteEndpoint`, optional `startupCommand` (used when `remoteEndpoint` omitted), optional `localPort`. Validation in `linked_server_from_tunnel_request` (`servers.rs:2055-2082`) requires either `remoteEndpoint` or `startupCommand`.
- `ws-dashboard/crates/daemon/src/router.rs:102-105` — `POST /api/dashboard/servers/{server_route}/tunnel/reconnect`, handler `reconnect_dashboard_server_tunnel` (`servers.rs:421+`) — use this to re-establish a dropped tunnel without re-supplying `sshTarget`.
- `ws-dashboard/crates/daemon/src/servers.rs:2144-2172` — `start_system_ssh_tunnel`: spawns `ssh -N -L 127.0.0.1:<local>:127.0.0.1:<remote> <ssh_target>` (env override `WS_DASHBOARD_SSH_BIN`). This is the actual tunnel process to track/kill for teardown.
- `ws-dashboard/crates/daemon/src/router.rs:93` — `POST /api/dashboard/servers/link` (direct-endpoint add), handler `link_endpoint_server` (`servers.rs:280-326`). Body (`EndpointLinkedServerRequest`, `servers.rs:1934-1942`): `serverId`, `label`, `endpoint`, optional `passphrase`. Endpoint validated in `normalize_dashboard_endpoint` (`servers.rs:2104-2121`, must be `http`/`https`, no query/fragment) — use `http://localhost:<port>` per ticket's reversed-topology step.
- `ws-dashboard/crates/daemon/src/router.rs:98-101` — `POST /api/dashboard/servers/{server_route}/link-auth`, handler `link_dashboard_server` (`servers.rs:232-278`), body `{ "passphrase": "..." }` — needed if the initial link/tunnel-start call didn't already auto-authenticate.
- `ws-dashboard/crates/daemon/src/cli.rs:23-49` — `ServeArgs`: `--host` (default `127.0.0.1`), `--bind-mode {local|tunnel|public}` (default `local`, enum at `cli.rs:51-60`), `--no-auth`, `--port`. Both remote (Windows) and WSL daemons in this phase use default `local` bind mode + normal owner-auth (no `--bind-mode public`, no `--no-auth`) per the ticket's explicit instruction.
- `ws-dashboard/crates/daemon/src/server.rs:65-72` (per survey) — startup stdout prints the owner pairing URL (`/pair?token=<pairing_token>`) and, for a linked-server-capable daemon, the remote link passphrase. **No bearer token is ever printed for primary owner-auth** — a curl-driven session must first `GET /pair?token=<token>` once to receive `Set-Cookie: ws-dashboard-owner=...`, then send that cookie on every subsequent request (or use `--no-auth` for a loopback-only leg, which the ticket does not authorize here since it explicitly wants "normal owner-auth").
- `ws-dashboard/crates/daemon/src/auth.rs:274-277` (`entrypoint_headers_allowed`) and `auth.rs:308-335` (`is_allowed_host`) — passes only `localhost` or a loopback IP literal (with or without port) on `Host` and, if present, `Origin`. Confirms: (a) do not send a custom `Origin` header from curl scripts unless it's loopback-based; (b) the reversed-topology direct-endpoint leg (`http://localhost:<port>`) is exactly the case this check is designed to allow, since the WSL remote is bound to `127.0.0.1` and reached via `localhost` — the ticket's stated rationale for expecting it to work.
- `ws-dashboard/crates/daemon/src/router.rs:94-215` — forwarded-operation routes under `/api/dashboard/servers/{server_route}/...`: `resources` (94-97), `root-picker` + `root-picker/directories` + `root-picker/pins` (106-117), `work-roots/open` (118-121), `work-roots/{id}/files` + `/files/read` + `/files/write` (126-137), `work-roots/{id}/git/status` + `/git/branches` (170-177), `work-roots/{id}/terminals` (194-197), `terminals/{id}/socket` (202-205, WebSocket relay). This is the exact route set the ticket calls "the full forwarded-operation set" — use these paths for the curl verification pass on both legs.
- `ws-dashboard/crates/daemon/src/servers.rs:1346-1416` (`terminal_websocket_relay`) and `servers.rs:1291-1345` (`connect_remote_terminal_websocket`) — the WS relay goes through the same `require_owner_auth`/`entrypoint_headers_allowed` gate (special-cased for `Upgrade: websocket` in `router.rs:393-401`), so a bare `wscat`/`curl --upgrade` test without the owner session cookie or with a non-loopback Host will be rejected before upgrade — carry the captured cookie into the WS test too.
- `ws-dashboard/crates/daemon/src/persistent_state.rs:478-491` (`default_state_file`) — state file path resolution order: `$WS_DASHBOARD_STATE_FILE` > `$WS_DASHBOARD_STATE_HOME/opened-workroots.json` > `$XDG_STATE_HOME/ws-dashboard/opened-workroots.json` > `~/.local/state/ws-dashboard/opened-workroots.json`. Linked servers persist under JSON key `linked_servers` (`persistent_state.rs:165`, `248`), deduped by `id` (`servers.rs:1870-1877`, `persistent_state.rs:463`).
- **Risk signal — no removal API**: grepped all `DELETE` routes in `router.rs` (lines 116, 144, 216, 237, 242, 306); none target a linked server. There is no `DELETE`/disconnect route for `/api/dashboard/servers/{server_route}`. Teardown (ticket step 6, "remove any linked-server entries created solely for this dogfood run from each daemon's persisted state") requires either setting `$WS_DASHBOARD_STATE_FILE`/`$WS_DASHBOARD_STATE_HOME` to a scratch path for both daemons during this run (cleanest — just delete the scratch dir afterward, no risk to any pre-existing state), or manually editing the real state file's `linked_servers` JSON array afterward. **Recommend using a scratch state dir via env var for both daemons in this run** to make teardown trivial and avoid touching any pre-existing persisted state.
- `ws-dashboard/crates/daemon/src/cli.rs:14-15`, `88-138` — `--remote-guide` prints an embedded SSH-tunnel deployment walkthrough (recommends remote `--bind-mode tunnel --host 127.0.0.1 --port 0`); useful as a first read before scripting the tunnel-start curl call, and already anticipates this exact dogfood shape.
- `ai-docs/_index.local.md` (gitignored, machine-local) — already contains a validated methodology for the reversed direction of this exact cross-environment setup (WSL gateway/Windows remote), including: re-deriving the WSL2 gateway IP, building the Windows-native daemon via `powershell.exe` interop (`cargo build -p ws-dashboard-daemon`, binary lands at `target/debug/ws-dashboard.exe`), the credential-capture pattern (`RESP=$(curl ...); TOKEN=$(...); curl ... -H "Authorization: Bearer $TOKEN"; unset TOKEN RESP`), and the git-fetch workaround for the `/mnt/d/dbg-ws-dashboard-dev` checkout's UNC-path `origin` limitation. This session's topology is the *opposite* direction (SSH-tunnel: WSL gateway -> Windows remote; direct-endpoint: Windows gateway -> WSL remote) but the mechanics (PID capture via `Start-Process -PassThru`, interop process lifecycle) carry over directly.
- `ai-docs/tickets/.dropped/260707-bug-dashboard-public-bind-host-check-rejects-own-address.md` — confirms the public-bind Host-check behavior is a settled non-bug; this phase must not re-litigate or attempt to fix it. The only prior attempt used `--bind-mode public`, which this phase's Constraints explicitly avoid (both legs use default `local` bind mode).

## Implementation Plan

1. Re-derive the WSL2 gateway IP: `ip route show default`. Run
   `ssh -o BatchMode=yes -o ConnectTimeout=5 <user>@<windows-host> true` from
   WSL. If it fails, check (in order): Windows "OpenSSH Server" optional
   feature installed/running (via `powershell.exe` interop,
   e.g. `Get-Service sshd`), and whether the WSL public key is present in the
   Windows host's `authorized_keys` (typically
   `C:\Users\<user>\.ssh\authorized_keys` for a non-admin account, or
   `%ProgramData%\ssh\administrators_authorized_keys` for an admin account).
   Fix only if it's a configuration gap (e.g. missing key, wrong file
   permissions); do not provision a new SSH server.
2. Pick a scratch state directory for both daemons this run, e.g.
   `WS_DASHBOARD_STATE_HOME=/tmp/dogfood-ssh-tunnel-<host>` per host, to make
   step 6 teardown trivial (delete the dir) without touching real state.
3. **If SSH confirmed** — SSH-tunnel leg:
   - Launch remote (Windows) daemon: `ws-dashboard serve --host 127.0.0.1
     --bind-mode local --port 0` via `powershell.exe` interop (capture PID via
     `Start-Process -PassThru`, per `ai-docs/_index.local.md`'s documented
     pattern); capture the printed pairing URL/passphrase into shell
     variables without echoing.
   - Launch WSL gateway daemon: `ws-dashboard serve --host 127.0.0.1
     --bind-mode local --port 0`, same credential-capture discipline.
   - `GET /pair?token=<gateway pairing token>` on the gateway, capture
     `Set-Cookie` into `$COOKIE`.
   - `POST /api/dashboard/servers/ssh/start` with `Cookie: $COOKIE`, body
     `{serverId, label, sshTarget, startupCommand}` where `startupCommand`
     re-runs the remote daemon's serve command over SSH (per
     `linked_server_from_tunnel_request`, `servers.rs:2055-2082`) — or, if the
     remote daemon from the step above is already running and its pairing URL
     is known, pass `remoteEndpoint` instead of `startupCommand`.
   - Walk the forwarded-operation set (`router.rs:94-215`) via curl with
     `Cookie: $COOKIE`: `GET .../resources`, `GET .../root-picker`, `POST
     .../work-roots/open`, `GET`/`POST .../files`+`/files/read`+`/files/write`,
     `GET .../git/status`+`/git/branches`, `POST .../terminals` (create), then
     the WS relay at `GET .../terminals/{id}/socket` (send typed input, confirm
     output round-trips) — a `wscat`/`websocat` invocation with the same
     `Cookie` header, or the `Monitor` tool's `ws` source for a quick
     request/response check.
4. **Regardless of step 3's outcome** — reversed-topology direct-endpoint leg:
   - Launch WSL remote daemon: `ws-dashboard serve --host 127.0.0.1
     --bind-mode local --port 0`.
   - Launch Windows gateway daemon (via interop): same flags.
   - Pair with the Windows gateway, capture its owner cookie.
   - `POST /api/dashboard/servers/link` on the Windows gateway with
     `endpoint: "http://localhost:<wsl-remote-port>"` — note this only works if
     the WSL remote's port is reachable from Windows via `localhost`, which
     `ai-docs/_index.local.md`'s prior test already confirmed works for
     WSL2's default networking.
   - Repeat the same forwarded-operation-set walk as step 3.
5. Record findings in `ai-docs/tickets/ready/260525-feat-ws-dashboard-server-scoped-operation-forwarding.md` as a new dated append (mirroring the existing `#### Verification note - 2026-07-07` style at line 862) — state which leg(s) succeeded, whether the SSH connectivity precondition held, and whether either leg surfaced a genuine implementation bug distinct from the already-settled public-bind non-bug.
6. If a genuine bug surfaces, file it as a new ticket under `ai-docs/tickets/idea/` or `todo/` (per repo convention) — do not fix it in this ticket.
7. Teardown: kill both daemon PIDs on both hosts (`kill <pid>` on WSL, `Stop-Process -Force -Id <pid>` via `powershell.exe` interop on Windows), kill the SSH tunnel process (the `ssh -N -L ...` child spawned by `start_system_ssh_tunnel`, `servers.rs:2144-2172`), and remove the scratch state directories from step 2 (`rm -rf` on the WSL scratch dir; interop `Remove-Item` on the Windows scratch dir) — this satisfies "remove any linked-server entries created solely for this dogfood run" without touching real persisted state.

## Verification Plan

- Manual-only verification: this phase's own success criteria are the curl/WS
  responses observed during the dogfood walk (200s on each forwarded route,
  visible input/output round-trip on the terminal WebSocket) — there is no
  automated test suite this phase should add or run.
- Credential-handling check throughout: never let a pairing token, link
  passphrase, bearer/session cookie value appear in a file, this session's
  final report, or an echoed shell command — capture-and-consume in the same
  command chain per `ai-docs/_index.local.md`'s documented pattern.
- Before declaring the direct-endpoint reversed leg "succeeded," explicitly
  confirm no `403`/Host-check rejection occurred (re-check via a Host-header
  A/B curl if any request unexpectedly fails, to avoid re-misdiagnosing the
  already-settled non-bug).

## Escalations

- None.
