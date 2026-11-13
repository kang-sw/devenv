---
title: "ws: only the dashboard daemon runs a ws executable"
related:
  261110-feat-ws-mcp-daemon-hosted-proxy-launcher: reverses its direct-fallback decisions and completes the Python-only adapter architecture
  261112-feat-ws-mixed-version-compat-guards: owns skew notices, schema guards, and tool-list refresh; its direct-fallback assumptions are superseded here
  261113-bug-ws-pi-subagent-turn-dies-on-daemon-restart: holder recovery remains separately owned
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2295162df900666f
sage-review-completeness-reviewed: 2295162df900666f
completed: 2026-11-14
---

# ws: only the dashboard daemon runs a ws executable

## Background

**Owner priority: high.** AhnLab V3 prompts when a new executable runs. A
content-hash staging path does not solve that if adapters still execute it
for probes, mailbox waits, hooks, or fallback MCP servers. The intended
process model is the one dashboard daemon of its cache root hosting ws servers
for that cache root, with Python launchers/holders as adapters. A direct `serve --stdio` is a full
MCP server, not a harmless transport helper, and still opens shared stores.

### Verified evidence

Coordinates below were checked on `develop` at `47b7f4417`; paths under
`cmd/ws-mcp/` mean `agents-plugin-tool/cmd/ws-mcp/`, not a root-level `cmd/`.
Use the named symbols/searches to find edit sites after parallel work lands.

| Evidence | Current behavior |
|---|---|
| `agents-plugin/bin/ws-mcp-launcher.py:1276-1283,1381-1415,1643-1652` (`DaemonLink`, `connect`, `proxy_serve`) | Unreachable/hung daemon, unsuccessful start/replacement, minor skew, or rejected upgrade/hello can select the direct initial-connect fallback. The upgrade/hello implementation is at `1212-1253`. |
| Launcher `893-898,1175-1180,1536-1566` (`PROXY_MAX_QUICK_CLOSES`, `ProcessStream`, `McpProxy.recover`) | Reconnect failure or three quick closes creates a direct child; once the stream is a `ProcessStream`, recovery returns rather than switching back to the daemon. The session stays direct. |
| Launcher `1634-1640,1716-1717,1784-1802` (`proxy_mode`, `proxy_disabled`, final dispatch) | Only exact `serve --stdio` and `mailbox wait` are proxied; `WS_MCP_LAUNCHER_DIRECT` bypasses even those. All other arguments reach `subprocess.call`/`os.execvpe`. |
| Launcher `210-241,711-755,1699-1713,1754-1765` (`run_binary`, `tools_compatible`, `runtime_fully_compatible`, `compat_probe_binary`) | Compatibility executes `runtime capabilities`, then legacy `version`, temporary `serve --stdio`/`tools/list`, and command probes. Staging the probe copy still executes an exe. |
| Launcher `1346-1379` (`start_daemon`) | Stages the binary and executes `daemon-start`; this is the allowed daemon lifecycle operation, not a compatibility probe. |
| `scripts/dev.py:146-159,207-232,234-273,280-297` (`check_built`, `run_daemon_start`, seeding) | `check_built` executes the fresh binary's `version` and capabilities before staging/starting it; seeding carries the compatibility stamp. Preserve the intended start/replacement, remove probe execution. |
| `agents-plugin-pi/src/index.ts:1016-1046`; `agents-plugin-pi/src/mailbox-waiter.ts:849-878` | Source-loaded Pi stages `localRuntimeBinary` and passes it to the waiter, whose `spawn(options.runtimeBinary ?? "python3", ...)` executes it directly instead of Python. |
| `agents-plugin-pi/src/local-devenv.ts:249-273`; `agents-plugin-pi/src/bridge.ts:637-653` | Child-runtime repair invokes the Python launcher with `version`, which currently falls through to the exe. Python as the outer command alone is not proof of zero exe execution. |
| `agents-plugin/.codex-plugin/hooks.json:4-10`; launcher `1634-1640,1794-1802` | Codex Stop already invokes Python with `mailbox codex-stop-hook`, but that command is not proxied and therefore executes the runtime directly. Check the wsflow hook mirror too. |
| `agents-plugin-tool/cmd/ws-mcp/main.go:84-112` (`serve`) | Direct serve constructs `mcp.NewServer`, starts dashboard handling, and runs `ServeStdio`: another in-process server over the shared stores. |
| `agents-plugin-tool/internal/wsdashboard/daemon.go:634,701-715` (`handleHealth`) | Existing `GET /health` supplies version, PID, build time, and content hash. A new version endpoint is not necessary for the current daemon. |
| `agents-plugin-tool/cmd/ws-mcp/dashboard.go:269-275` (`newAgentRunner` / `WSMCP`) | Daemon-created worker MCP currently uses `os.Executable()` with `serve --stdio`. This related fix belongs to branch `fix/pi-spawn-key-bound-to-cwd-worktree`, not this ticket. |

## Decisions

1. **Only the dashboard daemon of its cache root runs a ws executable (owner, 2026-11-13; scope clarified by lead).**
   > "exe는 전체 통틀어서 대시보드 하나만 뜨고, 나머지는 전부 python 어댑터인 컨셉입니다."

   The dashboard daemon is the only long-lived ws executable in its cache
   root. Everything else in the ws process topology is a Python adapter
   (`bin/ws-mcp-launcher.py`, holder). This concerns which processes execute
   a ws exe, not the number of daemons across cache roots. Keep the one-daemon-
   per-cache-root decision of `261110-feat-ws-mcp-daemon-hosted-proxy-launcher`
   (in practice one per machine for one install). This does not ban the user
   harness, Git, or other tools from running. The only permitted ws exe
   execution is starting the daemon itself (`daemon-start` or equivalent)
   when none runs for that cache root or replacement is required; a short-lived
   lifecycle starter is not a second MCP server. Preserve the per-cache-root
   singleton lock and orderly handoff.

2. **Remove direct fallback entirely (owner, 2026-11-13).**
   > "fallback으로 직접 실행이 결정사항에 있었을텐데, 이 경로를 아예 버리는 방향으로 갑니다."

   This explicitly reverses Decision 5, the direct-minor-skew/reconnect parts
   of Decision 7, and the fallback/one-shot-execution exceptions of Decision 8
   in `261110-feat-ws-mcp-daemon-hosted-proxy-launcher` (board (644), (646),
   (649)). Implementation evidence: `09b041ed6` introduced the fallback and
   `ProcessStream`; `96b270265` added bounded reconnect/three-quick-close
   handover and hung-daemon fallback. Recover history with
   `git log -S 'class ProcessStream' -- agents-plugin/bin/ws-mcp-launcher.py`
   and `git show 96b270265`. The earlier preference for availability over an
   antivirus prompt is no longer authority.

3. **Whiteboard (38)=a, including the owner's note (2026-11-13).** Remove
   every long-lived direct exe path: initial/reconnect/three-quick-close and
   version-skew `serve --stdio` fallback; `WS_MCP_LAUNCHER_DIRECT`; the Pi
   waiter's direct runtime spawn; and Codex `mailbox codex-stop-hook`'s
   non-proxied dispatch. Route waits and hooks Python -> daemon, not Python
   -> exe. Remove the catch-all runtime exec escape for product commands;
   retained launcher command shapes must use daemon dispatch or metadata-only
   introspection, never silently become direct execution.

   > "버전 확인도 exe를 실행하도록 하면 결국 안랩 v3를 계속 터치하는 거 아닌가요? 실행중인 데몬에 REST API를 날리는 방식이라던지 해서 버전체크를 하면 좋겠다"

   Version and compatibility checks execute **zero ws binaries**, including
   cold load, cache/stamp miss, repair, and local-devenv paths. Query the
   running daemon through existing HTTP `GET /health`; add a small read-only
   endpoint only for information genuinely missing there. Read the staged
   binary's identity from metadata (`runtime.json`, its file name, or a
   build-written stamp), never by running it. Remove the temporary
   `serve --stdio` probe and executable capabilities/command/version probes,
   not merely move them to the staged path. An inspection-only version check
   does not start a daemon just to obtain a version.

4. **Unavailable daemon behavior (lead technical decision).** Start/replace
   the daemon when appropriate, otherwise keep retrying with bounded
   per-attempt waits/backoff. While unavailable, tool calls fail clearly with
   **"dashboard daemon not reachable"** (include the underlying reason on
   stderr); never launch a second server. Initial-connect failure, failed
   replacement, reconnect failure, repeated quick closes, and incompatible
   peers all obey this rule. Retain JSON-RPC framing and request IDs; do not
   replay tool side effects. Retry exhaustion must not hang a tool call
   indefinitely or leave a request unanswered. Healthy reconnect preserves
   initialize replay, in-flight failure, EOF/drain, and mailbox ownership
   behavior without a `ProcessStream` handover. An obsolete DIRECT setting
   must never restore the bypass.

5. **Compatibility tier: T0 for the cross-version surface.** Launcher <->
   daemon is a cross-version contract: Claude, Codex, Pi, ws, and wsflow
   update independently. Keep hello protocol 1, existing fields, MCP tools
   and arguments, CLI argument shapes, and store formats unchanged; any
   added endpoint/fields/daemon command support are additive and optional.
   The removal of the owner-rejected local execution strategy/environment
   bypass is deliberate, not removal of an MCP tool or manifest field.
   Existing reconnect recovery is preserved, not a new protocol contract.

   A newer launcher meeting an otherwise compatible older daemon **without
   a version endpoint** must still work: fall back to the existing `/mcp`
   hello's returned daemon version (and MCP initialize/tools/list for live
   capabilities when needed), never to exe execution. Do not make a new HTTP
   route, build metadata field, or mandatory hello field a startup gate.
   Distinguish missing version information from an unreachable daemon.
   Preserve older-launcher -> newer-daemon behavior too. Actual protocol or
   minor rejection is diagnosed and retried/replaced where permitted, never
   bypassed with a second server; cross-minor notice/acceptance policy remains
   with `261112-feat-ws-mixed-version-compat-guards`.

   Metadata must identify the actual binary, not just assert the connecting
   package's desired version: bind build stamps to content hash and carry
   them through download/bootstrap/copy/stage/seed paths. Keep old binaries
   without the new stamp usable through existing verified release metadata
   or repair/rebuild that produces metadata, without executing a probe.
   No schema/protocol bump is needed for this additive contract. If the
   implementation requires a mandatory wire/manifest change, stop and
   re-tier under the release manual rather than shipping a hidden T2.

6. **Related ownership boundary.** The daemon spawning Pi workers' MCP as
   its own exe (`dashboard.go:269-275`) is being fixed on
   `fix/pi-spawn-key-bound-to-cwd-worktree`. Coordinate/rebase and include
   that route in combined acceptance evidence; do not duplicate its fix or
   absorb holder restart-survival work. This ticket owns launcher mirrors,
   launcher/daemon command routing needed by hooks, `scripts/dev.py`, and
   Pi package waiter/version-check paths. No UI redesign or daemon restart
   is part of drafting this ticket.

## Constraints

- Applicable `AGENTS.md` Implementation Conventions rows; workers read each
  matching manual before edits:

  | paths | manual |
  |---|---|
  | `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/` | `ai-docs/manuals/shipped-surface-boundary.md` |
  | `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/` | `ai-docs/manuals/wsflow-mirroring.md` |
  | `agents-plugin-tool/internal/mcp/` | `ai-docs/manuals/ws-mcp.md` |
  | `agents-plugin-tool/internal/` | `ai-docs/manuals/release-compatibility.md` |

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

- Keep all three `bin/ws-mcp-launcher.py` mirrors byte-identical:
  `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-pi/`. Python
  adapters remain standard-library-only. Preserve per-connection root,
  environment, config manifests, tool restrictions, and owner identity;
  do not inherit another package's daemon-wide values.
- Non-UI implementation runs through `ws/pi.*` workers. Builds/tests run on
  **sys_lock remote w5090**, holding its resource lock; no local Go builds,
  tests, or vet and no silent local-build fallback for worker verification.
  The **Windows live check belongs to the lead/owner**, not the worker gate.
- Tests use isolated `WS_CACHE_HOME`, `WS_CONFIG_HOME`, ports, and fake peers;
  never restart the shared daemon, kill user processes, or push. Lifecycle
  subprocess tests stub the starter or use an isolated daemon. Distinguish
  the one allowed start/replacement from a forbidden probe/server child.
- Update `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/release-compatibility.md` in implementation: remove direct
  fallback, direct CLI/probe guidance, and minor-skew split-server claims;
  align with Decisions above and note the mixed-version ticket overlap.
  Historical completed ticket Results stay frozen. Shipped diagnostics and
  docs must be downstream-neutral; repo-only citations stay in this ticket.
- This is accepted `todo/` backlog; the lead populates route facts and runs
  the configured design/completeness reviews before `ready/` promotion.

## Prior Decisions

- 261110-feat-ws-mcp-daemon-hosted-proxy-launcher (2026-11-10, Decisions): "One daemon per cache root, as today" — bearing: constrains
- 09b041ed6 (2026-11-10, commit): "the direct path runs with one stderr line when the daemon cannot be used (or on WS_MCP_LAUNCHER_DIRECT=1)" — bearing: supports
- 96b270265 (2026-11-10, commit): "after PROXY_MAX_QUICK_CLOSES hands over to direct serve" — bearing: supports
- Rationale query unavailable; these verified history pointers are not a complete prior-decision search.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Launcher mirrors, agents-plugin-pi/src/, scripts/dev.py, agents-plugin-tool/ and manuals |
| scope.surface | cross-module | Python launchers, Go daemon command routing, Pi and dev tooling share a cross-version contract |
| scope.new_public_symbol | unknown | Additive daemon command support or endpoint may be needed; no new symbol is specified |
| scope.new_type_contract | unknown | Additive metadata or command framing may be needed; exact signature is unspecified |
| scope.test_surface | existing | agents-plugin/tests/test_ws_mcp_launcher_proxy.py, agents-plugin-tool/cmd/ws-mcp/launcher_proxy_test.go, scripts/test_dev.py |
| complexity.reuse_points | confirmed | DaemonLink, McpProxy, proxy_cli and handleHealth exist in launcher and daemon |
| complexity.side_effect_risk | high | Daemon lifecycle, reconnect and mailbox routing affect shared sessions and processes |
| risk.correctness | high | Retry, in-flight request handling and metadata identity must prevent lost or replayed side effects |
| risk.fit | high | Changes cross Python, Go and Pi boundaries while preserving separate ownership and compatibility |
| risk.test | high | Cross-version peers, process-spawn assertions and Windows antivirus acceptance require distinct coverage |
| risk.security_or_contract | high | Authenticated daemon routing and independently updated clients must preserve isolation and T0 compatibility |

## Implementation Checkpoint (1866275ec) - 2026-11-13

No phase is complete; this ticket remains in `ready/` without Results.

- `8ac8230ff`: hash-bound build/release/bootstrap/copy/seed identity replaces
  executable compatibility probes; `dev.check_built` reads metadata.
- `eaf75a31c`: existing product CLI handlers run in-process in the daemon,
  with per-connection environment/root/owner and optional hello `stdin`.
- `1866275ec`: daemon-only launcher dispatch, no ProcessStream/DIRECT/exec
  escape, immediate disconnected MCP errors and initialize-only replay,
  health/legacy hello discovery, Python mailbox re-arm commands, metadata
  staging, synchronized launcher mirrors, and updated operational manuals.
- Remote verification: metadata leaf passed 38 capabilities + 15 coldload +
  9 dev tests and metadata-only Linux/Windows cross-build checks. Go CLI leaf
  passed affected packages excluding launcher integration and the base LAN
  timeout. Latest launcher run passed all 87 launcher Python tests.
- Latest command: `bash D:/sys_lock.sh remote w5090 run
  single-exe-launcher 15 --sync <worktree> -- 'export
  PATH=$HOME/sdk/go/bin:$PATH; export PYTHONDONTWRITEBYTECODE=1;
  python3 -m unittest discover -s agents-plugin/tests -p
  "test_ws_mcp_launcher_*.py" -b && cd agents-plugin-tool && go test
  ./cmd/ws-mcp -count=1 -run "TestLauncher"'` was not green:
  `TestLauncherServesThroughTheDaemonItStartsFromTheStagedCopy` at
  `launcher_proxy_test.go:283` expects exactly one staged file, but staging
  now correctly carries `<binary>.metadata.json` too. The analogous Python
  directory assertion was fixed in the preceding run: worker stop (f), a
  second failure with the same stale-test root cause. The Go assertion is
  intentionally left for the lead's resumed/higher-tier run.
- Still required: finish Phase 1 integration/full gate and independent R1,
  fixes-only R2; Phase 2 Pi waiter/bootstrap metadata producer and cleanup,
  Pi tests and full gate. The daemon-created worker MCP bridge and holder
  spawning remain untouched, owned by their separate branches.
- No live daemon restart, image-name process kill, push, or Windows/AhnLab
  live acceptance was performed. Combined worker MCP acceptance and the
  Windows/AhnLab check remain lead/owner work.

## Forward

- Pre-existing remote Linux test: at base `91ca854f2`, locked w5090 command
  `go test ./cmd/ws-mcp -count=1 -run
  "^TestHostedConnectionSeeingALANChangeReplacesTheDaemon$"` reproduced
  `daemon_proxy_test.go:505: not within 10s: the predecessor to exit`
  (19.058 s). The CLI leaf diagnosed an exited zombie counted alive. Exclude
  this one test from the phase gate and track its process-liveness test
  separately; do not broaden this urgent ticket into lifecycle repair. The
  lead also confirmed three w5090-environment holder failures at `v0.49.0`:
  `TestUnattachedHolderIsListedWithItsCauseAndKilledUnderTheGuard`,
  `TestGuardedKillHoldsAfterAWallClockStep`, and
  `TestHolderOutlivesTheProcessThatStartedIt`. These four tests are the only
  exclusions from the full Go gates.

## Phases

### Phase 1: Make the launcher daemon-only and inspect versions without execution

**High priority; independently landable.** Remove `ProcessStream` fallback,
DIRECT bypass, and final catch-all exe dispatch in all launcher mirrors.
Unavailability uses the retry/error contract above. Route existing CLI calls,
including `mailbox codex-stop-hook`, to in-process daemon handlers using the
existing authenticated transport/result framing, or serve read-only identity
locally from metadata. Preserve hook stdout/exit behavior and cancellation;
do not implement a daemon handler by spawning its own exe. Keep existing
public command shapes rather than deleting them.

Use `/health` and backward-compatible hello/version discovery for the daemon;
use content-bound metadata for staged binaries. Carry metadata through the
build/release/bootstrap/repair/staging paths that this requires; never replace
an executable probe with another subprocess spelling. No mandatory metadata
upgrade for an already compatible older daemon. Keep `daemon-start` only as
the lifecycle exception. Update the operational/compatibility manuals and
replace tests that currently require direct fallback with the new assertions.

**Verification:** add/update focused tests (suggested names) in
`agents-plugin/tests/test_ws_mcp_launcher_proxy.py`,
`test_ws_mcp_launcher_capabilities.py`, `test_ws_mcp_launcher_coldload.py`,
and `agents-plugin-tool/cmd/ws-mcp/launcher_proxy_test.go` as applicable:

- `test_daemon_down_retries_then_error_without_exe_child`: missing/refused/
  hung endpoint and failed start/replacement; fake starter, bounded retries,
  tool error containing "dashboard daemon not reachable", zero actual exe
  children. Separate lifecycle test permits only `daemon-start` when absent
  or replaceable, never a parallel `serve`.
- `test_reconnect_failure_and_three_quick_closes_never_spawn_exe`: repeated
  EOF, grace/start failure, minor mismatch, rejected hello, and DIRECT set;
  no permanent direct session, no stranded request, no side-effect replay.
- `test_version_and_compatibility_checks_spawn_zero_ws_processes`: warm/cold
  stamp, repair/bootstrap, legacy capabilities-missing case, same-content and
  changed-content builds; spy on all process APIs and assert no ws exe spawn,
  including no temporary `serve --stdio` or inspection-only `daemon-start`.
- `test_new_launcher_uses_older_daemon_hello_when_version_endpoint_missing`:
  HTTP 404/missing version field, existing hello succeeds, tools remain
  callable, zero exe probe. Also test old hello against the new daemon.
- `test_codex_stop_hook_runs_via_python_launcher_and_daemon`: real manifest
  command shape for ws; wsflow has no Codex hooks manifest in the current tree
  (agents-plugin-wsflow/.codex-plugin/plugin.json; search for hooks.json under
  agents-plugin-wsflow). Assert no non-proxied exec and the same stdout/exit
  codes; daemon unavailable is diagnostic, never direct execution.
- `test_launcher_mirrors_are_byte_identical`; retain healthy reconnect,
  initialize replay IDs, EOF/drain, CLI UTF-8 framing, and owner-isolation
  regression cases. Run focused Python suites and remote Go proxy tests.

**Deferred boundary:** Phase 2 owns Pi-specific waiter/bootstrap callers and
`dev.py`; the separate branch owns daemon-created workers' MCP command.

### Result (fa267b5f8) - 2026-11-13

- Phase 1 completed. Commits `8ac8230ff`, `eaf75a31c`, `1866275ec`,
  `bb0d0e72b`, `f3305c01c`, and `fa267b5f8` implement daemon-only product
  dispatch, metadata-only compatibility/identity, in-process CLI/Stop routing,
  UTF-8 stdin, all supported stdio flag shapes, recovery-time repair, bounded
  download/build waits, and synchronized launcher mirrors/manuals.
- Lead continuation explicitly accepted the sidecar expectation change and
  overrode the earlier stop (f). Native merge `e1c0b61a9` imported develop's
  Python worker MCP bridge and Pi holder isolation without conflict; both
  intents are retained. No duplicate bridge/holder implementation was added.
- Full locked w5090 gate passed: 87 launcher Python tests; 24 wsflow tests
  (2 environment skips); `go test ./... -count=1` with only the four
  lead-confirmed environment exclusions in Forward. Independent R1 found
  three Critical and two Important issues: stdio flag dispatch, locale input,
  cold-repair bounds, worker-token CLI parity, and repair after a no-local
  runtime startup. All were fixed. Locked fix verification passed all 92
  launcher Python tests and Go launcher/worker-token CLI targets. Fixes-only
  R2 was clean (0 Critical/Important/Minor); no third review or extra sweep.
- `dev.py` metadata checks/seeding landed early with the identity seam;
  Phase 2 still owns the Pi waiter/bootstrap callers. Workflow route/render
  tools were unavailable; bundled conventions/reviewer guidance and native
  Git were used. No live dashboard restart, Windows/AhnLab acceptance, or
  push was performed. Live Windows/AhnLab acceptance remains owner-owned.

### Phase 2: Remove Pi/dev probe and waiter execution, then verify the topology

**Depends on Phase 1.** Pi always starts its mailbox waiter through Python
launcher -> daemon, including source-loaded/local-devenv sessions. Remove the
immutable-exe waiter staging/bypass and obsolete cleanup ownership, while
preserving re-arm, board wake kinds, abort, and TUI-safe diagnostics. Audit
Pi bootstrap/child-runtime version checks (including `runLauncher(["version"])`)
so no wrapper conceals an exe launch. In `scripts/dev.py`, replace
`check_built`'s executable version/capability checks with build metadata;
propagate the same identity through seeding/stamps. Keep the explicit daemon
start/replacement operation, but no pre/post-start exe probes.

**Verification:** add `test_pi_waiter_always_uses_python_launcher_and_daemon`
(source-loaded and release-backed; mail/board/interrupt, re-arm and abort),
`test_child_runtime_version_check_never_executes_ws_binary`, and
`test_dev_check_built_reads_metadata_without_ws_process_spawn` (in
`scripts/test_dev.py`; seed/copy/hash identity retained). Run focused Pi
waiter/local-devenv/bridge tests, dev tests, mirror/package guards, and remote
Go tests for affected packages. Record process-spawn assertions, not merely
that the outer process was Python.

Lead/owner Windows live acceptance, after coordination with
`fix/pi-spawn-key-bound-to-cwd-worktree`: with an already running compatible
daemon, exercise launcher startup, version checks, Pi mailbox wait, Codex Stop,
and daemon-spawned Pi worker MCP. Observe the one steady-state ws exe for
its cache root (the daemon), Python adapters/holders elsewhere, and no exe probes. Confirm
only a genuinely needed daemon start/replacement executes a ws binary. Do not
claim remote Linux tests prove the Windows/AhnLab behavior; record any live
check not yet performed explicitly. No UI changes, holder recovery redesign,
or mixed-version guard implementation is included in this phase.

### Result (1fea54cbc) - 2026-11-14

- Phase 2 completed, owner (551)=a. `677711b3a` was merged to develop in
  `0b3daf8e1` without its review under owner (45)=b. It sends every Pi mailbox
  wait, stream and one-shot, through `python3 <launcher> mailbox wait`, and it
  removes the waiter's immutable-exe staging, bypass and cleanup ownership.
  `b11facf4f` adds the named process-spawn tests, drops the redundant
  `runtimeReady` guard (`releaseLocalBootstrap` is idempotent), and refreshes
  stale comments. `1fea54cbc` regenerates the committed Pi bundle, which those
  `src/` edits had made stale. `dev.py` `check_built` and seeding had already
  moved to hash-bound metadata in Phase 1.
- Where the named tests live:
  - `test_pi_waiter_always_uses_python_launcher_and_daemon`:
    `agents-plugin-pi/test/mailbox-daemon.integration.test.ts`. It covers a
    release-backed and a source-loaded plugin, using a real launcher copy and
    the in-process fake daemon `test/fixtures/fake-daemon.ts`.
  - `test_child_runtime_version_check_never_executes_ws_binary`: the
    `createChildRuntimeEnsurer` case in `test/local-devenv.test.ts`.
  - `test_dev_check_built_reads_metadata_without_ws_process_spawn`:
    `scripts/test_dev.py`.
- Process-spawn assertions:
  - The runtime binary is a sentinel that journals any execution.
  - Every waiter spawn is `python3` with `[launcher, "mailbox", "wait", ...]`.
    Each wait reaches the fake daemon as exactly one CLI-mode hello.
  - The sentinel journal stays empty. No `cache/dashboard/bin` is staged, and
    the `go` stub of the source build never runs.
  - Stream mode: three wake kinds arrive on one child; a re-arm with changed
    board flags restarts it; abort closes the daemon connection.
  - One-shot mode: mail, board-inbox, board-steer, interrupt, timeout and
    abort.
  - Child-runtime version check: it installs the bootstrap and reports the
    metadata version. A bare `version` next to a daemon the launcher would
    replace reports from `/health`, with no hello, no execution and no staged
    daemon.
  - `check_built`: every `subprocess` and `os` process API is spied, and none
    is called.
  - Two mutations each failed the new tests: the launcher executing the binary,
    and inspection starting the daemon.
- Verification ran locally on macOS. sys_lock w5090 can only be driven from the
  Windows host, so this is a declared deviation.
  - Full Pi `node --test`: 2688 pass, 3 skipped, 0 fail.
  - Focused Pi set after `npm ci`: 247 pass, 2 skipped.
  - Launcher Python suites: 101 OK. wsflow: 24 OK (2 skips).
    `scripts/test_dev.py`: 10 OK. The three launchers are byte-identical.
- Go: this run changed no Go or launcher input. The only Go-visible input was
  the Pi bundle; the stale bundle failed `TestPiBundleCommittedIsFresh` until
  `1fea54cbc`, and that test then passed. The rest of the local
  `go test ./... -count=1` passed apart from failures that are either
  pre-existing or caused by the test environment:
  - The Forward exclusion `TestHostedConnectionSeeingALANChangeReplacesTheDaemon`.
  - The known env-only `TestClaudeLeadLocalSettingsCanonicalWorktree` and
    `TestPiLeadResolverAdapter`.
  - macOS `/private/var` path canonicalization in three `internal/mcp` render
    tests.
  - `tk`/`wbtk` badge-class expectations in
    `TestDashboardTopicTicketMoveInvalidatesTheBoard` and
    `TestWhiteboardFilePageResolvesTopicTickets`, from the dashboard CSS
    rename work.
  - The timing-flaky `TestLeadHandedToATerminalIsLetGo`, which fails 4 of 10
    runs at base.
  - All except the Forward exclusion and `TestWhiteboardFilePageResolvesTopicTickets`
    reproduced at base `f1e45292c` in a temporary worktree.
  - `internal/mcp` and `wsledger` needed more than the default 10m timeout
    locally. With `-timeout 40m`, `wsledger` passed.
- Lite review ran once, with one fresh reviewer over `677711b3a` and
  `b11facf4f`: 0 Critical, 0 Important, 7 Minor. The Minor findings are
  recorded only:
  - On Windows the mailbox-daemon test passes without proving anything, because
    the sentinel and the `go` stub are scripts.
  - The fixture's `git commit` does not set `commit.gpgsign=false`.
  - `dispose?.()` silently no-ops if `dispose` is dropped.
  - The diagnostics assertion is narrow.
  - The child-runtime test injects its own `runLauncher`.
  - The source fixture hand-unlinks the bootstrap.
  - The `os` spy list is narrower than its comment claims.
- Not performed: the Windows/AhnLab live acceptance. That covers launcher
  startup, version checks, the Pi mailbox wait, Codex Stop and daemon-spawned
  Pi worker MCP, and it should follow coordination with
  `fix/pi-spawn-key-bound-to-cwd-worktree`. It stays owner-owned. The local
  macOS tests do not prove the Windows/AhnLab behavior.


## Resolution (2026-11-14)

Phases 1 and 2 are implemented and have Results. Only the dashboard daemon executes a ws binary; the launcher, Pi waiters and version checks are Python adapters that read health or hash-bound metadata. The Windows/AhnLab live acceptance was not performed and remains owner-owned.
