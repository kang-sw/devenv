---
title: "ws-mcp: the launcher becomes a thin proxy to an always-on daemon that hosts the MCP servers"
related:
  261110-bug-ws-dashboard-sse-connection-budget: "landed the WebSocket upgrade path and Origin check on the daemon's HTTP port that this ticket's upgrade route sits beside"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 770913c17719f476
sage-review-completeness-reviewed: 770913c17719f476
completed: 2026-11-10
---

# ws-mcp: daemon-hosted MCP behind a thin proxy launcher

## Background

A company endpoint protector (AhnLab V3) blocks or prompts on every new
`ws-mcp` executable it sees run, and the user cannot get an exception on the
company machine. Today every MCP connection runs the plugin-cache executable
directly (`python3 <plugin>/bin/ws-mcp-launcher.py serve --stdio` ->
`os.execvpe` / `subprocess.call` of `<plugin>/.runtime/<platform>/ws-mcp[.exe]`),
the launcher's compatibility check runs that executable again
(`runtime capabilities`), and the Windows dashboard daemon runs yet another
copy staged under the cache root (`dashboard/bin/ws-mcp-dashboard-<sha16>.exe`,
`internal/wsdashboard/spawn.go` `StageExecutable`). Each plugin update and each
subagent spawn re-triggers the protector. The user must deploy to that machine
next.

The user's design ((638) c): the launcher finds the daemon's port, connects to
it if present and starts it otherwise, and forwards its input to the daemon
unchanged, so the daemon is started at most once and an executable runs once
per version. The daemon always runs, whether or not the dashboard is used.

The MCP server state is per process today (`internal/mcp/server.go`: root,
session harness, session bindings, mailbox, config manifests read from the
environment, live hub, dashboard link, tracker), and the server reads its
configuration from its own environment (`WS_MCP_ALLOWED_TOOLS`,
`WS_RSRC_ROOT`, `WS_SKILLS_ROOT`, `WS_MCP_PROJECT_ROOT`, `WS_CONFIG_HOME`,
`WS_CACHE_HOME`, `WS_WORKTREE_DIR`, `WS_TARGET_BRANCH`, `WS_BASE`,
`WS_MCP_DEBUG_LOG`, `WS_MCP_CONFIG_MANIFESTS`, `WS_MCP_CLI_NAME`, and others).
A daemon inherits only the environment of the process that first started it,
so these values must travel with each connection.

## Decisions

1. **The launcher is a thin proxy; the daemon always runs and hosts the MCP
   servers (user, (638) c, (640)).** `serve --stdio` through the launcher
   locates the daemon through its endpoint file (`internal/wsdashboard/endpoint.go`:
   port, pid, version, token), starts it when absent, opens one connection,
   and copies bytes in both directions between its own stdin/stdout and that
   connection until either side closes. One daemon per cache root, as today
   (`wsdashboard.Ensure`).
   Rejected: keeping a direct `serve` process per connection (the protector
   prompt this ticket exists to remove).

2. **Transport: a token-authenticated HTTP Upgrade on the daemon's existing
   port, then a raw byte stream (technical, (641)).** The user asked whether
   this is WebSocket; the lead's reading, recorded as assumed: the same
   Upgrade handshake WebSocket uses, but no WebSocket framing after the 101 —
   newline-delimited JSON-RPC exactly as on stdio. The token is the endpoint
   file's `token`. The route accepts loopback peers only, also in LAN mode,
   because it exposes every MCP tool (git commit, merge). Server-initiated
   messages (notifications, progress, server-to-client requests) travel on
   the same stream, so nothing is mapped per call.
   Rejected: a separate TCP port (a second port and auth scheme); per-call
   REST (server-initiated messages and long-blocking calls such as mailbox
   waits would each need their own mapping); WebSocket framing (Python's
   standard library has no WebSocket client, so the launcher would carry
   framing and masking code on every MCP byte).

3. **A hello line carries the connection's process context; one MCP server
   per connection; closing the connection ends its session (technical,
   (642)).** The launcher's first line on the stream carries the working
   directory and detected project root, the `WS_*` environment the direct
   `serve` would have read (including `WS_MCP_ALLOWED_TOOLS`,
   `WS_RSRC_ROOT`/`WS_SKILLS_ROOT`, `WS_MCP_CONFIG_MANIFESTS`, the package's
   `WS_MCP_CLI_NAME`), and the harness kind. The daemon builds a fresh MCP
   server for that connection from those values, never from the daemon's own
   environment, so ws and wsflow packages and worker tool restrictions never
   mix. Connection close replaces today's parent-death watch
   (`startParentDeathWatch`) and the dashboard session end (`DashboardEnd`).
   Rejected: one shared MCP server for all connections (per-package manifests
   and worker permissions would leak across sessions).

   What "never from the daemon's own environment" covers (from design
   review, auto-confirmed):
   - **The full client environment travels.** The hello carries the
     launcher's whole process environment, not only `WS_*`: hosts may start
     the launcher with a sanitized environment, and tools need the client's
     `PATH`/`PATHEXT`, `HOME`, SSH and credential variables.
   - **Subprocesses use the connection's environment.** Every process a
     connection's tools start builds its environment from that
     connection's, never from the daemon's `os.Environ()`: git
     (`internal/wsgit/git.go`, `wsgit/executable.go` lookup,
     `wsindex/git.go`), worktree hooks (`mcp/worktree_hooks.go`), exec jobs
     (`execjob`), agent backends (`wsagent/backend.go`), the tracker runner
     (`wstrackerhost/runner.go`), and the SSH command (`gitbatch`).
   - **Per-connection owner identity.** The hello carries the launcher's PID
     as the connection's owner identity; mailbox presence ownership and
     liveness (`internal/mcp/mailbox_runtime.go`) and the `mailbox wait`
     listening-marker owner check (`wsmailbox/listening.go`) key on it in
     place of `os.Getpid()`, so two sessions claiming one mailbox name still
     conflict, a disconnected session's record goes stale, and an old wait's
     cleanup never deletes a replacement wait's marker.
   - **Process-global state becomes per connection.** The debug event ring
     and the `WS_MCP_DEBUG_LOG` path (`internal/mcp/server.go`) are per
     connection. Resource and skills roots always come from the hello (the
     launcher always sets them); the daemon never falls back to
     `os.Executable()/../rsrc`. Startup diagnostics a direct `serve` prints
     to stderr (for example a rejected config manifest) go to the daemon log
     and the connection's debug events.

4. **One executable path per version on the normal path (technical,
   (643)).** The launcher stages the plugin's runtime binary into the
   daemon's `bin/` directory under its content-hash name (the same name
   `StageExecutable` produces) and starts the daemon from that path;
   compatibility is read from the daemon's health response instead of
   running `runtime capabilities` on the plugin copy. On the normal path no
   other ws-mcp executable runs. The staged name stays `StageExecutable`'s
   content-hash name on every OS; the version-in-name rule of the unpromoted
   idea `260513-feat-runtime-binary-staging-copy` is not adopted here (the
   content hash already separates versions and builds).

   Daemon start (from design review, auto-confirmed): the launcher runs the
   staged binary, with its own environment, as a daemon-start entry; Go
   resolves `dashboard.port`, `dashboard.lan_enabled`, and the build time
   from config exactly as `serve` does today (`cmd/ws-mcp/dashboard.go`) and
   runs the `Ensure` handoff (`keepRunning`), so the launcher never
   re-implements config resolution in Python. Compatibility read from health
   is the version range of Decision 7 only, never the runtime.json tool or
   command set match, so a newer same-minor daemon with extra tools is
   accepted. The launcher runs the daemon-start entry only when no daemon
   answers, or the running daemon is an older version, or the same version
   with an older build (from runtime.json, the staged binary's mtime, and
   health). A `dashboard.lan_enabled` change, which today hands a
   same-version daemon off at the next server start, is applied by the
   daemon itself: it re-reads the global `dashboard.lan_enabled` when a
   connection arrives and, on a change, replaces itself through the
   Decision 7 drain path. The `dashboard.*` manifest descriptions in all three packages
   that say the MCP server starts or replaces the daemon at startup are
   reworded to match. Rejected: keeping the launcher's
   compatibility run of the plugin copy (a second executable path per
   version; it is not known whether the protector keys on path or hash, so
   fewer paths is the safe side).

5. **Fallback to direct `serve --stdio` (technical, (644)).** When the
   daemon cannot be started or reached within a few seconds, or the version
   rule below says so, the launcher runs today's direct path and writes one
   line to stderr saying why. Two budgets (from design review,
   auto-confirmed): connecting to a running daemon that does not answer,
   with no start or replacement in progress, falls back after 5 s (10 s on
   Windows, the launcher's existing allowance for endpoint-protector
   delays); a daemon start, a replacement, or a reconnect waits at least
   `DefaultStartWait` plus 5 s (Windows plus 10 s), so a draining
   predecessor never pushes the launcher onto the direct path. Rejected: failing the MCP connection (no MCP at
   all is worse than one protector prompt).

6. **The daemon always hosts MCP; `dashboard.enabled` stays a per-session
   switch (technical, (645); user, (650) a).** `dashboard.enabled` decides
   only whether the dashboard feature is on; one daemon, never two. The
   daemon's HTTP server always runs (the upgrade route needs it). Each
   connection's MCP server resolves `dashboard.enabled` per session from its
   own declared manifests and the project and global values, as
   `dashboardSessionEnabled` (`internal/mcp/dashboard_sessions.go`) does
   today: only an enabled session registers dashboard pages and opens the
   browser; a disabled session gets no page and opens nothing. A settings
   difference between connections never replaces the daemon. Port and LAN
   settings stay daemon-wide and keep today's `Ensure` rules.
   This supersedes the serve-startup spawn gating of (285) (7a70197d, ws on
   and wsflow off): wsflow, whose dashboard default stays off, now also
   starts the daemon, as an MCP host whose sessions register no pages.
   Rejected: no daemon when the dashboard is off (the protector prompt
   returns for those users); one global value for the whole daemon, and the
   starting launcher's value deciding (both make pages depend on which
   package started the daemon, ws defaulting on and wsflow off).

7. **Version mismatch (user, (646) a).** Within the same minor (0.48.x) the
   newest daemon wins, as `Ensure`'s `keepRunning` rule does today: a
   launcher whose package is newer starts its own daemon, which replaces the
   older one; an older launcher uses the newer daemon. When the daemon is
   replaced under a live connection, the proxy reconnects transparently: it
   re-sends the client's captured `initialize` request and
   `notifications/initialized`, drops the replayed `initialize` response, and
   answers each request still in flight with a JSON-RPC error, so the harness
   never sees the MCP server disconnect. Across minors the launcher falls
   back to direct `serve` (Decision 5). The Pi bridge's exact version pin
   (`agents-plugin-pi` `version-check.ts`) relaxes to "same minor, equal or
   newer daemon".
   Replacement drains (from design review, auto-confirmed): a daemon asked
   to shut down closes its listener at once (freeing the port for its
   successor), accepts no new connection, lets in-flight calls finish for
   strictly less than the successor's lock wait (`DefaultLockWait` minus
   1 s), cancels long waits
   (mailbox waits and other blocking waits) at once, then closes its
   connections; a git commit or merge in flight is not cut midway. A daemon
   that dies with no successor: the proxy re-runs the start-and-connect
   path and replays as for a replacement; when that fails within the
   Decision 5 timeout, it starts direct `serve` and replays into it.
   Rejected: one daemon per version (split endpoints, ports, and dashboards);
   newest always wins across minors with no fallback (no compatibility
   promise across minors).

8. **`mailbox wait` also goes through the daemon (technical, (649)).** The
   Claude mod's mailbox and dashboard-message push spawns
   `python3 <launcher> mailbox wait ...` on every wait cycle
   (`agents-plugin/hooks/claude-mod/register.ts` `waitArgv`), and the Pi
   mailbox waiter spawns the runtime too; that is the recurring executable
   run besides MCP. The hello line carries a CLI mode with the argv; the
   daemon runs the wait and returns its stdout (the JSON result) and exit
   code on the stream. Other one-shot CLI commands (`ws-mcp call`,
   `version`, ...) keep running the executable directly; they are rare and
   user-invoked. A missing or unreachable daemon falls back to the direct run
   (Decision 5).
   The `mailbox wait` CLI, which exits through `os.Exit` and is cancelled by
   signals and the parent-death watch (`cmd/ws-mcp/mailbox.go`), becomes a
   function that returns its stdout and exit code, is cancelled when its
   connection closes, and clears its listening marker on that path. The Pi
   mailbox waiter keeps spawning `mailbox wait` through the launcher, so it
   takes this path unchanged.
   Rejected: leaving `mailbox wait` on the direct executable (one executable
   run per wait cycle remains).

9. **Release scope (user, (647) a, (648) b).** This ships in the next
   release together with the already-merged lock-scope and WebSocket fixes;
   the user postponed the 0.48.16 publish until this ticket lands. If
   verification cannot finish, the release ships without it (the user
   decides at the release gate).

10. **Hello wire format (from review, auto-confirmed).** One JSON object
   line carrying at least: a protocol version, `mode` (`mcp` or `cli`),
   `argv` (CLI mode), `cwd`, `project_root`, `env` (the full map),
   `harness` (derived the way the direct `serve` derives it today), and
   `owner_pid`. The daemon answers one JSON line (accepted, or refused with
   a reason that sends the launcher to the direct fallback) before the raw
   stream starts. The proxy's replayed `initialize` uses an id of its own,
   never the client's original, so a late reply from the old daemon cannot
   be mistaken for it. The worker pins the exact shape and the CLI-mode result
   framing, documents them in `ai-docs/manuals/ws-mcp.md`, and records them
   in the Result; all three hosts depend on it.

## Constraints

- The three launcher copies (`agents-plugin/bin/`, `agents-plugin-wsflow/bin/`,
  `agents-plugin-pi/bin/ws-mcp-launcher.py`) stay byte-identical, as today.
- The launcher uses the Python standard library only.
- The launcher must compute the same cache root and daemon state directory
  as Go's `wsdashboard.Dir` and the same staged name as `StageExecutable`;
  a test pins the Python and Go results against each other.
- Direct `serve --stdio` (fallback, tests, the dashboard support-agent runner
  in `cmd/ws-mcp/dashboard.go` that spawns `serve --stdio`) keeps working
  unchanged.
- Codex, Claude, and Pi hosts all start MCP through the launcher; none of
  their manifests change shape.
- Never run go build/test/vet locally; use
  `~/.local/bin/ws-rgo -C <checkout>/agents-plugin-tool -m 30 -- <go args>`.
  Only `nice -n 19 node --check` and `nice -n 19 gofmt -l` run locally.
- Never touch the user's running dashboard daemons (ports 29417 / 45743);
  tests use their own cache roots and ports. With the daemon always on, no
  test may start a daemon in the developer's cache root: launcher, Go, and
  Pi integration tests point `WS_CACHE_HOME` (or the equivalent) at a
  temporary directory (prior decision 14816e39 kept Pi integration tests
  from spawning a real daemon by leaving out the dashboard manifest; that
  guard no longer suffices).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (all three packages
  and the tool tree), `ai-docs/manuals/ws-mcp.md` (`internal/mcp/`, launcher
  environment; update its launcher section to the new path),
  `ai-docs/manuals/wsflow-mirroring.md` (launcher copies).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 7a70197d (2026-11-06, commit): "(285): spawn at serve startup gated by a config key, ws on and wsflow off; rejected first-board-update-only and explicit-command-only." — bearing: contradiction-candidate
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, Decisions): "Daemon entry point. The daemon is a new `ws-mcp dashboard` subcommand. Rejected: a non-stdio flag on `ws-mcp serve`, ... a separate subcommand keeps that path untouched." — bearing: constrains
- 14816e39 (2026-11-06, commit): "Pi integration tests that copy package files into a temp plugin dir do NOT get dashboard-config-manifest.json: with dashboard.enabled on by default, their ws-mcp would spawn a real detached daemon in the developer's cache root" — bearing: constrains
- fb298f63 (2026-11-07, commit): "Order: version first, build time only breaks a tie; an older or equal build never replaces. Rejected 'different build id -> replace': the Claude and Pi runtimes run different ws-mcp binaries of one version concurrently" — bearing: constrains
- 4f447bc6 (2026-11-07, commit): "Ensure: an older ws-mcp never replaces a newer daemon, whatever its mode (the existing downgrade rule); only a same-version daemon in the other mode is handed off." — bearing: supports
- ba911edb (2026-11-09, commit): "startDetached (spawn_unix/spawn_windows) sets no cmd.Env, so a serve-started daemon inherits serve's environment" — bearing: supports
- 260513-feat-runtime-binary-staging-copy (2026-05-13, Decisions): "The staged executable name must include the runtime version deterministically, such as `ws-mcp-v0.26.1.exe` or the platform-appropriate equivalent." — bearing: contradiction-candidate
- fb8e156d (2026-05-23, commit): "Used plugin version plus runtime.json content hash for the cache-local binary name so explicit shared runtime directories do not make different runtime contracts fight over the same executable path." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/bin/ws-mcp-launcher.py and its two copies, agents-plugin-tool/internal/wsdashboard/ensure.go spawn.go endpoint.go plus a new upgrade route, agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/cmd/ws-mcp/, agents-plugin/hooks/claude-mod/register.ts, agents-plugin-pi/src/version-check.ts mailbox-waiter.ts |
| scope.surface | cross-module | launcher CLI behavior, daemon HTTP route, MCP server construction, Pi bridge version rule, Claude mod wait path |
| scope.new_public_symbol | yes | a daemon upgrade route and hello-line handler; exact Go names not yet chosen |
| scope.new_type_contract | yes | hello-line wire format carrying cwd, project root, WS_* env, harness kind, and CLI mode argv; mailbox-wait result framing on the stream |
| scope.test_surface | existing | agents-plugin/tests/test_ws_mcp_launcher_capabilities.py, agents-plugin/tests/test_ws_mcp_launcher_coldload.py, agents-plugin-tool/internal/wsdashboard/daemon_test.go, agents-plugin-pi/test/version-check.test.ts; new test files likely for the upgrade route and e2e smoke |
| complexity.reuse_points | confirmed | wsdashboard.Ensure and keepRunning ensure.go, StageExecutable spawn.go, endpoint file endpoint.go, wsUpgrade stream_ws.go, mcp.Server DashboardEnd dashboard_sessions.go |
| complexity.side_effect_risk | high | every MCP connection on every host routes through a long-lived daemon; process lifecycle, staging, and replacement under live connections |
| risk.correctness | high | byte proxy, reconnect with initialize replay and in-flight error answers, per-connection env isolation replacing process-global env reads |
| risk.fit | high | server.go and many packages read WS_* from os env today; per-connection construction cuts across internal/mcp, wsrsrc, wsstate, worktree hooks |
| risk.test | high | Python and Go cross-language pinning, real-daemon e2e on a scratch cache root, Windows paths not exercised remotely |
| risk.security_or_contract | high | new route exposes every MCP tool including git commit and merge; token, loopback-only also in LAN mode, Origin refusal; Pi version-pin contract relaxes |

## Phases

### Phase 1: daemon side

The Go side of Decisions 2, 3, 4 (daemon-start entry from the staged path),
6, 7 (replacement drain), 8 (`mailbox wait` as a returning function and its
CLI mode on the stream), and 10. Direct `serve --stdio` keeps working.

Verification (Go tests, remote):
- The upgrade route refuses a missing or wrong token, a non-loopback peer
  (also in LAN mode), and a browser Origin; a refused hello gets its reason
  line.
- Two connections with different `WS_MCP_ALLOWED_TOOLS`, config manifests,
  project roots, `WS_CONFIG_HOME`, and `WS_WORKTREE_DIR` each get their own
  tool list and manifests and each act on their own root (a git or ticket
  tool call per connection).
- A subprocess started by a connection's tool sees that connection's
  environment (for example `PATH`/`HOME`), not the daemon's.
- Two connections claiming one mailbox name conflict; a closed
  connection's presence goes stale; an old `mailbox wait`'s cleanup does not
  delete a replacement wait's marker on the same session key.
- A connection close ends its dashboard session.
- `mailbox wait` through the stream returns the same JSON and exit code as
  the direct CLI, and a closed connection cancels it and clears its marker.
- Replacement during an active tool call: the call finishes within the drain
  bound and its result is delivered; a blocked mailbox wait is cancelled at
  once; a call running close to the drain bound still lets the successor
  take the lock and serve (no fallback to direct `serve`).
- A global `dashboard.lan_enabled` change seen on a new connection replaces
  the daemon through the drain path.
- `dashboard.enabled` follows the rule of Decision 6.

### Result (b9f4fb8) - 2026-11-10

Commits `61e32d7` (spine: upgrade route, hello, per-connection `mcp.Server`
seam, `daemon-start`, drain), `7c3a8c5` (every process-global read in
`internal/mcp` routed through the seam; `wsgit.LookPathEnv` so a hosted
server's git is the caller's PATH's), `b9f4fb8` (exec jobs and the tracker
host's git under the connection environment; hosted-session tests through
the real daemon).

Hello wire format (Decision 10), pinned in `internal/wsdashboard/proxy.go`
and documented under `## Daemon-Hosted MCP` in `ai-docs/manuals/ws-mcp.md`:
`GET /mcp HTTP/1.1` with `Upgrade: ws-mcp`, `Connection: Upgrade`,
`Authorization: Bearer <endpoint token>`, from a loopback peer with no
`Origin`; `101 Switching Protocols` (401 token, 403 peer/Origin, 426 no
upgrade headers, 503 no proxy handler), then a raw newline-delimited stream.
First line from the launcher:
`{"protocol": 1, "mode": "mcp"|"cli", "argv": [...], "cwd", "project_root",
"env": {whole environment}, "harness": "" (optional hint), "owner_pid",
"version"}`. Daemon answer: `{"ok": true, "version", "pid"}` or
`{"ok": false, "reason"}` then close. `mcp` mode: the stream is the MCP
session of a server built from the hello (`NewServerWithOptions`). `cli`
mode (argv `mailbox wait ...` only): frames `{"stream": "stderr", "data"}`
as written, then `{"final": true, "exit_code", "data": <stdout>}`; the
connection's end cancels the wait (`WaitOptions.Done`, exit 130) and clears
its pid-owned listening marker.

Decisions taken: the hello `harness` is a hint only, the server still
detects the harness from `initialize`; `owner_pid` replaces `os.Getpid()`
for mailbox presence and listening markers (a replacement wait by another
launcher keeps its marker: `ClearListeningMarker` is pid-gated);
`WS_MCP_NO_DASHBOARD` now means no registration/prune only, the daemon is
the launcher's business; `DashboardSessionSource` readers, the crash sinks
(`crashDir`, package `recordPanic`, `RecordLifecycleEvent`), `LeadToolNames`
(the CLI's capabilities list) and the wsagent runner (a daemon component)
stay process-level; the wsmailbox machine store path and wsconfig/wsstate
fall back to the daemon environment when the hello carries no
`WS_CONFIG_HOME`/`WS_CACHE_HOME`, which the launcher's daemon dir resolution
makes the same root.

Verification: `ws-rgo test ./... -count=1` ok, `vet ./...` clean, `gofmt -l`
only the pre-existing `internal/wstracker` hits. Covered by
`internal/wsdashboard/proxy_test.go` (route refusals, hello refusal reasons,
drain in-flight/long-wait/budget), `internal/mcp/process_env_test.go` (two
hosted servers isolated by env, owner pid in presence, subprocess on the hello
PATH), `cmd/ws-mcp/daemon_proxy_test.go` (session end marks the registration
ended and leaves a stale presence; same-name claim conflicts; dashboard-off
session registers nothing and replaces nothing; hosted wait cancelled by its
connection, replacement marker kept, timed-out wait answers in frames with
exit 3; global `lan_enabled` change replaces the daemon through drain and
the drained streams end). `WS_WORKTREE_DIR` isolation is covered by the
cache-home isolation of the session-key store, not by a dedicated assertion.

### Phase 2: proxy launcher, Pi, and acceptance

The launcher side of Decisions 1, 4, 5, 7 (proxy, staged start,
compatibility, reconnect and replay, crash path), 8 (CLI-mode proxy), the
Pi version rule, the manifest description rewording, and the `ws-mcp.md`
launcher section.

Verification:
- Python launcher tests (the existing `agents-plugin/tests/test_ws_mcp_launcher_*.py`
  location): daemon present -> proxy; absent -> staged start then proxy;
  unreachable within the timeout -> direct fallback with a stderr line;
  cross-minor -> direct fallback; a newer same-minor daemon with extra tools
  is accepted; daemon replaced mid-session -> reconnect replays
  `initialize` and in-flight requests get errors; daemon killed with no
  successor -> restart and replay, or direct fallback with replay;
  `mailbox wait` through the daemon and its direct fallback; Python and Go
  agree on the cache dir and staged name.
- An end-to-end smoke through the real launcher and a real daemon on a
  scratch cache root: `initialize`, `tools/list`, one tool call, a server
  notification, then close.
- Pi tests for the relaxed version check.
- Windows: the `ws-mcp-ci` Windows go test shards pass (worker-owned;
  recorded in the Result). Acceptance on the
  user's Windows machine (with V3) is deferred to the user at the release
  gate: the daemon runs from the staged `dashboard/bin/` executable, a
  session spawns no plugin-copy executable, and the protector prompts at
  most once per version.

### Result (09b041e) - 2026-11-10

Commit `09b041e`. The launcher's `DaemonLink`/`McpProxy` block (stdlib only,
three byte-identical copies) does Decisions 1, 4, 5, 7, 8; Pi
`serverVersionAcceptable` (same minor, equal or newer); manifest
descriptions reworded in all three packages (wsflow still differs only in
`dashboard.enabled`'s default); `ws-mcp.md` gained `## Daemon-Hosted MCP`
and the `WS_MCP_LAUNCHER_DIRECT` row.

Decisions taken: the launcher's runtime version for the daemon comparison
is `runtime.json` `plugin_version` (no second executable run); build time is
the runtime binary's mtime in ms; a reconnect gives a successor 1.5 s before
running `daemon-start`; `WS_MCP_LAUNCHER_DIRECT=1` opts out of the proxy;
the replayed `initialize` carries a launcher-owned id
(`ws-mcp-launcher-replay-<n>`) and its answer is dropped; parent-death watch
on Windows only (`_winapi`), as in Go.

Verification: `agents-plugin/tests` 99 tests ok (18 new in
`test_ws_mcp_launcher_proxy.py` over a fake daemon: present, staged start,
unanswering, cross-minor, refused, newer same-minor with extra tools,
replacement reconnect with replay and in-flight errors, died with and
without a successor, mailbox wait frames and fallback, cache root/daemon
dir/staged name parity with Go); Pi 9/9; Go end-to-end
`cmd/ws-mcp/launcher_proxy_test.go`: the real launcher stages the binary,
starts the daemon from the staged copy (the only entry in `dashboard/bin/`),
serves `initialize`, `tools/list`, `runtime.read`, a second session reuses
the daemon, and `mailbox wait` through the launcher answers like the direct
CLI (same JSON but the re-arm command's program name, same exit 3). The
server sends no server-initiated notification of its own, so notification
pass-through is covered by the fake-daemon test (`notify-me`), not the
smoke. Windows `ws-mcp-ci` shards: not run; the workflow runs on
`pull_request`/`workflow_dispatch` only and the worker never pushes, so the
lead dispatches it after the push (deferred). Acceptance on the user's
Windows machine stays with the user at the release gate.

Lite review (one pass, one reviewer): 1 Critical and 4 Important fixed in
the follow-up commit: relative path values in the hello environment
(Codex's `./config-manifest.json`) are made absolute against the launcher's
cwd (`hello_env`; the Go smoke now passes a relative manifest and asserts
the lead session registers); a client that leaves during a reconnect ends
the successor stream instead of hanging the launcher (`replay_into`);
`Server.Close` frees the nudge goroutine and tracker host of an ended hosted
session; `canonicalGitRoot` runs through the connection's git runner; the
compatibility probe runs the staged copy on the daemon path, so the
plugin-directory binary never executes (Decision 4). Minor, recorded only:
concurrent cold starts can race in `stage_daemon_binary` (the loser takes
the direct path); a daemon that dies after a CLI-mode wait has started
returns exit 1 rather than falling back to the direct CLI; long-wait
cancellation on drain is pinned against the echo stub, not `mcp.Server`
(no MCP tool blocks long today); `WS_WORKTREE_DIR` isolation has no
dedicated assertion. The tracker runner's `core.sshCommand` probe now runs
under the host environment and compares Windows keys case-insensitively.
