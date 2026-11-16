---
title: "Electron standalone daemon start and replacement without stopping session holders"
related:
  261115-feat-werkschaft-electron-shell: "attach-only first slice and planned missing-daemon start"
  261115-feat-ws-daemon-release-auto-update: "separate user-approved release download and swap; reconcile ownership"
  261115-feat-ws-dev-daemon-accepted-build-recovery: "development accepted-build recovery and rollback follow-up"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 4bf35c6c91796d9c
sage-review-design-reviewed: dbccf1873e641d01
---

# Electron standalone daemon start and replacement

## Background

The owner requires Electron itself to start **and** replace the daemon
standalone, without a plugin session or `scripts/dev.py` starting it first
(2026-11-16). The existing shell only discovers an endpoint and connects; on
failure it asks the user to start the dashboard normally
(`agents-plugin-tool/desktop-electron/endpoint.js:8-75`,
`agents-plugin-tool/desktop-electron/connection.html:14-19`). The earlier
(1115) override decision reverses the shell's attach-only rule after v0.50.0;
(1124)=b schedules it through the Electron auto-updater.

## Decisions

- **(1196)=c:** Electron launches the daemon binary bundled with the app by
  default. A setting can select a development build for dogfooding. Neither
  path depends on an installed plugin runtime.
- **(1197)=b:** The newer numeric core wins between Electron and plugin
  daemons. If the bundled core is older, Electron attaches rather than
  replacing it. For equal cores retain the existing rule: ordinary connection
  keeps the daemon; explicit `--replace` requests one swap, regardless of build
  time or hash. A newer core is never downgraded
  (`agents-plugin-tool/internal/wsdashboard/ensure.go:40-53,89-107,163-174`).
  This monotonic core precedence prevents cross-core replace loops and avoids
  replacing a newer-minor daemon with an older one. It does **not** remove
  existing cross-minor hello refusals
  (`agents-plugin-tool/internal/wsdashboard/proxy.go:232-254`).
- **(1198)=a:** The daemon always outlives Electron; app exit and daemon
  shutdown are separate. The daemon stops only through explicit shutdown,
  including the authenticated drain for a designated replacement. On Windows,
  the current job-object breakaway fallback can leave it inside the app's job
  and kill it with the host; a phase must handle that risk, not silently claim
  the lifetime guarantee (`agents-plugin-tool/internal/wsdashboard/spawn_windows.go:12-34`).
- **(1124):** Replacement never stops, signals or restarts session holders;
  their PIDs remain the same and sessions detach and reconnect. Holders exit
  only when their sessions end. This applies to standalone replacement too.
- **Compatibility target T0** for this feature: retain existing protocol,
  endpoint, lock and store meanings and transparent old-client behavior. The
  replacement itself has no independent tier; grade each affected call and
  store path directionally, including recovery, before release. Existing
  cross-minor hello refusal is a **T3 boundary**, not evidence of transparent
  cross-minor compatibility. Any new T3 boundary requires a minor release and
  explicit owner approval; T4 must not ship
  (`ai-docs/manuals/release-compatibility.md:27-51,80-91,108-119`).

- **Development selection (technical, delegated by the coordinating lead):**
  select an existing hash-bound binary and its matching runtime contract,
  resources and skills. Selection never builds, downloads, or promotes a build
  to accepted status. Explicitly selecting a candidate authorizes its launch,
  not background accepted-build recovery. The accepted-build ticket continues
  to own promotion, retained accepted state, observation and rollback. Rejected:
  inventing a second accepted-build state machine in the shell while that
  ticket's ownership and authorization remain unsettled.

- **Updater coexistence (announced default, owner board 2026-11-16):** retain
  the separate user-approved release-download daemon updater alongside
  shell-bundled updates, derived from (867)/(868). Numeric-newer core wins;
  equal-core replacement requires an explicit swap. This ticket implements no
  downloader. Rejected: implicitly retiring the already-confirmed updater.
- **Cross-boundary replacement (announced default, owner board 2026-11-16):**
  reuse (868): one confirmation lists affected sessions and plugin-update steps
  before a bundled/dev swap across a minor/protocol boundary. Cancel attaches
  to the existing daemon. If attached hello versions cannot be discovered,
  fail closed for the automatic swap and show the reason; add no new endpoint.
  Rejected: automatic replacement silently locking out existing sessions.
- **Preserve running LAN mode (review-raised technical decision):** Electron
  ordinary connection must not reconfigure LAN. Existing Ensure treats an
  equal-core LAN-mode change as a replacement request (`ensure.go:163-174`).
  No inherited unrelated dashboard manifest may convert attach into a drain.
  Merely omitting that manifest is insufficient: daemonStartLAN can fall back
  to a different global LAN value after a failed probe (`cmd/ws-mcp/proxy.go:341-349`).
  The Phase 2/3 handoff must preserve a concurrently discovered equal-core
  daemon's LAN mode even after transient health failure; pin this race in a
  focused test before connecting the shell to a replacement-capable starter.
  If satisfying it needs another cross-module interface, obtain approval first.
  Rejected: claiming an early health probe alone guarantees mode preservation.

- **Missing-only start (technical decision approved by coordinating lead,
  2026-11-16):** after REST discovery finds no live daemon, use the existing
  detached `dashboard --port ...` equivalent NEW-daemon entry. StartDaemon's
  lock arbitrates concurrent starts without a drain, unlike daemon-start's
  possible LAN-probe race. Controlled REST-authenticated replacement remains
  Phase 3. Phase 2 does not take the launcher's separate launcher-start.lock;
  the coordinating lead accepted a rare losing launch there. For Phase 3, use
  Exec1's merged start-lock semantics if participating is cheap; evaluate only
  after that implementation lands rather than inventing a parallel lock
  protocol or depending on an unpublished format.

- **Windows strict lifetime, (1203)=a (owner 2026-11-16 11:14):** add the
  Windows-only `daemon-start --require-independent-lifetime` flag. Only Electron
  passes it. Breakaway failure fails explicitly with no in-job retry; ordinary
  plugin callers retain today's retry. T0 additive: old callers are unchanged,
  and an old selected binary rejects the unknown flag before running Ensure or
  draining anything. Preserve the strict-preflight-before-drain constraint.
- **Updater and cross-boundary reaffirmation, (1201)=ok and (1202)=ok
  (owner 2026-11-16 11:14):** retain the separate downloader ticket without
  implementing it here; show affected sessions and plugin-update guidance once
  for a cross-minor/protocol swap, cancel attaches existing, and unknown session
  versions prevent automatic replacement with an explicit reason.
- **Current REST evidence (technical, source validated):** `/health` exposes
  daemon identity only (`wsdashboard/daemon.go`, HealthInfo/handleHealth);
  `/agents` lists runner conversations without attached launcher hello versions
  (`wsagent/http.go`, Runner.List). ProxyHello has protocol/version fields but
  the live proxy registry retains only connection/session handles and a count
  (`wsdashboard/proxy.go`, serveProxyConn/ProxySessions). Therefore current
  interfaces cannot enumerate affected client versions. Cross-boundary automatic
  replacement must keep the existing connection and explain the unknown-version
  reason plus plugin-update guidance, not invent a list or add an endpoint.

## Prior parking and current coordination (2026-11-16)

The coordinating lead parked this ticket after Phase 2, then resumed Phase 3
once the perf-daemon candidate reached its approved-swap/after-measurement gate.
The owner has now confirmed the Windows interface above. Keep this ticket open
and retain `impl/develop/cape-barn-hence` and its leased worktree without merging
or releasing. Before touching cmd/ws-mcp daemon-start/staging or dashboard spawn
files, coordinate exact paths with the lead because Exec1 is editing the startup
path in parallel. The lead released cmd/ws-mcp, Ensure and spawn edits after
Exec1 merged as 74924b4c0 and explicitly authorized merging/rebasing this feature
onto that baseline before Go work. Merge b0e5ee290 brings current develop
(including 74924b4c0) into this retained feature checkout; no shared-root branch
was changed. The earlier separate-Go-branch hold is superseded by that explicit
integration instruction. The approved sequencing seam is the opt-in Ensure
SpawnBeforeShutdown bool (not a callback/prestarted-object abstraction), with
PreserveRunningLAN applied within Ensure's live health observation. A strict
spawn fails on breakaway error without plain retry; its successor lock wait is
bounded and covers the drain. Default plugin behavior stays unchanged. No extra
executable probe or starter is allowed. No real Windows/macOS acceptance has
been claimed.

Preflight means that the OS accepted the independently detached successor
creation before authenticated drain. It does not promise that the child has
parsed every argument, become healthy, or reached the lock while its predecessor
owns it. A later initialization/lock/health failure is the separately required
post-preflight failure mode and must report unavailable without claiming the
predecessor survived. Do not turn the shell review's stronger lock-ready
assumption into a new unpublished readiness protocol, executable probe, or a
permanent production replacement refusal; implement the approved detachment
ordering and its explicit late-failure handling.

## Constraints

- **REST-only inspection (owner 2026-11-16 09:48, relayed by coordinating lead):**
  every check of an already-running daemon (health, version, core, compatibility,
  LAN mode) uses its REST API. Read candidate metadata and runtime.json as files.
  Never execute ws-mcp for --version, hello, probes or compatibility inspection.
  Electron executes a daemon entry only after deciding a NEW daemon must start;
  assert zero executable launches on healthy attach/inspection paths in tests.
- **Hostable selected package (dogfood #12, coordinating lead 2026-11-16):**
  validate the parent package of selected WS_RSRC_ROOT contains a readable
  `.claude-plugin/plugin.json` with supported name ws or wsflow, matching
  `internal/wsagent/lead_plugins.go:16-53`. Apply to both bundled and selected
  dev input, and fail closed for missing/invalid/Pi-only package metadata.
  Snapshot the complete prepared package, not only rsrc/skills and the descriptor:
  validate and retain manifest-referenced `bin/ws-mcp-launcher.py`, the config
  manifests, and `claude-agents/` entries so Claude --plugin-dir can use the
  staged root. The daemon start itself still requires no Python execution.
- **Independent replacement ordering (coordinating lead 2026-11-16):** prove
  strict detachment before draining a healthy predecessor, through preflight or
  an already-independent starter. Test failed strict preflight leaves the old
  daemon serving. A spawn failure after drain is a separate tested failure
  mode, never described as preserving the predecessor.
- **Out-of-bundle staging (coordinating lead 2026-11-16):** packaged
  app.getAppPath()/daemon is a source, not the running location. Before launch,
  snapshot binary, metadata, runtime contract, plugin metadata, resources and
  skills together to a versioned per-user directory outside the replaceable
  Electron bundle. Retain old staged versions without automatic pruning;
  explicit manual cleanup is allowed only after explicit daemon shutdown and
  when no process uses that version. Clean only a failed/incomplete staging
  attempt owned by this operation. This avoids removing a running daemon's
  resources or locking a shell self-update.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- All work, ticket promotion included, stays in the leased full worktree on
  `impl/develop/cape-barn-hence`. The coordinating lead owns integration; never
  change the shared root checkout's branch, commit there, merge or push.
- Go builds/tests run only through sys_lock on w5090. No local Windows Go
  test/build or ad-hoc test executables: corporate security flagged a local
  test-built fake codex.exe. Node checks may run locally without spawning such
  executables. Before any local Windows binary acceptance, ask the coordinating
  lead with the exact command; cross-compile on w5090 and fetch only under
  `D:/Temp/exec3-electron/`, never the default Windows TEMP directory.
  Use scoped checks only. Remove temporary artifacts after use; stop when D:
  has less than 20 GB free.
- **Approved Windows implementation gate:** owner (1203)=a authorizes the
  additive strict-lifetime CLI interface, not an unsafe fallback or a claim that
  Windows acceptance has already passed. Coordinate the shared Go-file edit
  boundary with the lead first. Until the implementation lands, retain the
  explicit Windows start/swap refusal; healthy read-only attach stays available.
  Strict-launch failure must be detected before draining a healthy predecessor,
  not discovered only by Ensure's post-drain Spawn call. Unsupported selected
  binaries fail before drain. No real Windows executable acceptance is allowed
  without the lead's prior approval of its exact command.
- Lifecycle tests use an isolated absolute WS_CACHE_HOME and non-shared port;
  never start, replace, stop or shut down the shared daemon on port 29417.
- No new release downloader, accepted-build registry, recovery promotion,
  rollback authorization, MCP tool, hello field or shared store schema in this
  ticket. Do not claim reconnect repairs an existing protocol/minor refusal.
- Packaged Windows/macOS acceptance is a real completion gate. Missing access to
  a platform is reported as an unverified boundary, not a passing simulation.

## Compatibility Evidence

This is a source-backed T0 target, to be rechecked against the implementation;
no new T3 boundary is authorized.

| Direction / surface | Existing reader, writer and recovery evidence | Required result |
|---|---|---|
| New shell starts a daemon; old launcher reconnects | Existing daemon writes the same endpoint fields atomically (`internal/wsdashboard/endpoint.go:48-94`); both Go endpoint readers and the shell consume identity without rewriting it. Shutdown is authenticated with the retained token (`client.go:53-69`). | Preserve endpoint, lock, health and authenticated-drain meanings. No endpoint migration or new field required. |
| New starter / old starter | Existing `daemon-start` parses flags before constructing the server or invoking Ensure (`cmd/ws-mcp/proxy.go:365-374`). New optional strict lifetime behavior must leave the absent-flag path unchanged; an old selected binary must reject the unknown flag before any drain. | T0 additive CLI behavior for existing callers; unsupported strict-lifetime candidates fail explicitly, never fall back to unsafe launch. |
| Old compatible session calls new daemon after a swap | Hello still checks proxy protocol and same major.minor (`internal/wsdashboard/proxy.go:232-254`). Launcher replays initialization and unsent calls, not already-sent requests (`agents-plugin/bin/ws-mcp-launcher.py:1636-1662`). | No MCP tool or argument removal, protocol change or handler-store change. Report uncertain in-flight outcomes; do not call reconnect a rollback. |
| Old-minor session meets new-minor daemon | Existing hello rejects before handing the session to the handler (`internal/wsdashboard/proxy.go:232-254`); no handler/store write occurs and reopening cannot repair the refusal. | Existing T3 boundary remains; warning/confirmation or refusal must precede a shell-triggered cross-boundary swap. |
| Old daemon / new daemon shared holder records | Shutdown detaches only (`internal/wsdashboard/daemon.go:653-695`); Manager.Close closes attach connections without killing holders, and Scan reattaches from the same descriptors (`internal/wsholder/manager.go:235-264`). | No descriptor/store format change; preserve holder and child PIDs across compatible replacement. Reopening the new daemon reads the same retained records. |
| New shell selection and staged provenance / old shell or launcher | New shell-only files must use distinct app-user-data names; existing launchers do not read or write that namespace. Binary metadata remains schema 1 with a SHA256 binding (`agents-plugin/bin/ws-mcp-launcher.py:196-240`); optional protocol/inventories follow the existing gate (`:730-752`). | No shared accepted-build writer or config-manifest field. Old readers neither overwrite nor lose these private files; new reopening validates retained bytes again. |

Go paths above are relative to `agents-plugin-tool/`.

## Prior Decisions

- 481f0f765 (2026-11-16, commit): "Record owner decisions for bundled daemon selection, core precedence, and independent lifetime." — bearing: supports
- b820cd188 (2026-11-15, commit): "Record Electron daemon override decision for after v0.50.0." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Electron shell, launcher, daemon and compatibility paths named in the phases: `agents-plugin-tool/desktop-electron/`, `agents-plugin/bin/ws-mcp-launcher.py`, `agents-plugin-tool/cmd/ws-mcp/proxy.go`, `agents-plugin-tool/internal/wsdashboard/` |
| scope.surface | cross-module | Electron package, plugin launcher and Go daemon handoff are coordinated; no single-module change |
| scope.new_public_symbol | yes | Owner (1203)=a approved Windows opt-in `daemon-start --require-independent-lifetime`; coordinate Go edit paths with the lead and preserve existing callers' defaults. |
| scope.new_type_contract | yes | Shell-local development selection records an existing binary and matching resource/skills/contract root; no shared ws manifest or protocol field. |
| scope.test_surface | existing | Existing Electron tests include `agents-plugin-tool/desktop-electron/test/endpoint.test.js` and `main.test.js`; daemon Ensure/daemon-start tests exist in their Go packages. |
| complexity.reuse_points | confirmed | Existing `daemon-start`/`wsdashboard.Ensure` supports absent-daemon startup, authenticated drain, core precedence and bounded successor verification (`agents-plugin-tool/cmd/ws-mcp/proxy.go:354-420`, `agents-plugin-tool/internal/wsdashboard/ensure.go:89-160`). |
| complexity.side_effect_risk | high | Starts and drains processes, changes the daemon answering a shared cache root, and must preserve holder lifetimes. |
| risk.correctness | high | Incorrect precedence or handoff can cause replacement loops, stale endpoint use, or loss of live daemon availability. |
| risk.fit | moderate | The feature extends the Electron shell and existing daemon lifecycle, but spans distinct package/module responsibilities. |
| risk.test | high | Required coverage includes packaged Windows/macOS behavior, concurrent starts, reconnect, held-process continuity and app-exit survival. |
| risk.security_or_contract | high | Binary provenance, resource/skills environment, authenticated shutdown, compatibility boundaries and cross-minor refusal all constrain the contract. |

## Phases

All lifecycle tests use isolated `WS_CACHE_HOME` and isolated daemons, never
replace the shared daemon.

### Phase 1: Provenance and compatibility specification

Specify bundled and selected-dev binary provenance, selection and validation;
port the relevant Python launcher compatibility gate (hash-bound metadata,
version, protocol and capabilities) without requiring Python or an installed
plugin. Specify how the packaged app supplies `WS_RSRC_ROOT` and
`WS_SKILLS_ROOT`, including dev selection and failure reporting
(`agents-plugin/bin/ws-mcp-launcher.py:134-150,730-755,841-863`). Trace
old/new call and store directions and recovery against the T0 target; stop for
approval if a higher tier is needed. Reconcile the open updater and recovery
questions before implementing affected paths. Deliver the validation module and
focused fixture tests without executing a binary; do not wire a pending Windows
CLI interface in this phase. Specify package input layout and the shell-local
setting so the next phase consumes a concrete contract.

### Result (b927532f77) - 2026-11-16

- Added read-only `desktop-electron/daemon-selection.js` with non-executable Node fixtures. It checks schema-1 metadata adjacent to the selected binary against its SHA256 bytes, checks the producer's numeric version (bundled: exact core; selected dev: same major.minor), optional MCP protocol, and optional tools/commands inventories (missing inventories remain unknown, `runtime_capabilities.match: exact` rejects extras). It rejects missing/malformed inputs and verifies the resource and skills manifests, their listed file hashes and complete file inventories before returning launch inputs. The lite review found an unlisted-skill bypass; e86c02f8ec added inventory traversal and the unlisted-file regression.
- **Phase 2 packaging input contract:** prepare one platform-specific directory `daemon/` inside the packaged app's unpacked application root (`app.getAppPath()/daemon`): `ws-mcp.exe` on Windows or `ws-mcp` on macOS, adjacent `<binary>.metadata.json`, `runtime.json`, `rsrc/manifest.json` plus its tree, and `skills/manifest.json` plus its tree. The package builder still needs to copy this preassembled input; this phase does not claim the current archives contain it. No installed plugin/Python runtime is an implicit source.
- **Shell-local dev setting:** absence of `app.getPath('userData')/daemon-selection.json` selects bundled. An explicitly written JSON file `{ "schema_version": 1, "root": "<absolute directory>" }` selects a preexisting, identically laid-out dev tree. Invalid/missing selected input fails closed without bundled fallback, accepted-build promotion, auto-recovery, downloading or rollback. The selector performs no writes. It returns `{ source, binary, contract, metadata, env }`; the later starter must pass its returned `env.WS_RSRC_ROOT = <selected root>/rsrc` and `env.WS_SKILLS_ROOT = <selected root>/skills` to the daemon-start child rather than inheriting unrelated plugin roots. These are daemon launch inputs, not a rewrite of connected plugin sessions' resource paths. Shell UI and lifecycle are deferred.
- **Compatibility trace (this slice): T0 target, not a cross-minor guarantee.** Old launcher/new shell: `endpoint.js:20-42,76-82` only reads endpoint identity; selector reads app-private files, writes neither endpoint nor shared stores. New shell/old daemon: no selection path calls Ensure, modifies LAN or opens `/mcp`, so existing daemon stays untouched. New daemon/old launcher after a future swap: `wsdashboard/endpoint.go:48-94` still reads/writes the same schema-1 endpoint atomically; `wsdashboard/proxy.go:232-254` still refuses old-minor/protocol hellos before handler writes (existing T3; reopening cannot repair refusal). Old/new holder data: `wsdashboard/daemon.go:653-695` detaches while `wsholder/manager.go:235-264` closes connections and reattaches from retained descriptors; this slice does not change those records. `ws-mcp-launcher.py:196-240,730-752` remains the old producer/reader of binary metadata; this shell-only reader neither overwrites metadata nor adds a shared manifest field and revalidates retained bytes on every selection. Reconnect of an already-sent request is not recovery (`ws-mcp-launcher.py:1636-1662`). Each future replacement still needs directional handler/store and cross-minor confirmation evidence before release; no new T3 is approved here.
- Verification: `TMPDIR=TEMP=TMP=D:/Temp/exec3-daemon/phase1-tests` with `sys_lock.sh run exec3-daemon 3 -- node --test agents-plugin-tool/desktop-electron/test/daemon-selection.test.js`: 4 pass, 0 fail after review fix; `git diff --check` passed. No executable fixture, Go test, daemon lifecycle, package build or Windows/macOS acceptance run. Windows independent lifetime/strict CLI flag, updater download and accepted-build recovery remain separate future gates.

### Phase 2: Standalone missing-daemon start

Start the selected binary when no daemon is available, using the existing
single-instance daemon lock and endpoint/health verification; handle races,
hung daemons and failures without killing holders. For this missing-only slice,
the existing `dashboard` entry is an equivalent NEW-daemon start under the
owner's REST-only contract: a detached launch takes the existing StartDaemon
lock and never requests a drain. Do not invoke replacement-capable Ensure just
to discover whether a concurrent daemon exists; it can turn a failed early LAN
probe into an unintended equal-core mode-change drain. Healthy REST discovery
attaches without execution; a concurrent lock winner is verified and retained.
Controlled replacement remains Phase 3. Source references:
(`agents-plugin-tool/cmd/ws-mcp/proxy.go:354-420`,
`agents-plugin-tool/internal/wsdashboard/ensure.go:89-160`). Ensure app exit
does not stop the daemon, including a verified solution or explicit failure
for Windows job-object breakaway failure, rather than the current unsafe
fallback. Test first start, retry and app exit in isolation. The Windows
strict-lifetime step is separately gated by owner confirmation above; before
it lands, verify the Windows refusal path without spawning any executable,
and report Windows start/survival acceptance as pending. Do not claim the
Windows slice or this ticket complete on portable/mock evidence. Include the
new REST-only, hostable-package validation and out-of-bundle staging constraints
above. Package only a supplied, file-validated daemon input; never run a binary
to produce metadata or build/download it on first launch. Verify with scoped
Node fixtures and injected spawns only until platform acceptance is authorized.

### Result (edf7e13fca) - 2026-11-16

- Strengthened **both bundled and selected-dev** validation beyond Phase 1: a readable `.claude-plugin/plugin.json` must name `ws` or `wsflow` and agree with `runtime.json`; its referenced launcher, both config manifests and all declared Claude agents must be readable files inside the selected package. Missing, invalid, Pi-only or incomplete roots fail closed. Phase 1's frozen Result is unchanged.
- `scripts/build.cjs` now requires a supplied absolute `WERKSCHAFT_DAEMON_INPUT` (or injected `daemonInput`) and copies its complete validated directory into the offline app package. It never builds, downloads or executes the supplied binary. On missing-daemon macOS start, `daemon-stage.js` copies and revalidates the entire package to a uniquely versioned `userData/daemon-stages/` snapshot outside the replaceable app, including the binary, metadata, runtime contract, launcher, manifests, resources, skills and Claude agents. It rejects links, in-bundle canonical paths and changed copies; it removes only an owned failed partial, never an older complete stage. An untrusted version suffix cannot escape the stage container.
- Main-process connection first discovers via endpoint plus REST `/health` only. Healthy attach does not select a candidate, stage, spawn or change LAN. On an unavailable/uncertain macOS endpoint it stages the candidate, checks REST again, then detaches the `dashboard` entry. Go `StartDaemon` arbitrates the lock; no shell call requests Ensure, shutdown, drain or mode change. Bounded REST polling attaches a concurrent winner or reports failure without terminating a process. A launch failure and an uncertain/hung lock are reported for Retry. The daemon child uses staged roots for `WS_RSRC_ROOT`, `WS_SKILLS_ROOT`, config manifests, runtime binary/directory and plugin root; cache/config selection stays intact while hosted-session identity and inherited project/dev-asset paths are removed. Shell quit/update/renderer trust and titlebar behavior remain unchanged. Windows healthy attach stays available; missing/uncertain start explicitly refuses before any binary launch because independent job lifetime is not approved or proven. Linux missing start is unsupported.
- Review: R1 lite raised three Important findings (canonical symlink escape, concurrency lock-winner simulation, app-quit test using only a healthy stub); fix `14f4217726` addresses all three, and fixes-only R2 marked all fixed. No replacement-capable CLI flag, Phase 3 swap, accepted-build promotion, downloader, endpoint/store/hello change or shared daemon action was introduced. Compatibility target remains shell-private T0; cross-minor hello's existing T3 refusal is unchanged.
- Verification: `TMPDIR=TEMP=TMP=D:/Temp/exec3-daemon`, `sys_lock.sh run exec3-daemon 3 -- node --test` scoped to `daemon-selection.test.js`, `daemon-controller.test.js`, `daemon-stage.test.js`, `main.test.js`: 42 pass after initial corrections; after review fixes, focused controller/stage/main: 39 pass, 0 fail; `git diff --check` passed. All daemon bytes in fixtures were non-executable and all launches injected; the REST attach test bound only an ephemeral loopback port, not 29417. **No real macOS or Windows binary start/lifetime or packaged acceptance was run**. Windows strict-lifetime owner gate, macOS packaged/real survival, cross-version replacement and holder continuity remain unverified future boundaries; the ticket stays in `ready/` for Phases 3–4.

#### Edition (5d9c14ef01) - 2026-11-16

- Corrected the Phase 2 Claude package selector to validate the MCP launcher and config references under `mcpServers[plugin.name]`: the real wsflow manifest declares `mcpServers.wsflow`, not `mcpServers.ws`. An unrelated ws server no longer stands in for wsflow. Non-executable bundled and selected-dev wsflow fixtures now pass with matching runtime contracts; the existing ws fixture remains passing.
- Verification: `TMPDIR=TEMP=TMP=D:/Temp/exec3-daemon` with `sys_lock.sh run exec3-daemon 3 -- node --test agents-plugin-tool/desktop-electron/test/daemon-selection.test.js`: 6 pass, 0 fail; `git diff --check` passed. No real binary, Go test, lifecycle, Phase 3 replacement or platform acceptance run. Windows strict-lifetime gate and Phase 3–4 boundaries remain unchanged.

### Phase 3: Controlled replacement and reconnect

Apply newer-core precedence and the existing explicit equal-core rule; verify
which daemon actually answers after handoff. Test older, equal and newer cores,
concurrent plugin starts, failed drain and reconnect. Prove every holder retains
its PID and each compatible session reconnects; audit that no shell path kills,
signals or restarts holders. Surface incompatibility rather than claiming
reconnect overrides the existing hello gate. Unblock Windows via owner-approved
(1203)=a only after coordinating the Go-file boundary. Add a strict detached
preflight before authenticated drain; a preflight error must leave the current
daemon healthy. A successfully detached successor may wait on the existing
singleton lock before the old daemon is drained, with a wait budget that covers
the drain. A later successor failure is reported as unavailable, not as a claim
that the predecessor survived. Keep default plugin spawning unchanged.

Use only REST for running-daemon version/core/LAN/health/compatibility checks;
read candidate identity from metadata files and execute only to start a NEW
daemon. Equal/newer ordinary attach must execute zero binaries; explicit
same-core replacement is consumed once, and raced newer winners are retained.
If current interfaces do not reveal affected session versions, refuse automatic
cross-boundary replacement with an actionable reason and keep the old connection.
Repeat this guard at Ensure's actual healthy-daemon replacement decision, not
only the shell's earlier REST check: a compatible initial daemon can race with
a different cross-minor winner before the starter runs. Test that raced winner
is not drained. Apply LAN preservation at the same fresh observation point.
Keep staging outside the replaceable app. Follow Exec1's merged start-lock
semantics; close the accepted rare Electron-vs-launcher race if taking the same
lock is cheap, otherwise report its exact boundary as Forward. Do not implement
an unpublished lock format or edit Exec1's code before its merge ping.

The shell candidate below predates the released Go boundary. Current work may
complete strict lifetime/Ensure/start-lock integration on the refreshed feature
base. First fix the shell candidate's missing daemon-version.js packaging copy
and pin the packaged source's helper closure with a Node-only regression.
Evaluate the published launcher-start.lock byte-zero semantics directly; use
the existing Go flock dependency when cheap, but never add a Node native addon
or helper executable just to acquire it. Report any remaining single authorized
starter race precisely instead of claiming it was eliminated. Write a Phase 3
Result only when its integration and holder/reconnect verification boundary is
actually satisfied; real packaged Windows/macOS acceptance remains Phase 4.

#### Phase 3 shell candidate (5b07ad43f3) - 2026-11-16 (NOT A RESULT)

- Shell REST inspection now validates endpoint identity against `/health` and re-reads it after the request; discovery still returns only the URL, and only the controlled shutdown path reads a validated bearer credential. Direct loopback requests have bounded bodies/timeouts, no proxy and no redirects. The native Connection menu exposes a one-shot same-core replacement request without renderer IPC.
- Numeric candidate cores come only from the already file-validated selection; ordinary equal/newer attachment launches nothing. A cross-minor request keeps the incumbent and shows unknown-client-version/plugin-update guidance (current REST has no connected hello list). Staging and another REST inspection precede any handoff. Injected macOS preflight/lock-wait/drain/successor tests model failed preflight (no drain), failed drain, an older winner, a different-content winner and late successor failure. Actual macOS production replacement **fails closed before launch or drain**: the existing detached `dashboard` entry gives no signal that it reached the lock wait; a spawn event alone is not strict preflight. Windows missing start and replacement also remain fail-closed until the separately approved strict-lifetime Go implementation lands. Healthy attachments remain available on both platforms; no binary was executed.
- R1 lite found a Critical macOS drain-after-spawn-event gap. Commit 5b07ad43f3 removed that unsafe production path; fixes-only R2 verdict: clean. The first medium R1 invocation timed out without verdict; the completed small-tier R1 gave the finding. Verification: focused Node scope (`endpoint`, `daemon-controller`, `main`, `daemon-selection`, `daemon-stage`) through `sys_lock.sh run exec3-electron 3`: **58 pass, 0 fail**, with injected launches and OS-assigned loopback fake REST only; `git diff --check` passed. This is not packaged or live-daemon acceptance.
- **Forward / held dependency:** after Exec1 startup merges, the lead starts the Go slice from fresh develop, not this branch: approved Windows `daemon-start --require-independent-lifetime`, strict preflight before authenticated drain, opt-in `SpawnBeforeShutdown`, actual Ensure decision-point cross-minor/protocol and `PreserveRunningLAN` race guards, with bounded successor lock wait. Integrate a verifiable independent successor preflight for macOS rather than treating Node's child `spawn` event as proof. Assess the merged Exec1 launcher-start lock only then; Phase 2 intentionally did not join it, so the rare Electron-vs-launcher start race remains open. No invented lock format or launcher edit here. Real holder PID continuity, reconnect/in-flight outcomes and Windows/macOS packaged start/survival/replacement remain unverified; Phases 3 and 4 are incomplete, and the ticket stays ready/open. No Go source/test or real executable was used in this candidate.

### Result (6a592c037) - 2026-11-16

- Packaged source now includes `daemon-version.js`; an injected package/archive check resolves the complete relative-JS helper closure from the staged source without running Electron or a daemon. REST inspection fingerprints the endpoint token **before** health and rejects same-PID/port token rotation during the response and before authenticated shutdown; no token enters the returned discovery object.
- macOS replacement now uses OS-accepted detached `dashboard` process creation as the approved preflight, then an authenticated drain with bounded successor lock wait. This is not proof that the child parsed flags or reached the lock; a late failure reports unavailable. Windows-only `daemon-start --require-independent-lifetime` rejects breakaway failure without the unflagged plugin's in-job fallback, uses opt-in Ensure `SpawnBeforeShutdown`, and waits for strict Go completion before verifying the answering daemon by REST. Strict Go starters hold the persistent launcher-start.lock through the final health decision, not dashboard.lock; plugin callers already hold the launcher lock and are unchanged. The one Electron starter executable launched before its Go lock acquisition may still lose to a launcher; neither shell claims zero losing starter executions nor adds a Node native lock.
- Ensure's **fresh** REST health observation applies `PreserveRunningLAN` and refuses unknown-client-version cross-minor replacement, even for a raced winner; ordinary same/newer attach runs no ws executable. Equal-core explicit swap is one-shot, numeric newer wins, and unknown attached versions cause actionable cross-minor refusal. A failed strict preflight leaves the old daemon healthy; accepted drain followed by missing successor is unavailable, never a predecessor-survival promise. macOS failed-drain inspection and post-handoff verification also reject raced incompatible winners. Staged packages remain outside the app and are not pruned; UI native action, renderer/navigation authority and app-quit semantics remain unchanged.
- Review: the first medium R1 invocation timed out without a verdict; the completed small-tier R1 found an Important failed-drain raced cross-minor attachment. That catch and the successor wait now reject incompatible racers; fixes-only R2 marked the finding fixed, with no new observation.
- Verification: `TMPDIR=TEMP=TMP=D:/Temp/exec3-electron` with `sys_lock.sh run exec3-electron 3 -- node --test` on `endpoint`, `daemon-controller`, `daemon-stage`, `main`: **56 pass, 0 fail**, only injected exec and OS-assigned fake loopback REST. Remote w5090 via sys_lock: `go test ./internal/wsdashboard/ -run '^(TestEnsure|TestKeepRunning|TestProxyCompatibleClientReconnectsAfterDrain|TestProxyRefusedHelloGetsAReasonLine|TestProxyDrainFinishesCallsInFlightAndCancelsLongWaits|TestDaemonRestartReattachesToARunningHolder)$' -count=1` passed; focused `cmd/ws-mcp` daemon-start tests passed; `internal/wsholder/ -run '^TestChildSurvivesADaemonRestart$' -count=1` passed with a real Linux representative holder/child PID; Windows focused test binaries cross-compiled remotely only, then deleted without execution. The proxy test opens a real Go loopback transport after drain with a compatible hello and new request; it does **not** run the Python launcher or replay in-flight work. `git diff --check` passed. No local Go test/build, real Windows/macOS app/daemon/launcher execution, shared daemon operation or packaged platform acceptance occurred.
- Directional compatibility stays T0 for this additive CLI opt-in and shell-only selection/staging: `cmd/ws-mcp/proxy.go` parses the new flag before `Ensure` or a drain; old callers omit it, and an old selected binary rejects the unknown flag before starting. `wsdashboard/endpoint.go:73-94` still reads/writes the same endpoint, and `client.go:53-69` still authenticates shutdown with its token. `wsdashboard/proxy.go:232-254` still refuses a different protocol/minor before handler writes (existing T3, not cured by reconnect), while `wsholder/manager.go:237-264` closes attach connections and scans retained descriptors without new holder-store fields; reopening the new daemon reads the same descriptors. Shell-private stage/selection files have no old shared-store writer, so no old-client overwrite or recovery migration is introduced. The new proxy test proves compatible re-hello/new calls, not already-sent request recovery. **Forward to Phase 4:** real packaged Windows job/breakaway refusal and app-quit survival, macOS detached successor lifetime, actual Python/Pi/Claude client reconnect/in-flight outcomes, resource availability and platform holder continuity remain unverified; no synthetic spawn is presented as that evidence.

#### Edition (3d791f606) - 2026-11-16

- Corrected Phase 3 winner attachment: a REST-verified numerically newer daemon is attachable even when another actor won across a minor/major boundary. Unknown connected client versions still prohibit **initiating** a cross-boundary drain, not observing a newer winner. Older/unidentified raced winners remain refused. Staging, failed macOS drain, failed Windows strict starter and successor-wait paths now share that rule; successor detection uses endpoint generation (including token fingerprint), not only PID/start time. Guidance notes that connected plugins may need updates after a newer winner; no further spawn/drain is attempted for that winner.
- Corrected the Phase 3 remote Go verification citation: the previously printed anchored regex `^(TestEnsure|TestKeepRunning|...)$` did not select suffixed `TestEnsure*` functions. The coordinating lead independently reran the correctly selected **remote w5090** gate `go test` with `-run '^(TestEnsure.*|TestKeepRunning.*|TestProxyCompatibleClientReconnectsAfterDrain|TestProxyRefusedHelloGetsAReasonLine|TestProxyDrainFinishesCallsInFlightAndCancelsLongWaits|TestDaemonRestartReattachesToARunningHolder)$' -count=1 -v` via sys_lock: all passed, including all six `TestEnsureElectronPreflightAndFreshHealth` subtests and representative Linux holder restart/proxy reconnect (2.003s). This replacement worker made no Go edits and did not rerun Go tests.
- Verification after the follow-up: `TMPDIR=TEMP=TMP=D:/Temp/exec3-electron` with `sys_lock.sh run exec3-electron 3 -- node --test` scoped to `daemon-controller.test.js`, `endpoint.test.js`, `main.test.js`: **56 pass, 0 fail**. Injected spawn and assigned-port fake REST cover newer cross-major winner, older/unidentified refusals, Windows-starter failure, failed drain, staging race and same-PID/start-time token rotation. Narrow independent read-only lite review: clean; `git diff --check` passed. No Windows/macOS packaged or actual client/holder acceptance was run; Phase 4 remains open.

### Phase 4: Packaged acceptance

On packaged Windows and macOS builds, verify clean-machine standalone start,
bundled and selected-dev provenance, replacement and newer-core attach,
resource/skills availability, holder continuity, and daemon survival after app
exit. Verify explicit shutdown separately. Update/recovery integration here
means bundled-vs-dev selection authorization, newer-core coexistence and
isolation from accepted-build state; do not implement or require either
separate ticket's downloader, promotion or rollback operation. Windows real
start/survival acceptance awaits the separately approved strict-lifetime step;
missing Windows or macOS evidence leaves Phase 4 and ticket closure pending.
Document exact results and limitations before closing the ticket.
