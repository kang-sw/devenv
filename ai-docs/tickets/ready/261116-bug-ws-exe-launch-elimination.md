---
title: Eliminate ws executable launches outside new-daemon start
related:
  261115-feat-werkschaft-electron-daemon-override: Electron new-daemon start is one of the two permitted launch points
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1bc4ce86eb9de2d4
sage-review-completeness-reviewed: 1bc4ce86eb9de2d4
---

# Eliminate ws executable launches outside new-daemon start

## Background

The owner's Windows PC runs AhnLab V3, which flags repeated executable
launches. ws was moved to a daemon to avoid this, but V3 still fires dozens
of times per daemon swap and periodically ("MCP launch") while only hosted
Claude / Pi sessions are in use.

Owner contract, 2026-11-16 09:48 (verbatim, binding):

> 이 exe 실행 지점 없애는 걸 최우선으로 삼죠. 오로지 런쳐에서 새 데몬 띄울 때+
> (이미 살아있는 데몬 버전 체크도 REST API로만 하고 실행 파일 실행으로 검증 하지 않음)
> electron이 새 데몬 띄울 때 외에는 exe 실행 아예 없어야 합니다.

Meaning: a ws executable is executed only when (a) the launcher must start a
NEW daemon because none is alive, or (b) the Electron app must start a NEW
daemon. A live daemon's version / compatibility is checked only through its
REST API, never by executing a binary. Everything else talks to the daemon
over the network. Priority: top.

### Live trace evidence (2026-11-16 09:48-09:53 KST)

Sampled `Win32_Process` every ~250 ms (process-start trace was access-denied);
CSV at `D:/Temp/exe-spawn-trace/spawns-compact.csv` (machine-local, deleted
after this ticket is promoted).

- Steady state, 3.1 min window: zero daemon-start, zero new
  `ws-mcp-dashboard-*.exe`, zero new launcher / holder processes, zero
  launches from Claude hooks or the Pi mailbox/wake code.
- The V3 hits at 09:49-09:53 were a hosted Pi session (model-initiated)
  running `agents-plugin-pi/.runtime/windows-amd64/ws-mcp-0.50.1-<hash>.exe
  call tickets.create_empty ...` from its bash tool, including python loops
  that launched the exe once per call. Chain: daemon -> holder python ->
  pi node -> bash -> python -> ws-mcp exe. Cause: `tickets.create_empty` is
  not in that Pi worker's tool profile, so the model fell back to the exe CLI.
- V3 `AnalysisRpt.exe` ran at 09:44:38 against a newly staged
  `dashboard\bin\ws-mcp-dashboard-<sha16>.exe` right after the 09:43:35 swap:
  every distinct build stages a new unique path, which V3 treats as an unknown
  file. `dashboard\bin` held 4 staged exes plus 2 orphan `.tmp` copies.
  Staging timestamps (09:07 vs 09:10) suggest different sessions with
  different builds flip-flop the daemon (unverified).
- Daemon swap 09:43-09:46: each restarted hosted session costs ~6-7 process
  launches (holder python + conhost, claude.exe / pi node + conhost, launcher
  python + conhost); ~9-10 sessions restarted, ~60+ launches. Whether the
  swap or the owner's resume triggered the restarts is unresolved.

### Code audit: launch sites (paths relative to the repo root)

Launcher `agents-plugin/bin/ws-mcp-launcher.py` (byte-identical copies in
`agents-plugin-pi/bin/` and `agents-plugin-wsflow/bin/`):

1. Live-daemon probes are already network-only: `/health` (`daemon_health`
   ~:1041, `probe` ~:1311-1330), socket hello (~:1177), sidecar metadata
   (`read_runtime_metadata`, `runtime_capabilities_compatible` ~:200,
   :730-755), `inspect_runtime` (~:1803-1820). `serve --stdio`,
   `mailbox wait` and CLI calls are socket proxies (`proxy_serve` ~:1746,
   `proxy_cli` ~:1764). Keep it that way.
2. `start_daemon` ~:1357-1405 runs `subprocess.run([staged, "daemon-start",
   ...])` (~:1386) when `health is None` or `needs_start` (`connect`
   ~:1424-1426). `health is None` means "did not answer", not "no daemon".
   Reconnect storm: the reconnect loop ~:1625-1628 calls
   `link.connect(hello, grace=1.5)`; once a swap leaves the daemon silent for
   more than 1.5 s, every connected proxy independently stages the binary
   (`stage_daemon_binary` ~:941) and execs `daemon-start`, which itself
   spawns the real daemon via `SpawnDetached`
   (`agents-plugin-tool/internal/wsdashboard/spawn_windows.go#L23-L34`): one
   lifecycle invocation per successful launcher start attempt, followed by
   zero to two detached daemon spawn calls inside `Ensure`, not invariably
   two exe launches per proxy (`agents-plugin-tool/internal/wsdashboard/ensure.go#L35-L38,L124-L159`).
3. Lifecycle mode ~:1922 `subprocess.call([staged, *argv])`: only on
   health None / `needs_start` / `--replace`. Permitted when it really starts
   a new daemon; same "None means not answering" issue as item 2.
4. `go build` at `agents-plugin/bin/ws-mcp-launcher.py#L622-L640`
   (`build_local_devenv_runtime`), `scripts/dev.py#L81-L96,L116-L142`,
   `agents-plugin-pi/src/bridge.ts#L614-L634` and
   `agents-plugin-pi/src/local-devenv.ts#L221-L240`: source-build entry points;
   scripts/dev.py normally selects remote w5090 but can fall back locally
   (`scripts/dev.py#L152-L158`). The claimed dozens of toolchain processes
   per rebuild is not established by these source sites.
5. `detect_project_root` conditionally calls `git_root`, which runs
   `git rev-parse`; an existing `WS_MCP_PROJECT_ROOT` skips detection and
   multiple candidate roots may be tried, not exactly one git process per
   launcher start (`agents-plugin/bin/ws-mcp-launcher.py#L780-L822`).
6. `stage_daemon_binary` already reuses a hash-matching
   `ws-mcp-dashboard-<sha16>.exe`; its copy/replace lacks failure cleanup
   (`agents-plugin/bin/ws-mcp-launcher.py#L941-L968`). Go pruning already
   removes superseded staged entries after a two-minute grace, but always
   retains `.tmp` files (`agents-plugin-tool/internal/wsdashboard/spawn.go#L76-L107`).

Daemon / tool (`agents-plugin-tool/`):

- `cmd/ws-mcp/proxy.go:194` LAN-mode self-respawn and
  `cmd/ws-mcp/dashboard.go#L371-L380` direct-serve stage+spawn: permitted
  (they start a new daemon).
- `internal/mcp/playbook_tools.go#L330-L354`: printed commands prefer
  `WS_MCP_CLI_NAME`, then `WS_MCP_RUNTIME_BINARY`, then `os.Args[0]`.
  Normal launcher startup already sets the first to the Python launcher
  (`agents-plugin/bin/ws-mcp-launcher.py#L1848-L1852`); the executable
  fallbacks and generic `ws-mcp mailbox wait` text remain audit targets
  (`agents-plugin-tool/internal/mcp/playbook_tools.go#L313-L321`).
- `internal/wsdashboard/plan_usage_probe.go#L19-L43` with
  `plan_usage_cmd_windows.go#L10-L19`: runs `codex app-server` on viewer
  queries when Codex login presence is detected, ordinarily at five-minute
  intervals but at five-second intervals for forced refresh, not at most
  once per five minutes (`agents-plugin-tool/internal/wsdashboard/plan_usage.go#L198-L221`).
- `internal/wsagent/pi.go#L57-L69` runs `pi --offline --no-approve --list-models`;
  support choices already cache this in memory for five minutes
  (`agents-plugin-tool/internal/wsagent/support.go#L48-L49,L278-L319`).
  `pi --version` is separately cached once per process, including failures
  (`agents-plugin-tool/internal/wsagent/pi.go#L1244-L1267`;
  `agents-plugin-tool/internal/mcp/pi_tools.go#L72-L94`).
- Per hosted-session children (inherent to hosting): holder python
  (`internal/wsholder/manager.go#L601-L648`, `spawn_windows.go#L23-L31`), `claude.exe`
  (`internal/wsagent/claude.go:341`), pi node (`internal/wsagent/pi.go:429,458`),
  Pi payload python (`pi_payload.go:43` via `pi_lead.go:131`), per-worker
  launcher `serve --stdio` (`cmd/ws-mcp/agent_mcp.go:12-50`), `taskkill`
  per stop (`internal/wsholder/kill_windows.go:33`).
- Periodic git (not ws exe): `internal/wsdashboard/gitbadge.go:51,147` every
  3 min per watched root.

Python-launcher spawns per session (outer commands use Python; cold CLI
startup can still reach a ws executable through `main`/`connect`,
`agents-plugin/bin/ws-mcp-launcher.py#L1861-L1911,L1422-L1428`):

- Claude: `.mcp.json` `python3 ./bin/ws-mcp-launcher.py serve --stdio` per
  session / subagent; `claude-mod/register.ts:525,531,588` (via `:1039`)
  `mailbox wait --stream` per session plus one-shot fallback;
  `hooks/hooks.json:10` one python per compaction.
- Pi: `agents-plugin-pi/src/mcp-stdio-client.ts:372` launcher per session /
  fork / subagent; `mailbox-waiter.ts:910` long-lived `--stream` per session
  (respawn on mode change / crash), `:1125` one-shot per wake fallback;
  `local-devenv.ts#L87-L109` inspects at lead/fork startup only when a valid
  local-devenv marker enables that path; it is not unconditional per lead start.

## Decisions

- Live-acceptance repair, coordinator assignment 2026-11-16 02:59:27Z:
  fix the proven Python dev lifecycle gaps before the Codex REST follow-up.
  `scripts/dev.py` must participate in the existing launcher starter gate
  through bounded expected-build REST confirmation; builds stay outside it.
  Reuse the imported `LauncherStartLock`, not Go's daemon singleton or a new
  gate/interface. For the specific exit-1, parsed `outcome: unavailable`
  response, use the remaining lifecycle deadline to confirm the expected
  content hash over REST, without another executable invocation. Keep the
  original diagnostic; reject malformed reports, unrelated failures, old
  hashes and deadline expiry. Technical choices follow the two source-triage
  findings; rejected: unconditional nonzero-exit success, a second starter,
  a global wait increase or an unbounded/resetting deadline.
- Acceptance evidence distinguishes process creation from readiness. The
  sampled extra starter is real, but the supplied CSV does not establish
  continuous health at its under-lock probe. Equal numeric cores already
  ignore hash/build time; do not change that policy based on process age.
  Coordinator-owned repeat sampling, not a worker runtime operation, must
  determine any remaining anomaly after repairing the dev gate bypass.
  Coordinator correction at 03:11:45Z: 02:54:29 was process creation plus
  the /health `started` field time, not independently observed health. The
  sampler first recorded healthy around 02:57:00; old retry waves stopped and
  dashboard reconnect logging appeared around 02:55:08. Reachability may have begun
  between 02:54:29 and 02:55:08, so an unpublished-endpoint window plausibly
  explains PID 3452 at 02:54:52, but its exact admission cause remains unproven.
  Do not pursue that historical anomaly or add diagnostics in this repair.
- Repair compatibility tier: T0, internal Python caller coordination and
  result handling only. Existing lock, endpoint and Go JSON outcome shapes
  remain unchanged. Old launchers ignore the persistent gate; new launchers
  reopen the same byte-zero lock. `scripts/dev.py` reads the existing outcome
  and REST hash; it writes no shared data shape. Source evidence:
  `scripts/dev.py:206-220,291-303`, launcher `:1079-1127,1540-1573`,
  `cmd/ws-mcp/proxy.go:419-422`, `internal/wsdashboard/ensure.go:178-207`.

- item 4 deferred to follow-up by owner direction 10:56
  The isolated Codex admission implementation is parked at local branch
  `parked/exe-phase1-item4-codex` and reverted from this ticket's retained
  branch. The owner-directed Codex REST follow-up is not implemented here.

- Live-daemon inspection uses REST normally and retains the existing
  network-only `/mcp` hello fallback for compatible older daemons whose
  `/health` lacks a version (coordinator approval, 2026-11-16 01:14:40Z).
  No executable is run to read a version, capability, or hello. Rejected:
  removing the legacy fallback and creating an old-client boundary.
- "Did not answer" is not "absent": absence-based starts need a swap-sized
  health grace window and cross-process single-flight. Preserve real new
  daemon starts for newer numeric cores, explicit `--replace` and LAN-mode
  handoffs (coordinator approval, 2026-11-16 01:14:40Z). Equal-core hash,
  suffix or build time never automatically replaces a healthy daemon.
- Startup coordination (coordinator approval, 2026-11-16 01:17:44Z): updated
  launchers serialize starts with persistent `dashboard/launcher-start.lock`,
  separate from Go's daemon-held `dashboard.lock`. Use a non-inheritable
  descriptor and OS advisory lock (`fcntl.flock` on POSIX; byte-zero
  `msvcrt.locking` on Windows), with finally-unlock/close; never truncate,
  unlink or TTL-steal the lock. Hold the starter lock through health recheck,
  authorized staging/start and healthy successor or bounded failure. Losers
  poll over the network/retry the lock within the existing 75-second budget.
  Never acquire/hold Go's singleton lock across invocation of the starter.
  Preserve `DaemonHung`; a live or unqueryable endpoint PID vetoes absence
  startup. No process kill or endpoint deletion to manufacture absence.
  Rejected: treating silence alone as absence or sharing Go's held lock.
- The single-flight guarantee covers updated launchers, not old binaries.
  A rare Electron-versus-launcher race may execute one losing starter that
  exits under Go's singleton; the coordinator explicitly accepts this as a
  new-daemon start event. No cross-language start-gate interface is added.
- Owner (1205)=a, 2026-11-16 10:01: scope is ws executables only. The
  Python launcher stays; `.mcp.json` intentionally routes through it. Python
  process creation is not part of the V3 issue. Rejected: direct HTTP MCP
  transport replacing the launcher; the former Phase 2 is removed. The
  coordinator clarified on 2026-11-16 01:14:40Z that original Phase 1 items
  4 (Codex probe) and 5 (Pi probe cache) remain explicitly approved; Python
  is exempt, not those probes. Preserve credential/profile semantics.
- Staging keeps hash-addressed immutable filenames. Clean failed-copy `.tmp`
  files and conservatively prune only stale launcher-owned `dashboard/bin`
  executables unused by any live process, excluding current/accepted targets.
  Never prune Electron packages or retained rollback targets (coordinator
  approval, 2026-11-16 01:14:40Z). Rejected: stable-path overwrite on Windows.
  This ticket does not rewire Electron startup to replacement-capable Ensure.
- Mailbox waits are Python + network only, including Claude mod and Pi
  stream waits, one-shot fallbacks, and launcher CLI waits. They never stage
  a binary or invoke `daemon-start`; an unreachable daemon causes network
  backoff/reconnect instead. Only a session MCP launcher (`serve --stdio`)
  or explicit lifecycle mode may initiate a new daemon start. Coordinator
  approval, 2026-11-16 01:04:06Z, extends this rule to every other CLI proxy
  path: no staging/start on initial connect or reconnect; return or back off
  with a clear "daemon not running" result when unavailable.
- Profile audit: add only `tickets.create_empty` to `PiWorkerWSTools`;
  mini-lead inherits it (`internal/wsagent/builtin.go#L95-L114`). No incident
  establishes another necessary grant. Preserve parentless delegate keys,
  bootstrap exclusions, overrides and saved launched-profile ceilings; do
  not widen an existing process snapshot to admit a new child definition.
  Rejected: blanket ticket tools or lead authority. This technical fact-gap
  settlement is lead-confirmed under the coordinator's autonomy directive.
- Codex admission on Windows uses the existing registry definition of live:
  some `d.index.Sessions()` entry has `Harness == "codex"` and `Live`;
  nil index means none. `Sessions` recomputes no-ended/heartbeat-under-five-
  minutes liveness (`internal/wsdashboard/index.go#L473-L492`,
  `registry.go#L202-L213`). A crashed session can remain live until expiry;
  accept that existing definition rather than adding PID/holder admission.
  Gate immediately before `queryCodex`, including forced refresh. Retain
  previous usage values and existing unavailable status; suppression never
  authorizes a Pi-token fallback. Non-Windows behavior remains unchanged
  (`plan_usage.go#L198-L221`, `plan_usage_probe.go#L19-L64`).
- Pi derived cache: operation-specific files under
  `<cache>/dashboard/pi-probes-v1/<fingerprint>-{version,models}.json`,
  schema 1 with kind, identity, original observation time and parsed value.
  Cache only successes, never raw output, credentials, errors, permissions
  or profiles. Fingerprint operation, canonical command path, size and
  nanosecond mtime; include existing resolved npm shim/Node/CLI/package
  stamps and, for models, effective agent-directory identity plus stat-only
  `models.json`/`auth.json` stamps including absence. Reuse wrapper resolution
  without changing how the probe is executed (`internal/wsagent/backend.go#L449-L519`,
  `pi.go#L57-L64,L1251-L1257`). Version success lasts while identity matches;
  model success lasts five minutes from observation, not cache read. Preserve
  existing in-memory version once/failure behavior and model failure TTL
  (`support.go#L48-L55,L141-L157`, `internal/mcp/pi_tools.go#L72-L94`).
  Missing/corrupt/stale records use the existing bounded probe; newer schemas
  are ignored and preserved. Validate decoded values using existing parsers;
  owner-only atomic write failure must not invalidate a successful probe.
  Rejected: shared read-modify-write maps, credential-content fingerprinting,
  persistent failure caching or an unrelated mid-process version refresh.
- Compatibility tier: T0. No tool/argument/hello/endpoint/profile or existing
  usage-store shape changes; the grant exposes an existing handler, and
  legacy hello remains supported. Old endpoint/metadata/stamp readers and
  writers retain their current shapes (`internal/wsdashboard/endpoint.go#L48-L94`,
  launcher #L196-L240,L300-L349); new reopen reads them unchanged. The new
  derived Pi directory is disjoint from old `support.json` writes
  (`internal/wsagent/support.go#L183-L213`), session pruning
  (`internal/wsdashboard/registry.go#L331-L385`) and `dashboard/bin` pruning
  (`spawn.go#L89-L107`). Thus old code neither rewrites nor deletes the cache;
  new reopen validates/reuses it, or recomputes missing/invalid derived data.
  Add a directional fixture exercising those old write/prune paths before
  new reopen; no T2 recovery or new T3 boundary is claimed.

## Prior Decisions

- 1866275e (2026-11-13, commit): "Owner priority: AhnLab prompts on non-daemon ws execution; remove ProcessStream, DIRECT bypass, catch-all exec, and executable inspection instead of adding another fallback." — bearing: constrains
- 261113-bug-ws-only-the-dashboard-daemon-runs-an-exe (2026-11-13, Decisions): "A newer launcher meeting an otherwise compatible older daemon without a version endpoint must still work: fall back to the existing /mcp hello's returned daemon version" — bearing: resolved by retaining network-only legacy hello
- 96b27026 (2026-11-10, commit): "stage_daemon_binary no longer deletes other entries: one may be the running daemon's own executable (it respawns itself from it on a LAN change)." — bearing: constrains
- f9b3c792 (2026-11-13, commit): "Codex CLI owns its credentials. Only absent/API-key CLI logins use Pi's unrefreshed token, confined to a bounded usage request; redirects and upstream errors cannot forward or expose it." — bearing: resolved by preserving credential precedence
- d3500a7c (2026-11-14, commit): "Release-gate G3 D: detach usage queries from viewer cancellation with a bounded 10-second context, record lastQuery after completion, and require same-origin evidence for forced refresh" — bearing: constrains
- 7f40e811 (2026-11-13, commit): "pi-worker ws tools: git.status, git.diff, git.log, git.merge_base, git.commit, enumerated from the delegate-implementer floor's commit step and the reviewer playbook's git.diff" — bearing: constrains
- 261115-feat-werkschaft-electron-daemon-override (2026-11-15, Decisions): "Electron launches the daemon binary bundled with the app by default. A setting can select a development build for dogfooding. Neither path depends on an installed plugin runtime." — bearing: constrains
- fbd1cd6a (2026-11-13, commit): "The launcher sets WS_RSRC_ROOT=<plugin>/rsrc before daemon-start; dev.py set nothing, so the daemon fell back" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1: three bin/ws-mcp-launcher.py copies; agents-plugin-tool/internal/{mcp,wsagent,wsdashboard}; Claude register.ts and Pi mailbox-waiter.ts. |
| scope.surface | cross-module | Launcher lifecycle/CLI, daemon Ensure/staging, MCP command rendering, Pi profiles and usage/probe caches; Electron shares startup contracts. |
| scope.new_public_symbol | no | Existing tickets.create_empty is exposed in builtin worker profiles; no new MCP/API or cross-language start-gate interface. |
| scope.new_type_contract | yes | Decisions define the private schema-1 Pi derived cache and updated-launcher advisory starter lock. |
| scope.test_surface | existing | agents-plugin/tests/test_ws_mcp_launcher_proxy.py; agents-plugin-pi/test/mailbox-daemon.integration.test.ts; agents-plugin-tool/internal/wsagent/{builtin,profile_spawn,spawn}_test.go; internal/wsdashboard/{daemon,version,plan_usage_probe,plan_usage_history,plan_usage_compat}_test.go; internal/mcp/{pi_tools,mailbox_tools}_test.go. |
| complexity.reuse_points | confirmed | Read DaemonLink.probe/wait_healthy/connect, wait_daemon_connection/proxy_cli, stage_daemon_binary, Ensure/StageExecutable/PruneStagedExecutables, runListing/piProbe, Support.listChoices, queryPi and profile snapshot/subset fixtures. |
| complexity.side_effect_risk | high | Starts/drains shared daemon processes, deletes staged files and changes worker tool admission; mailbox must never acquire lifecycle authority. |
| risk.correctness | high | Endpoint silence does not establish absence; concurrent starters and cleanup can race; forced refresh and persistent stale probe results can change behavior. |
| risk.fit | high | Preserve Go singleton/Electron startup ownership, approved single-flight boundary and credential/profile semantics across the explicitly retained Codex/Pi probe items. |
| risk.test | high | Existing warm-daemon mailbox tests do not prove cold/no-start behavior; Windows executable fixtures are prohibited and process sampling needs coordinator approval. |
| risk.security_or_contract | high | Parentless delegate keys and launched-profile ceilings must survive the audit; credential source, old-client discovery and new disk caches require directional compatibility evidence. |

## Phases

### Phase 1: No ws executable launch except a real new-daemon start

Goal: ws executables run only for real new-daemon starts by the session MCP
launcher, explicit lifecycle mode, or Electron. This includes legitimate
newer-core, explicit replacement and LAN handoffs. Ordinary steady-state,
mailbox/CLI proxy, reconnect-to-successor and model-tool paths launch none.

1. Reconnect storm (launcher items 2-3): replace the 1.5-second grace with
   swap-sized health polling. Before permitted staging/start, acquire the
   separate `dashboard/launcher-start.lock` defined in Decisions, recheck
   health and keep the lock until healthy successor or bounded failure.
   Losing updated launchers wait/retry over the network within the existing
   75-second budget. Preserve hung/live/unqueryable-PID distinctions rather
   than converting timeout into absence; keep legitimate replacement cases.
   Apply the gate to session-MCP startup/reconnect and lifecycle startup.
2. Model-run exe CLI: add only `tickets.create_empty` to the inherited
   builtin Pi worker tool floor, preserving scope/bootstrap/profile ceilings
   as settled in Decisions. Make every printed or taught command
   (`internal/mcp/playbook_tools.go:342` and any similar site) never name the
   runtime exe. Where a CLI path must be shown, show the python launcher's
   socket-proxy form, or better, the MCP tool name.
3. Staging hygiene for V3: retain hash-matching content-addressed reuse;
   clean failed-copy `.tmp` files and conservatively prune stale owned
   `dashboard/bin` entries not used by any live process. Preserve current,
   accepted and rollback targets and exclude Electron package staging.
   Verify existing equal-core retention rather than assuming an unproven
   build flip-flop; fix a remaining automatic replacement path if reproduced.
4. `codex app-server` plan-usage probe: suppress launch on Windows unless
   the existing session registry has a live Codex entry, including forced
   refresh. Preserve cached values, credential precedence and non-Windows
   behavior; suppressing Codex never permits a Pi-token fallback.
5. `pi --list-models` / `pi --version`: implement the private success-only
   cache, identity/invalidation and five-minute model freshness settled in
   Decisions. Prove restart reuse and invalidation without profile changes.
6. Mailbox waits: cut every path from Claude mod `register.ts`, Pi
   `mailbox-waiter.ts` (stream and one-shot fallbacks), and the launcher's
   `mailbox wait` CLI/reconnect handling to binary staging or daemon start.
   These paths use Python + HTTP/socket only and back off/reconnect when
   the daemon is unavailable. Only `serve --stdio` or explicit lifecycle
   mode may initiate a new daemon start. Apply the same no-start permission
   to every other CLI proxy command, with a clear "daemon not running"
   result when unavailable. Verify initial connect and reconnect, mailbox
   stream/one-shot, and non-mailbox CLI cases with mocked launcher tests
   that forbid staging/start.
7. Verification: no Go tests or test binaries on the owner's Windows PC
   (owner rule: Go tests on remote w5090 only). Prove item 1 with launcher unit tests (Python, no exe) and
   a w5090 run; prove the whole phase with a short process-sampling window
   (PowerShell `Get-CimInstance Win32_Process` polling, as in the trace)
   across one coordinated daemon swap: expect one starter invocation and
   its necessary daemon child, with no mailbox/CLI/probe ws executions.
   Report the accepted Electron-race boundary separately. Also cover fresh,
   expired and ended Codex registry entries, forced refresh, Pi-only/nil
   index, cached-value retention and unchanged credential fallback; Pi
   cache restart, wrapper/PATH/config invalidation, expiry, corrupt/newer
   schemas, failed writes and directional old-writer/new-reopen behavior.

### Live-acceptance repair scope (coordinator assignment 2026-11-16 02:59:27Z)

- Implement only the two dev lifecycle fixes in Decisions, in
  `scripts/dev.py` and focused `scripts/test_dev.py` cases. If an existing
  launcher test needs a deterministic assertion, limit it to this dev-gate
  interaction; do not split or refactor the three launchers.
- Mock all executable/PID APIs and control clocks. Prove a dev-held gate
  prevents an updated MCP contender from starting beyond its 20-second
  grace; successor publication yields zero extra starts. Cover bounded
  contention, exception/failure release and unchanged explicit replacement.
- Prove unavailable -> expected REST hash recovery invokes exactly one
  starter; old hash, no health, malformed/non-unavailable failures and budget
  expiry still fail with useful diagnostics. Never accept a PID alone or
  shorten production grace. No local launcher runs, builds or live swap.
- Run one independent R1, then fixes-only R2 for substantive fixes. Return
  the reviewed code/head and evidence to the coordinator; leave this ticket
  open. Codex REST is subsequent work, not part of this repair commit. The
  parked Codex commit is CLI-admission work, not a REST implementation.
- Source-only triage at `ae86b1408` is recorded in the machine-local report
  `C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac/review-paths/462376e9-01-exe-live-triage.md`.
  Swap evidence is under `D:/Temp/daemon-swap-1116/`; old in-memory launchers
  are the previously accepted single-flight boundary, not a new defect.

## Constraints

- Python launcher, holder, and hosted harness process creation remain
  unchanged. `.mcp.json` continues to use the Python launcher.
- Dev-only `go build` bursts and git executable use are outside this ticket.
- A live daemon swap and process-sampling acceptance require the coordinating
  lead's approval before touching the shared daemon. Workers never perform
  that swap, install or replace the shared runtime without approval.
- Never run Go, ws-mcp/ws-mcp-dashboard executables, test binaries or
  executable fixtures on the owner's Windows PC, including local builds
  and runtime smoke checks. Go builds/tests run only on remote w5090 via
  `D:/sys_lock.sh remote w5090 run`, synchronizing this isolated worktree.
  Use focused `-run` families, never broad unscoped package suites. Local
  Python launcher tests must mock all executable spawns; local Node-only
  checks must not launch executable fixtures.
- Work only in the acquired implementation worktree; never commit in or
  switch the shared root's branch. No merge, push or worktree release;
  return branch/head to the coordinating lead and retain the lease.
- Temporary artifacts belong only under an owned `D:/Temp/<purpose>/`
  directory and are cleaned after use. Stop if D: free space is below
  20 GB. Pi-authored commits carry no Co-Authored-By trailer.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Source-only grounding baseline: c32629f72 on
  `impl/develop/down-frame-gooey`, 2026-11-16. No tests, Go, local ws/test
  executables, daemon operations, installs or source edits were performed.
  D: had 154 GB available before inspection. No temporary artifacts were created.
- All three launcher copies currently have SHA256
  `97b9874ca52732c5b933795785b085e4cf7d6c8d37a47de78a9af833f636c7d6`.
  Preserve their byte equality; ordinary `.mcp.json` Python routing is intentional
  (`agents-plugin/.mcp.json#L3-L8`; `agents-plugin-pi/src/mcp-stdio-client.ts#L362-L377`).
- Mailbox no-start cut points: both Claude paths share `waitArgv`'s Python
  command (`agents-plugin/hooks/claude-mod/register.ts#L281-L289,L525-L531,L588-L589,L1037-L1040`);
  Pi stream and one-shot both spawn Python
  (`agents-plugin-pi/src/mailbox-waiter.ts#L909-L913,L1124-L1128`).
  The hidden start authority is in the launcher, not these outer spawn commands:
  `main` makes cold CLI calls repair/install-capable before constructing a
  start-capable link (`agents-plugin/bin/ws-mcp-launcher.py#L1861-L1911`);
  `proxy_cli` calls `wait_daemon_connection`, whose FIRST attempt calls
  `link.connect` and can reach `start_daemon`
  (`agents-plugin/bin/ws-mcp-launcher.py#L1231-L1260,L1422-L1428,L1764-L1780`).
  Therefore cutting only adapter spawns or only reconnect is insufficient:
  mailbox entry must avoid repair/staging/start authority as well as connect's
  start path. Its subsequent pre-send retry loop already rereads endpoint and
  uses `daemon_upgrade` only; CLI EOF returns an error, leaving adapter backoff
  and respawn to re-enter main (`agents-plugin/bin/ws-mcp-launcher.py#L1244-L1260,L1781-L1787`;
  `agents-plugin-pi/src/mailbox-waiter.ts#L309-L310,L593-L600`).
  Keep `DaemonRequestSent` non-replay and cancellation semantics; MCP recovery
  is a different path (`McpProxy.recover`, launcher #L1612-L1634).
- Startup/absence evidence: `DaemonHung` refuses a no-grace hung probe, but
  `wait_healthy` catches it and can return no health after grace; the latter
  still authorizes start today (`agents-plugin/bin/ws-mcp-launcher.py#L1037-L1067,L1332-L1355,L1407-L1428`).
  The existing Go `dashboard.lock` is held only by the daemon; it is not a
  launcher single-flight lock (`agents-plugin-tool/internal/wsdashboard/endpoint.go#L6-L16`;
  `agents-plugin-tool/internal/wsdashboard/daemon.go#L303-L329`).
  Network silence plus a longer poll alone does not prove a daemon is absent.
  Decisions settle the separate starter lock, conservative hung/live-PID
  policy and updated-launcher-only guarantee; no Electron participation
  interface is introduced.
- Exec3 overlap, read-only branch snapshot `impl/develop/cape-barn-hence`
  at de648159179663a3877162898385b78136b1496c: its
  `agents-plugin-tool/desktop-electron/daemon-selection.js` is file-only,
  returning `{source,binary,contract,metadata,env}` from bundled/dev input.
  Its ready copy of `261115-feat-werkschaft-electron-daemon-override` records
  Phase 1 Result b927532f77 and plans missing-only detached `dashboard` startup
  through the existing singleton lock, not replacement-capable Ensure.
  This checkout still has that ticket in todo with no Result; the branch is
  not landed here (pending 261115-feat-werkschaft-electron-daemon-override).
  Shared contracts are `WS_CACHE_HOME`/dashboard lock, endpoint/REST identity,
  core precedence, metadata hashes, staged executable lifetime, and
  `WS_RSRC_ROOT`/`WS_SKILLS_ROOT`/hostable plugin package. Exec3 retains whole
  out-of-bundle staged packages with no automatic pruning and separately gates
  Windows strict lifetime. This ticket's item 3 must not silently prune that
  namespace or introduce a competing package/start interface. No ordering
  dependency or new strict-lifetime flag is settled by this evidence.
- Related startup work is not a landed prerequisite: todo
  `261116-bug-ws-shared-daemon-launcher-dependent-claude-plugin-root` owns
  Claude package identity independently of Pi resources; todo
  `261115-feat-ws-dev-daemon-accepted-build-recovery` owns accepted-build
  retention/promotion/rollback; todo `261115-feat-ws-daemon-release-auto-update`
  owns user-approved shell release download/swap. Item 3 cleanup cannot infer
  that an unused executable is an unneeded rollback target. Current equal-core
  builds do not automatically replace one another; older numeric cores do not
  replace newer ones (`agents-plugin/bin/ws-mcp-launcher.py#L1349-L1355`;
  `agents-plugin-tool/internal/wsdashboard/ensure.go#L163-L174`). A present-day
  flip-flop still needs causal evidence, not an assumed newer-build tie-break.
- Profile security: item 2 explicitly authorizes adding `tickets.create_empty`,
  not blanket ticket/lead authority. Both builtin Pi profiles currently omit
  it (`agents-plugin-tool/internal/wsagent/builtin.go#L95-L114`); the tool
  already dispatches to `wsdoc.TicketCreate` under the resolved root
  (`agents-plugin-tool/internal/mcp/server.go#L2046-L2079`). Non-lead agent
  keys are parentless delegates (`agents-plugin-tool/internal/mcp/agent_keys.go#L12-L29`),
  not top-level leads. The explicit allowlist precedes the keyed role gate,
  which alone does not enforce a worker-specific surface
  (`agents-plugin-tool/internal/mcp/server.go#L730-L772,L5694-L5702,L5723-L5737`).
  Preserve scope, keyless/bootstrap exclusions, token depth/tier checks,
  overrides and saved launched-profile subset ceilings. A routine-tool audit
  must name and justify any additional grant; it cannot convert these keys
  into leads or rewrite their parentage. Done
  `261113-feat-ws-pi-mini-lead-profile` previously rejected widening pi-worker
  alone; ready `261115-feat-ws-worker-profile-session-spawn` is a distinct
  no-ticket worker posture; todo `261116-feat-ws-subagent-worktree-lease`
  separately owns worktree-tool widening. None authorizes blanket widening here.
- Compatibility evidence is directional and provisional, not a blanket tier:
  - Unchanged endpoint/health/hello fields and existing calls target T0:
    old/new readers still read schema-1 `endpoint.json`; the daemon atomically
    rewrites its fixed shape (`agents-plugin-tool/internal/wsdashboard/endpoint.go#L48-L94`).
    Unknown additions would be lost on a fixed-shape rewrite, so do not add
    coordination state there without a reader/writer/reopen design. Existing
    protocol/minor refusal remains T3 with no data-reopen remedy
    (`agents-plugin-tool/internal/wsdashboard/proxy.go#L232-L254`); todo
    `261112-feat-ws-mixed-version-compat-guards` does not supply a landed bypass.
    Existing network-only legacy hello discovery remains approved
    (`agents-plugin/bin/ws-mcp-launcher.py#L1311-L1330`), preserving compatible
    older daemon use without executable inspection.
  - Keep schema-1 `<binary>.metadata.json` identity and schema-2
    `.compatibility.json` meanings: old-style metadata writers copy identity
    members, then bind actual bytes; stamp writers replace a fixed payload.
    Reopen rehashes bytes and recomputes the stamp, not lost arbitrary fields
    (`agents-plugin/bin/ws-mcp-launcher.py#L196-L240,L300-L349`). T0 is grounded
    for unchanged shapes, not for extra fields or an unchosen stable-path layout.
  - Existing `plan-usage.json` fields are `claude/gpt` and typed
    harness/plan/source/status/asOf/windows entries; old-style decode/save
    loses unknown members and new reopen has no raw recovery
    (`agents-plugin-tool/internal/wsdashboard/plan_usage.go#L18-L67`). Preserve
    those shapes for a T0 source-selection repair. `plan-usage-history.json`
    schema 1 preserves unknown fields but validates source values and refuses
    newer schemas/unknown source semantics before normal use; a new source
    label is not automatically T0 (`agents-plugin-tool/internal/wsdashboard/plan_usage_history.go#L13-L85,L128-L204`).
  - Decisions settle the private Pi cache and directional T0 evidence; keep
    strict Profile (`agents-plugin-tool/internal/wsagent/profile.go#L218-L237`)
    and nested `resolved_definition` unchanged.
- Probe evidence: the coordinator explicitly retained items 4 and 5.
  `queryPi` reads Pi OAuth credentials only
  when Codex login presence is absent, never as a selected Codex-credential REST
  source (`agents-plugin-tool/internal/wsdashboard/plan_usage_probe.go#L30-L43,L178-L222`).
  That detection alone supplies no active-session predicate
  (`agents-plugin-tool/internal/wsdashboard/plan_usage.go#L134-L165`);
  Decisions select the existing session-registry liveness definition instead.
  Persistent Pi identity must account for command wrappers/PATH, not assume
  the version and model probes resolve the same file
  (`agents-plugin-tool/internal/wsagent/pi.go#L57-L64,L1251-L1257`). Related idea
  `261114-bug-ws-pi-version-probe-never-refreshes` already records stale
  once-per-process/PATH selection; related idea
  `261115-bug-ws-dashboard-fixture-real-home-plan-usage-probe` records real-home
  fixture exposure, not permission to use live credentials.
- Scoped verification suggestions only (none run): extend mocked launcher
  `main`/`proxy_cli` cases in `agents-plugin/tests/test_ws_mcp_launcher_proxy.py`
  for cold/missing/stale/refused/hung endpoints, swap, stream EOF and one-shot
  re-arm with staging, repair and subprocess APIs forbidden. Assert zero ws
  execution for every CLI mode, plus separate permitted session-MCP/lifecycle
  starts. Add concurrent-process fake-starter lock tests with bounded time;
  retain initialize-only replay, request IDs, UTF-8, fail-open Stop and abort.
  Existing Pi `test/mailbox-daemon.integration.test.ts` covers warm fake-daemon
  routing, not unavailable-daemon no-start; its Windows script sentinel is not
  proof of zero executable fallback. Use injected spawns there and in Claude
  mod stream/fallback tests; keep actual `.mcp.json` Python routing.
  Remote w5090 only: focused `TestStageExecutableCopiesAndPrunes`, Ensure
  concurrent-start/core/LAN cases, Pi builtin/subset/snapshot/restart fixtures,
  `TestPiToolsGate`, `TestMailboxWaitCommandRenderVariable`, and plan-usage
  cadence/expiry/redirect/non-destruction fixtures. Some plan-usage tests copy
  and execute the Go test binary as fake codex
  (`agents-plugin-tool/internal/wsdashboard/plan_usage_probe_test.go#L79-L110`):
  NEVER run these locally. Isolate HOME, CLAUDE_CONFIG_DIR, CODEX_HOME,
  WS_CACHE_HOME and WS_CONFIG_HOME; use only owned D:/Temp/<purpose> artifacts
  locally and clean them. Item 7 process sampling and any shared swap remain
  coordinator-owned acceptance, not a fact-population command.

## Implementation checkpoint (not phase acceptance) - 2026-11-16

- Item 2 landed in `b5b73efb5`; item 5 in `4cc15f9ab`. Item 4 is parked
  at `parked/exe-phase1-item4-codex` (`d9fd94543`) and fully reverted by
  `36ecc6e43`; neither the Pi cache nor launcher changes depend on it.
- Pending launcher work adds a persistent OS starter lock, a 20-second
  swap grace, conservative recorded-PID absence admission and network-only
  CLI entry/retry paths. All subprocess APIs in the local proxy family are
  mocked, including permitted starters. No local ws/Go/test executable ran.
- Coordinator-approved conservative cleanup: each failed Python copy removes
  only its own random temp path. Go's existing pruning hook now retains all
  settled artifacts: this baseline has no authoritative accepted/rollback
  retention source and singleton ownership does not prove that the previous
  draining process has exited. Old executables, metadata, orphan temps,
  directories and symlinks deliberately remain; no retention index/API or
  Electron package pruning is invented.
- Verification: new mocked starter contract family passed (10 tests, one
  remote-process fixture skipped). Existing mocked proxy family: 56 of 58
  tests pass; `test_a_daemon_that_dies_with_no_successor_is_restarted` and
  `test_recovery_repairs_missing_runtime_only_when_start_becomes_needed`
  still fail waiting for the successor hello (5/8 seconds). A first correction
  isolated fake PID admission, removed obsolete CLI-start expectations and
  made timeout/symlink fixtures independent of Windows environment. The two
  reconnect failures repeated afterward: worker stop (f), no further fix
  attempt before checkpoint. Full commands/output are retained in machine-local
  `D:/Temp/exe-phase1-launcher/proxy-failure-{1,2}.txt`. Their
  implementation-versus-fixture blame remains unresolved at this checkpoint.
- The first new-family failure was a fixture reading a Windows-locked byte;
  reading after unlock fixed it. The proxy family's first run also exposed
  restricted Windows symlinks and an environment-dependent HTTP timeout;
  those fixture issues were corrected. No passing full-proxy claim is made.
- Item 5 focused remote wsagent family passed (0.158s, full output read by
  its implementer). Item 2 focused MCP render family passed (0.195s); its
  original wsagent output was truncated and deleted before full review, so
  that older run is not verified-pass evidence. The cmd re-arm fixture is
  updated but unrun under the internal-package-only command authorization.
- Coordinator 02:14:46Z authorized bounded same-branch continuation after this
  checkpoint: first diagnose whether the changed health grace exhausts the
  fixture deadline; production health grace must not be shortened to fit tests.
  Larger changes require a new report. Remaining: diagnose the repeated
  reconnect failure, focused remote starter/staging/profile gates, independent R1 and
  substantive fixes-only R2 if needed. No reviewed HEAD or phase Result yet.
- Coordinator alone owns subsequent merge/build/install/swap and live process
  sampling. Acceptance preparation after reviewed code: sample
  `Get-CimInstance Win32_Process` at 250-ms intervals into an owned
  `D:/Temp/exe-phase1-acceptance/` CSV before/during/after one coordinated swap;
  exercise session MCP reconnect plus mailbox stream/one-shot and ordinary
  CLI proxies, and inspect startup count and command lines. Expect one
  updated-launcher starter plus necessary daemon child; record the accepted
  rare Electron losing-starter boundary separately. Polling can miss very
  short-lived processes and is not a kernel process-start trace. No such
  command, swap or install has been executed by this worker.

### Bounded continuation evidence - 2026-11-16

- Confirmed diagnosis: with fixture grace `0.02s` and retry budget `0.3s`, a
  closed Windows localhost port raised `DaemonHung` after `0.0529s`, before
  any starter ran. The 20-second production grace was already test-overridden;
  the first PID-only fixture correction did not address this handshake/read
  timeout conflation. Coordinator 02:17:32Z approved separating connect
  failures/timeouts (unreachable) from established-connection timeouts (hung),
  retaining recorded-PID vetoes. Reconnect fixtures now use a short socket
  budget; production swap grace is unchanged.
- Mocked local gates passed: starter family 12 tests (one remote-only process
  fixture skipped), proxy family all 58 tests. A final pipe-fixture close
  removed ResourceWarnings; its affected one-test gate passed without warnings.
  Virtual-clock cases cover unreachable at t0, successor at t4 inside the
  20-second grace with zero starts (initial/reconnect), and no successor with
  exactly one start only after grace expiry.
- Remote w5090 focused `^Test(LauncherStarterSingleFlight|StageExecutable|Ensure|KeepRunningCorePolicy)`
  passed (1.226s), including six competing Python processes, crash lock release,
  failed-copy cleanup, conservative retention, numeric-core and LAN cases.
- Remote w5090 focused builtin/subset/saved-snapshot/old-writer family passed
  (0.190s). Its first run correctly exposed a test-fixture mismatch: the
  directional old-pruner fixture called today's now-conservative hook. Freeze
  the pre-change prune policy inside that compatibility fixture so it still
  proves actual old-write -> new-reopen; do not restore unsafe production
  deletion to satisfy the old assertion.
- Node-only injected-spawn Pi stream/fallback/EOF family passed (6 tests).
  Outer Claude/Pi wait commands remain Python; the launcher cuts their hidden
  start authority centrally. No executable integration fixtures ran locally.
- All three launcher files are byte-identical. Item 4's five source/test files
  remain byte-identical to the planning base after the explicit revert.
- Independent R1 is next; this continuation still records no phase Result,
  ticket closure, live swap, install, merge, push or acceptance success.

### Independent R1 and remediation - 2026-11-16

- R1 reviewed `8a8e52498..236ca99dd`, reporting no Critical and two Important:
  R1-I1 (Windows fresh-file initialization before byte-lock ownership) and
  R1-I2 (native PID overflow escaping the conservative unknown-PID guard).
  Full report: machine-local
  `C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac@a80733aa/review-paths/0d710944-01-exe-phase1-r1.md`.
- R1-I1 fix: initialize byte zero only after successfully taking its advisory
  lock. A deterministic mocked Windows contender regression forbids any
  initialization write without lock ownership; the native Windows unit case
  also exercises acquisition beyond EOF. The persistent file is neither
  truncated nor unlinked and its descriptor remains non-inheritable.
- R1-I2 fix: native PID-conversion overflow means unknown, never dead. Both
  platform helper paths and the controlled MCP-unavailable/zero-start path
  have explicit overflow regression cases, with native PID APIs mocked.
- Focused fix gates passed: local mocked starter family 15 tests (one
  remote-process fixture skipped), remote w5090 `^TestLauncherStarterSingleFlight$`
  passed (0.909s). The three launcher copies remain byte-identical, SHA256
  `0e137b1d0989d3ffab6f95295957d342474e48ac42efdbf8cc94ee48205f9ddd`.
- The routed lite allocation permits no worker-owned re-review. The lead
  dispatches independent fixes-only R2 for these substantive fixes; no
  reviewed-HEAD claim is made before that closure. Live acceptance remains
  separate, coordinator-owned, and unexecuted; ticket stays open without Result.

### Reviewed source boundary and coordinator handoff - 2026-11-16

- Reviewed code HEAD: `79841d02677a075cd3b55d31df8e1fe3af0f45fd`.
  Independent lead-dispatched fixes-only R2 PASS closes R1-I1 and R1-I2;
  no substantive fix-introduced issue or new Forward observation remained.
  Full report:
  `C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac@a80733aa/review-paths/6e57526d-01-exe-phase1-r2.md`.
  R2 inspected the exact `236ca99dd..79841d026` fixes and full focused evidence;
  it did not claim a fresh broad sweep or live acceptance. This handoff commit
  changes documentation only; reviewed source remains exactly that code HEAD.
- Source/verification boundary: original items 1/2/3/5 and 6-prep are implemented
  and independently reviewed, with the focused evidence above. Full suite,
  native Windows multiprocess sampling and coordinated swap acceptance remain
  unperformed. Windows contention/PID overflow have deterministic mocked
  coverage; the actual multi-process/crash-release gate ran only on w5090.
- Preserve machine-local evidence in `D:/Temp/exe-phase1-launcher/`:
  `proxy-failure-1.txt`, `proxy-failure-2.txt`, `continuation-evidence.txt`, and
  `r1-fix-evidence.txt`. These are small full-output records, not build caches.
- item 4 deferred to follow-up by owner direction 10:56
  Its retained parked branch is `parked/exe-phase1-item4-codex` at `d9fd94543`;
  final item-4 source/test diff against planning base is empty. Do not begin
  that follow-up before coordinator live-acceptance results.
- Coordinator owns release of this leased worktree, integration, remote build,
  install and one shared-daemon swap. The worker performed none of them.
  Keep this ticket in ready, without a phase Result or closure, until the
  coordinator supplies actual live-acceptance evidence.

#### Prepared acceptance procedure (coordinator only; not executed)

1. Approve/select the integrated build and updated launcher copies, using the
   coordinator's remote build/install procedure. Record pre-swap `/health`
   identity and the endpoint PID; do not execute a binary to inspect identity.
2. In a separate PowerShell console, capture a 90-second process sample:

   ```powershell
   $out = 'D:/Temp/exe-phase1-acceptance'
   New-Item -ItemType Directory -Force $out | Out-Null
   $until = (Get-Date).AddSeconds(90)
   $rows = [System.Collections.Generic.List[object]]::new()
   do {
     $seen = (Get-Date).ToUniversalTime().ToString('o')
     Get-CimInstance Win32_Process |
       Where-Object { $_.Name -like 'ws-mcp*.exe' } |
       ForEach-Object { $rows.Add([pscustomobject]@{
         SeenAt=$seen; PID=$_.ProcessId; ParentPID=$_.ParentProcessId
         Created=$_.CreationDate; Path=$_.ExecutablePath; Command=$_.CommandLine
       }) }
     Start-Sleep -Milliseconds 250
   } while ((Get-Date) -lt $until)
   $rows | Export-Csv "$out/processes.csv" -NoTypeInformation -Encoding utf8
   ```

3. Perform exactly one coordinator-approved swap while session MCP launchers
   reconnect. If using REST shutdown, read the active endpoint privately and
   send `POST http://127.0.0.1:<port>/shutdown` with its bearer token; allow the
   selected session MCP launcher or explicit lifecycle action to start the
   successor. Never log the token. Mailbox/ordinary CLI proxies are not starters.
4. Exercise existing Claude/Pi mailbox streams, one-shot fallback/re-arm and
   ordinary CLI proxies during the window. With approved plugin/key variables,
   the safe CLI form is `python3 "$plugin/bin/ws-mcp-launcher.py" tools` and
   `python3 "$plugin/bin/ws-mcp-launcher.py" mailbox wait --session-key "$key" --timeout 1s --format json`;
   stream adds `--stream` and is cancelled after observation. Read successor
   identity through `/health` and correlate distinct PID/creation-time rows.
5. Expect one updated-launcher starter invocation and its necessary daemon
   child, zero ws executions from mailbox/ordinary CLI/probe paths. Record the
   approved rare Electron losing-starter exception separately. A 250-ms poll
   can miss short-lived processes; report that measurement limit. Preserve
   only small acceptance evidence and remove owned temporary artifacts later.

## Live-acceptance repair source checkpoint - 2026-11-16

- Reviewed code HEAD: `d286b8eaf9fffc4dbfb2ae1ab86096de8e6d7a05`.
  Only `scripts/dev.py` and `scripts/test_dev.py` changed from planning HEAD
  `04e06b7b5fc080d824fb3b0d4a835bcf8e90f188`. The existing launcher gate is
  reused without a cross-module interface change; builds, seeding and staging
  finish outside it. One 60-second lifecycle deadline covers lock contention,
  explicit `--replace`, and expected-content-hash REST confirmation. Only
  parsed exit 1 / `outcome: unavailable` permits nonzero reconciliation;
  diagnostics remain visible and no second starter is invoked.
- Focused local command (all executable/PID/network boundaries mocked):
  `TMP=D:/Temp/exe-dev-repair TEMP=D:/Temp/exe-dev-repair DEV_TEST_TMP=D:/Temp/exe-dev-repair PYTHONDONTWRITEBYTECODE=1 python -B -m unittest scripts.test_dev.DevLifecycleTest -v`
  passed all 12 tests (0.593s); full output read at
  `D:/Temp/exe-dev-repair/dev-lifecycle-1.txt`. Actual advisory-lock checks and
  virtual clocks cover a dev-held gate through MCP contention beyond the
  production 20-second grace and successor publication at t25 with zero extra
  starters, shared-budget expiry, terminal release, explicit replacement,
  unavailable recovery, malformed/unrelated failures, old/no/PID-only health,
  late confirmation and hung responses. No executable fixture files were
  created by this focused family. This is not a full-suite or live proof.
- Independent focused R1 returned CLEAN for
  `04e06b7b5..d286b8eaf`, with no Critical, Important or Minor findings.
  Full report:
  `C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac@392ca909/review-paths/61cdf1a0-01-exe-dev-repair-r1.md`.
  No R1 fixes were required, so the conditional substantive-fix R2 does not
  apply. The reviewer read source and the supplied full test log; it ran no
  tests or live operations. This documentation checkpoint changes no reviewed
  source inputs.
- No Go/ws/test executable, launcher CLI, native PID query, build, install,
  runtime probe, daemon swap, sampling, merge, push or release ran. Temporary
  test directories were cleaned; only the small full-output record remains.
  Original accepted Phase 1 source, retention policy and the old in-memory
  launcher boundary are unchanged. PID 3452's historical cause is not proved.
- Ticket remains open without a phase Result: the coordinator owns integration
  and actual live acceptance. Stop before those live actions. Item 4 REST must
  be planned afresh after this repair lands; the parked CLI-admission branch
  is not a REST implementation and was not applied.
- Coordinator direction at 03:22:51Z folds duplicate
  `261116-bug-ws-dev-rerun-daemon-false-failure` into this repair. It is closed
  as absorbed, with a Resolution naming the reviewed code, focused tests and
  live-acceptance limit, so it cannot be assigned twice. Its unexecuted
  standalone phase warning is explicitly accepted for duplicate absorption,
  not treated as independent phase completion. No package-root/env-selection
  interface changed; the existing dev package selection is retained.

## Forward

- Live acceptance retained old executable/metadata pairs and pre-existing
  orphan temp files intentionally: the approved baseline has no safe
  process/accepted/rollback retention authority, so its pruning hook retains
  settled artifacts. Failed-copy cleanup owns only that copy's new temp;
  no historical sweep or new deletion policy is promised by this repair.
- PID 3452's historical extra-start admission lacks a captured decision/health
  trace. Repair the proven dev gate bypass without claiming that the CSV
  proves its exact cause; repeat acceptance remains coordinator-owned.
- Coordinator-approved boundary: old launchers and Electron do not share the new starter lock; a rare Electron race can execute one losing starter, with Go's singleton preventing a second daemon.
