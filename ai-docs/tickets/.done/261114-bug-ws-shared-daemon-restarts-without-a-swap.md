---
title: "Shared dashboard daemon restarts without an explicit swap"
related:
  261115-feat-ws-dev-daemon-accepted-build-recovery: follow-up
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 26de06ba1713a053
sage-review-completeness-reviewed: 26de06ba1713a053
completed: 2026-11-15
---

# Shared dashboard daemon restarts without an explicit swap

## Background

Former Phases 2–3 moved to `261115-feat-ws-dev-daemon-accepted-build-recovery`; this ticket implements Phase 1 only.

Dogfood surprise, 2026-11-13. The shared dashboard daemon of this checkout
(`~/.cache/ws@kang-sw-devenv/dashboard`, port 29417) was replaced twice
without anyone running `python3 scripts/dev.py rerun daemon`:

- 15:04:15Z: pid 98387 -> 63011. `dashboard.log` was recreated, so the old
  daemon's log was lost.
- 15:19:51Z: pid 63011 -> 8932, build `content_hash=e7c6e270d283`. A later
  explicit swap from develop `1ed33f98d` reported "running daemon has
  identical content", so the auto restart had already built the root
  checkout's current source.

Every lead and worker involved (MAIN, UX, EXEC workers) states it ran no
daemon start or swap command. Each restart came 1-3 minutes after a commit
and a `go test` finished in the root checkout.

Impact: a MAIN `whiteboard_update` carrying about 74 KB of panels failed
twice with "dashboard daemon not reachable; the request did not complete"
(`agents-plugin/bin/ws-mcp-launcher.py:1639`, `:1677`) at about 15:03Z,
just before the first restart. Small updates sent at the same time
succeeded. Live sessions also lose their stream for the restart window.

## Hypotheses to check

1. The launcher (local-devenv source build) sees source or metadata identity
   newer than the running daemon and replaces it on its own, for example
   when an MCP session reconnects after the root's source changes.
2. A test leaks onto the shared state directory (`WS_CACHE_HOME` unset in
   some test) and starts or stops the real daemon.
3. The large-request failure is independent: a size or timeout limit on the
   launcher -> daemon proxy path. Reproduce it on a stable daemon before
   ruling this out.

The answer decides whether this is a bug (an unannounced replacement, or a
test touching shared state) or intended behaviour that needs a notice. The
project's rule is that only the EXEC lead swaps the shared daemon, with mail
to the peer leads before and after.

## Evidence (2026-11-14, EXEC lead)

One unannounced replacement was traced end to end with `ps`. It supports
hypothesis 1.

- 01:22:41 KST: holder 4809 starts a hosted `pi` (4846), and its
  `ws-mcp-launcher.py serve --stdio` (5051) starts at 01:22:42.
- 01:22:43: a new shared daemon, 5776, appears (parent 1). It runs
  `ws-mcp-dashboard-f493f998ed0c90bd... dashboard --port 29417 --build-time
  1794586962277`. That build time is a fresh local-devenv build, made at
  that moment (VERIFIED: `dev.py rerun daemon` later reported "daemon before:
  ... build_time=1794586962277 content_hash=f493f998ed0c").
- The previous daemon (93316) is gone.
- The EXEC lead had just merged three branches into the root checkout's
  `develop` between about 01:21 and 01:22 (ef7940b8a, c49ae5a11,
  3ee1d448b), so the root's source changed seconds before (INFERRED link:
  the launcher saw newer source identity and rebuilt and replaced the
  daemon).
- Side effect: 5776's environment had PATH starting with
  `/Users/kang-sw/devenv/agents-plugin-pi/node_modules/.bin` plus npm
  run-script entries, inherited through the hosted Pi's launcher. So the
  daemon resolved the repo's dev Pi 0.84.4, and `pi.spawn` and `pi.stop`
  refused for every session ("pi 0.84.4 is installed; Pi subagents need pi
  1.0 or later"; UI/UX EXEC saw it on `pi.stop`). Sibling ticket:
  `261114-bug-ws-pi-version-probe-never-refreshes`.
- Fixed by `python3 scripts/dev.py rerun daemon` from a clean shell (pid
  12261).

## Investigation evidence - 2026-11-14 (EXEC 3)

This addition records evidence only. It changes no runtime, replacement
policy, ticket status, or implementation authority. Source coordinates below
refer to `9854ff350fa1b0eccfb5c71649c5190c405925b9`, not an assumed live build.

### Verified source mechanism

- **VERIFIED:** `agents-plugin/bin/ws-mcp-launcher.py:1858-1862` resolves
  local-devenv mode, forced installation and the source fingerprint.
  At `:1892-1899`, the usable-daemon shortcut applies only without forced
  installation; a current compatibility stamp can avoid rebuilding, while
  forced local mode with a stale stamp requests a source rebuild. This is
  not a claim that every session start unconditionally rebuilds.
- **VERIFIED:** `needs_start` at `:1347-1371` requests replacement for an
  older version, or for a same-core daemon with an older build timestamp
  whose content hash differs or is unavailable. Equal known hashes suppress
  replacement. `connect` at `:1437-1444` invokes `start_daemon` automatically
  when `allow_start` is true and the daemon is absent or needs replacement.
  These cited checks contain no active-turn or owner swap-approval gate.
- **VERIFIED:** `start_daemon` at `:1394-1409` runs the staged binary's
  `daemon-start` command with an environment copied from the launcher.
  This supplies a source mechanism for inherited PATH; it does not alone
  prove the environment of any historical process.
- **VERIFIED separate Pi build path:**
  `agents-plugin-pi/src/bridge.ts:916-932` invokes
  `buildLocalDevenvBootstrap` for lead/fork roles at bridge startup.
  With a valid local-devenv marker, `local-devenv.ts:153-185` runs
  `go build` using source HEAD in the version stamp without a fingerprint
  cache; `:207-209` returns `WS_MCP_BOOTSTRAP_BINARY`. The Python launcher's
  bootstrap override forces installation at `:1887-1891`. Thus a local-dev
  Pi lead or fork bridge startup can build even with unchanged source.
- **VERIFIED worker/explore distinction:** `bridge.ts:916-924` skips that
  TypeScript startup build for these roles. Native child launch arguments
  load the parent's extension (`spawner.ts:2935-2939`) and clear inherited
  bootstrap overrides (`:2929-2931`). Their Python MCP launcher still has
  the stale-source-stamp rebuild path above. The lead's separate pre-child
  guard rebuilds on `runtime.json` contract drift and memoizes settled
  contracts (`local-devenv.ts:266-302`); it does not rebuild for every
  ordinary worker/explore spawn.
- **VERIFIED connection route:** the session bootstrap calls `startBridge`
  (`agents-plugin-pi/src/index.ts:875-888`), which creates the client
  (`bridge.ts:954-962`) as `python3 <launcher> serve --stdio`
  (`mcp-stdio-client.ts:346-360`). The same route is loaded by native child
  extension arguments above. Daemon-hosted leads load bundled extension
  entries, with an MCP bridge fallback when unavailable
  (`agents-plugin-tool/internal/wsagent/pi_lead.go:35-62`).
- **INFERRED operational consequence:** a develop merge that changes
  fingerprinted inputs can make the next MCP launcher build a fresh binary
  and replace an older/different daemon without any manual build command.
  No branch-name gate or background source watcher is present in the cited
  launcher checks: a merge alone is not an immediate replacement trigger.
  CLI-mode calls using a usable daemon bypass installation/replacement
  (`agents-plugin/bin/ws-mcp-launcher.py:1881-1885,1924-1926`), while the
  separate Pi lead/fork startup build has the behavior described above.

### Retained incident testimony and causal limits

Transcript reference R is
`/Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/rebelrepugnantstonewareemcee/pi/session.jsonl`;
line numbers below are physical JSONL lines.

- **VERIFIED retained report:** R:39, received at 2026-11-14 01:07:37Z,
  records MAIN observing daemon PID 68420 without a swap notification.
  At that point MAIN explicitly left open whether EXEC had swapped it.
- **VERIFIED retained report:** R:59, received at 01:12:15Z, forwards EXEC's
  01:11:59Z account of a 01:05:40Z replacement: holder 67828 -> Pi 67831 ->
  installed launcher 67844, followed by daemon 68420 with content prefix
  `d2c34344`. The report attributes rebuilding to `.local-devenv-runtime`
  pointing at the primary checkout at `4804cf47f`. MAIN's appended note
  attributes the replacement to its own session start.
- **INFERRED historical cause:** that reported launcher/process sequence,
  the earlier process evidence above, and the verified source replacement
  path support automatic dev-source replacement as the cause of an
  unannounced restart. EXEC's historical wording was "ROOT CAUSE VERIFIED";
  this investigation verified the retained testimony and source mechanism,
  not the historical process tree independently. No new live reproduction,
  PID observation, environment capture or daemon operation was performed.
- **Still unverified:** the exact initiating process for each Nov 13
  15:04:15Z and 15:19:51Z restart, whether any test touched shared state,
  and whether the roughly 74 KB update failures have an independent size
  or timeout cause. Temporal proximity does not settle these questions.

### Related partial repair and policy hold

`261113-bug-ws-launcher-dev-rebuild-replaces-daemon-mid-work` records
this launcher mechanism and a partial bounded-wait repair, but is now dropped
as a duplicate of this ticket (its `## Resolution`,
`ai-docs/tickets/.dropped/261113-bug-ws-launcher-dev-rebuild-replaces-daemon-mid-work.md#L76-L77`).
Its `## Result (partial)` left replacement policy and completion mail durability
open; the bounded-wait repair did not disable automatic replacement
(same file, `#L57-L68`). This ticket owns the replacement-policy repair;
completion mail durability remains a separate investigation.

MAIN subsequently relayed the owner's correction **(703)=a**, superseding
the earlier answer **b**: replace the shared daemon only at explicitly
coordinated, designated timing. Ordinary connection must not replace it
because of a newer same-core development build. The advance-notice design
from option **b** is withdrawn, not pending.

The owner subsequently confirmed **(706)=a**, **(709)=a**, and **(714)=a**.
The accepted design below supersedes the earlier narrower fingerprint-reuse
repair and the earlier exclusion of restart/rollback. No notice design remains.

## Accepted repair design - 2026-11-14

### Verified development-only source-build boundary

The source-fingerprint auto-build path requires a valid local development
contract: `local_devenv_source_fingerprint` returns no fingerprint without it
(`agents-plugin/bin/ws-mcp-launcher.py:546-561`), and
`install_local_devenv_runtime` returns false without that same contract
(`:648-652`). Main computes the fingerprint only when `local_enabled`
(`:1858-1862`), and the forced source-rebuild branch requires both forced
installation and local mode (`:1895-1899`). Non-development runtime repair
continues to use the released download path (`:721-726`). Explicit
`WS_MCP_BOOTSTRAP_BINARY` or `WS_MCP_BOOTSTRAP_URL` overrides can force an
installation without that marker (`:534-541,679-720`); those overrides are not
themselves Go source-build commands. Do not describe every runtime install or
released-version replacement as development-only.

## Decisions

### Owner-confirmed scope

- **(703)=a:** shared development-daemon replacement happens only at explicitly
  coordinated, designated timing, not because an ordinary connection finds a
  newer same-core development build. Preserve released-version compatibility.
  Source: owner via MAIN.
- **(714)=a:** ordinary launcher startup must not automatically build or replace
  a development runtime because its source fingerprint changed. Surface that a
  new build is available; build and swap only through an explicit command at
  designated timing. This also removes the unconditional local-dev Pi lead/fork
  startup build. Reuse the selected runtime rather than merely caching that
  unconditional build; stale source alone is not permission to rebuild.
  Source: owner via MAIN.
- **(706)=a:** keep dead-daemon recovery separate from replacing a running
  daemon. Automatically restart a dead daemon using the last-known-good build,
  never an unaccepted newer candidate. Roll back a designated swap that fails
  its health checks, and record the recovery in the board log.
  Source: owner via MAIN. Carried by the follow-up ticket
  `261115-feat-ws-dev-daemon-accepted-build-recovery`.
- **(709)=a:** a candidate earns last-known-good status after post-start checks
  covering hello, session lookup, whiteboard read and mailbox receive, followed
  by approximately 30 minutes of quiet observation before promotion. Startup
  liveness alone does not establish session-feature correctness.
  Source: owner via MAIN. Carried by the follow-up ticket
  `261115-feat-ws-dev-daemon-accepted-build-recovery`.
- **(709)=a:** emergency rollback must be available through a launcher CLI
  command even when the dashboard is broken, plus a dashboard button visible
  only for development mode identified by `.local-devenv-runtime`.
  Source: owner via MAIN. Carried by the follow-up ticket
  `261115-feat-ws-dev-daemon-accepted-build-recovery`.

### Lead-settled Phase 1 technical decisions

- **(P1a):** ordinary connection keeps a healthy equal-numeric-core daemon
  regardless of build time or content hash, in Go `keepRunning` and Python
  `DaemonLink.needs_start`, including all three launcher copies. Older cores
  are still replaced and newer cores kept; preserve existing LAN-mode handling
  and released-version admission. Go currently orders suffixes as well as cores
  (`agents-plugin-tool/internal/wsdashboard/version.go:24-49`), so implement the
  equal-core exception locally in `keepRunning`, not by globally redefining
  `CompareVersions`. Python already identifies equal cores before its build
  tie-break (`agents-plugin/bin/ws-mcp-launcher.py:1363-1374`); Go's current
  tie-break and LAN branch are at `ensure.go:171-184`.
  Rejected: keep the newer-build-wins tie-break; it is the unannounced-replacement
  mechanism prohibited by (703).
- **(P1b):** add an optional `daemon-start --replace` flag (T0) for explicit
  drain and replacement of an equal-core daemon, including identical content.
  Preserve newer-core protection and the existing authenticated shutdown/lock
  handoff. Parse it in Go's existing `flag.FlagSet` and pass it into Ensure
  (`agents-plugin-tool/cmd/ws-mcp/proxy.go:361-409`). Consume the explicit
  replacement request once, rather than repeatedly replacing the successor in
  Ensure's loop (`agents-plugin-tool/internal/wsdashboard/ensure.go:127-159`).
  Python's lifecycle shortcut must not return `kept` for an explicit `--replace`
  (`agents-plugin/bin/ws-mcp-launcher.py:1932-1939`).
  `python3 scripts/dev.py rerun daemon` remains the one explicit build+swap
  command and passes `--replace` alongside `--build-time`
  (`scripts/dev.py:297-330,231`). Its identical-content pre-shutdown workaround
  (`:202-207,318-322`) becomes redundant: remove that path and let daemon-start
  own the drain for both identical and changed content.
  Rejected: a new launcher build subcommand, because dev.py already owns
  explicit build/swap; relying on build time, because P1a removes that trigger.
- **(P1c):** in Python local-devenv mode, a stale source fingerprint alone
  reuses an existing compatible staged runtime instead of forcing a build.
  Check compatibility independently of fingerprint freshness and print one
  stderr note per startup that a newer source build is available via
  `python3 scripts/dev.py rerun daemon`. Do not stamp stale content as freshly
  built source. A missing or incompatible runtime still installs/builds.
  `runtime_install_forced` covers both explicit bootstrap binary/URL overrides
  and local mode (`agents-plugin/bin/ws-mcp-launcher.py:534-543`); preserve
  explicit bootstrap override semantics (`:1889-1892`), changing the stale
  local-source branch (`:1897-1905`), not every forced install. Reuse the
  existing hash-bound compatibility check (`:732-764`); the source fingerprint
  is separate stamp data (`:300-334`).
  Rejected: rebuild-but-not-replace; it still runs an unrequested Go build
  under load, contrary to (714).
- **(P1d):** remove unconditional Go builds from ordinary Pi lead/fork bridge
  startup (`agents-plugin-pi/src/bridge.ts:916-932`,
  `agents-plugin-pi/src/local-devenv.ts:153-209`). Build only when no usable
  staged runtime exists; otherwise reuse it and notify source drift with the
  same explicit dev.py command as P1c. Reuse must not delete the retained
  runtime through the unique-bootstrap cleanup path (`bridge.ts:965-970`),
  or disable child contract validation merely because startup did not build
  (`:935-947`). Preserve worker/explore startup behavior.
  The separate child runtime.json contract-drift guard is outside this repair
  (`local-devenv.ts:234-302`). P1a prevents its equal-core builds from replacing
  a live daemon, but NOT replacement after a numeric-core bump: the guard reads
  the current plugin_version and builds it (`:267-276`); inspection installs
  without starting (`agents-plugin/bin/ws-mcp-launcher.py:1930-1931`), then a
  child's subsequent connection can replace an older core (`:1365-1366,
  :1441-1444`). No blanket no-replacement claim applies to that guard.
  Rejected: cache the unconditional startup build while still building merely
  for source drift; (714) requires reusing the selected usable runtime.
- **(P1e):** Phase 1 compatibility tier is **T0**: no store shape, protocol,
  manifest or tracker field change. `--replace` is additive to daemon-start's
  existing flags; dev.py invokes its newly built staged binary, not an old
  running daemon binary (`scripts/dev.py:308-324`). Existing calls omit it.
  Health writers retain version/build/hash fields
  (`agents-plugin-tool/internal/wsdashboard/daemon.go:761-775`); Go readers
  decode them (`client.go:25-46`) and currently use them for selection
  (`ensure.go:171-184`), as does old Python (`ws-mcp-launcher.py:1363-1374`).
  Stamp writers/readers keep the same shape (`ws-mcp-launcher.py:300-349`);
  reopening the new launcher revalidates/reuses the same runtime metadata,
  rather than migrating recovery data. Shutdown still uses the existing token
  and endpoint (`client.go:53-69`). Old binaries with the old tie-break can
  replace an equal-version daemon until rebuilt; that is the transition, not a
  regression or an old-client lockout. Recovery-state compatibility belongs
  to the follow-up, not this T0 verdict.
  Rejected: a version boundary or store/protocol redesign for a selection-policy
  repair and an additive CLI flag.

## Constraints

- Implementation is authorized for Phase 1 on isolated fixtures;
  shared-daemon activation is MAIN's swap. No implementation verification may
  touch the shared daemon or cache, kill its processes, or change its install.
- Preserve first-install and released-version compatibility behavior while
  changing development auto-build/replacement policy. A missing, dead, hung,
  incompatible or newer running daemon must be distinguished explicitly.
- Accepted-build retention/recovery, promotion probes and quiet observation,
  rollback CLI/UI and recovery logs are out of scope and carried by
  `261115-feat-ws-dev-daemon-accepted-build-recovery`. Phase 1 preserves existing
  absent/dead/hung handling using the selected runtime; it does not claim that
  this runtime has earned last-known-good status.
- Mail durability and large-request failures remain separate investigations.
- Before launcher/tooling edits read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, and `ai-docs/manuals/ws-mcp.md` where
  applicable. Keep launcher copies in ws, wsflow and Pi consistent; read
  `ai-docs/manuals/wsflow-mirroring.md` for mirrored plugin edits. Consult Pi SDK
  integration documentation before changing Pi bootstrap behavior.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261113-bug-ws-launcher-dev-rebuild-replaces-daemon-mid-work (2026-11-15, Resolution): "Triage 2026-11-15 (owner (1057)): DROP" — bearing: supports
- 8b20fd84 (2026-11-14, commit): "Cold CLI startup, explicit lifecycle commands, and interactive serve behavior remain unchanged." — bearing: constrains outside the explicit P1a–P1d startup/replacement changes.
- 1866275e (2026-11-13, commit): "Owner priority: AhnLab prompts on non-daemon ws execution; remove ProcessStream, DIRECT bypass, catch-all exec, and executable inspection instead of adding another fallback." — bearing: constrains
- fa267b5f (2026-11-13, commit): "Initialize alone is replayed; lost tool side effects are not." — bearing: constrains
- 677711b3 (2026-11-13, commit): "Bind local bootstrap metadata to the actual build and remove both artifacts after installation; version inspection relies on the reviewed launcher dispatch." — bearing: constrains
- 40da39ae (2026-11-11, commit): "Chosen: the plan's first candidate, equal core -> tie-break, inside needs_start only. compare_daemon_versions keeps Go parity." — bearing: historical; equal-core tie-break superseded by (703)/P1a, global comparator parity remains unchanged.
- cb0a89b7 (2026-09-07, commit): "No fingerprint cache and no cache fallback on build failure, per the ticket's explicit rejection of a caching layer for this dev-only path — every lead/fork session start rebuilds from source." — bearing: historical; unconditional startup-build policy superseded by (714)/P1d, separate child contract-drift guard remains outside Phase 1.
- 16dc6239 (2026-07-26, commit): "Scoped strictly to the marker path so release integrity is unchanged: the default False preserves exact-match everywhere the download/bootstrap paths run." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1: three Python launcher copies, agents-plugin-pi/src/bridge.ts#L916-L947 and src/local-devenv.ts#L153-L209, agents-plugin-tool/internal/wsdashboard/ensure.go#L127-L184, cmd/ws-mcp/proxy.go#L361-L409 and scripts/dev.py#L223-L244,L297-L330; recovery/dashboard controls moved to the follow-up. |
| scope.surface | cross-module | Python selection/lifecycle, Pi startup bootstrap, Go Ensure/daemon-start and the repository-only explicit dev command; existing Python entry points are agents-plugin/bin/ws-mcp-launcher.py#L1349-L1444,L1885-L1939. |
| scope.new_public_symbol | yes | Additive daemon-start --replace CLI flag in agents-plugin-tool/cmd/ws-mcp/proxy.go#L361-L365; no rollback command or dashboard endpoint in Phase 1. |
| scope.new_type_contract | no | No recovery-state schema or cross-version contract is added; existing EnsureOptions at agents-plugin-tool/internal/wsdashboard/ensure.go#L40-L79 needs an internal explicit-replacement option only. Existing health/stamp shapes remain unchanged (daemon.go#L283-L301; agents-plugin/bin/ws-mcp-launcher.py#L300-L349). |
| scope.test_surface | existing | Go agents-plugin-tool/internal/wsdashboard/daemon_test.go#L325-L606; Python agents-plugin/tests/test_ws_mcp_launcher_proxy.py#L412-L520,L957-L997 and test_ws_mcp_launcher_capabilities.py#L553-L635; Pi agents-plugin-pi/test/local-devenv.test.ts and test/version-check.test.ts#L155-L160; scripts/test_dev.py#L113-L127. Extend fixtures for reuse/notice/--replace and three-copy parity. No tests were run. |
| complexity.reuse_points | confirmed | DaemonLink.needs_start/start_daemon/connect and runtime_fully_compatible (agents-plugin/bin/ws-mcp-launcher.py#L732-L764,L1349-L1444), Go Ensure shutdown/lock handoff (ensure.go#L127-L159), and dev.py's existing build/staging command (#L297-L324) supply the Phase 1 seams. |
| complexity.side_effect_risk | high | Explicit --replace drains sessions through the existing RequestShutdown path (agents-plugin-tool/internal/wsdashboard/ensure.go#L140-L147; client.go#L53-L69); isolated fixtures only, MAIN owns shared activation. Ordinary source drift must no longer trigger build or equal-core replacement. |
| risk.correctness | high | Preserve older/newer-core ordering and LAN precedence while removing build selection (ensure.go#L171-L184; version.go#L24-L49); explicit replacement must be one-shot across Ensure's loop (#L127-L159) and bypass Python's lifecycle keep shortcut (ws-mcp-launcher.py#L1932-L1939). |
| risk.fit | high | runtime_install_forced includes bootstrap overrides and local mode (agents-plugin/bin/ws-mcp-launcher.py#L534-L543); Pi reuse must preserve cleanup ownership and child guard wiring (agents-plugin-pi/src/bridge.ts#L935-L970). Guard remains out of scope, but numeric-core drift can still cause version-driven replacement (local-devenv.ts#L267-L276; Python #L1365-L1366,L1441-L1444). |
| risk.test | high | Existing in-process Go spawner uses a supplied directory and loopback (agents-plugin-tool/internal/wsdashboard/daemon_test.go#L243-L282); Python proxy and Pi injected-build fixtures cover drift without real builds. Assert no build/replacement on equal-core reuse and one-shot explicit swap. No shared-daemon test is permitted. |
| risk.security_or_contract | moderate | P1e T0: unchanged health writer (agents-plugin-tool/internal/wsdashboard/daemon.go#L761-L775), reader (client.go#L25-L46), authenticated shutdown (#L53-L69), and stamp readers/writers (agents-plugin/bin/ws-mcp-launcher.py#L300-L349); additive CLI parsing (cmd/ws-mcp/proxy.go#L361-L365). Preserve existing minor admission (internal/wsdashboard/proxy.go#L232-L254); accepted-build recovery compatibility is deferred. |

## Phases

### Phase 1: Separate ordinary connection from explicit development build and swap

Implement (703)/(714) through P1a–P1e only:

1. Keep healthy equal-core daemons in Go and all Python launchers regardless of
   timestamp/hash, without changing older/newer-core compatibility or existing
   LAN handling. Keep absent/dead/hung states distinct.
2. Add one-shot `daemon-start --replace`, honor it through Python lifecycle
   dispatch, and have dev.py's existing build+swap command pass it. Remove dev.py's
   now-redundant identical-content pre-shutdown path; leave the authenticated
   drain and lock handoff to daemon-start. Do not permit a newer-core downgrade.
3. Reuse compatible local-devenv runtimes on stale source fingerprints and emit
   one stderr reuse/drift note pointing at `python3 scripts/dev.py rerun daemon`.
   Preserve first install/incompatible-runtime repair, explicit bootstrap overrides
   and released runtime behavior.
4. Reuse the staged runtime for ordinary Pi lead/fork startup instead of an
   unconditional build; only missing/unusable runtime triggers startup building.
   Preserve retained-runtime ownership, worker/explore behavior and child guard
   wiring. The separate contract-drift guard and its version-driven replacements
   are outside this phase; do not claim P1a prevents newer-core replacement.

Verification uses only temporary state/cache roots, loopback fake/in-process
servers, injected build seams and mocked subprocesses. No test may touch the
shared daemon, its cache, or real user/session/mail/board state. Cover:

- Go `keepRunning` table cases and existing Ensure fixtures in
  `agents-plugin-tool/internal/wsdashboard/daemon_test.go`: equal core with a
  newer candidate build is kept for different/missing/equal hashes and suffix
  variants; older core is replaced even with a newer timestamp; newer core is
  kept; existing LAN precedence is preserved. Explicit `--replace` replaces
  equal-core identical/different content once, keeps its successor, and does
  not downgrade a newer core. Test CLI flag wiring without a shared process.
- Python existing `agents-plugin/tests/test_ws_mcp_launcher_proxy.py` and
  `test_ws_mcp_launcher_capabilities.py`: `needs_start` core/hash/build matrix,
  stale-fingerprint compatible-runtime reuse with the single stderr command
  note and no install/build, unchanged-source reuse, missing/incompatible first
  install/repair, explicit bootstrap binary/URL semantics, lifecycle `--replace`
  bypass, and healthy/dead/hung and released-plugin reconnect behavior. Assert
  no ordinary equal-core startup requests daemon replacement.
- Pi `agents-plugin-pi/test/local-devenv.test.ts`: lead/fork startup with a usable
  runtime calls no `runBuild` despite source drift and surfaces the command
  note; missing/unusable runtimes use the injected build seam. Worker/explore
  startup stays build-free; retained runtimes survive cleanup and child guard
  wiring remains active without a startup build. Do not use live Go builds.
- Existing Pi launcher-copy parity test at
  `agents-plugin-pi/test/version-check.test.ts:155-160`; check wsflow parity too
  (all three launcher files are currently byte-identical). Extend isolated
  parity coverage to wsflow if no existing assertion covers that copy.
- `scripts/test_dev.py`: mocked rerun/daemon-start invocation passes `--replace`
  alongside `--build-time`, including identical content, without a separate
  pre-shutdown. Mock builds, staging and health/shutdown operations throughout.

Accepted-build recovery, promotion and emergency rollback are implemented only
by `261115-feat-ws-dev-daemon-accepted-build-recovery`, after this phase lands.

### Result (02ba6f86e) - 2026-11-15

Implemented P1a–P1e. Ordinary Python/Go connection keeps a healthy equal
numeric core across suffix, timestamp and hash differences. Numeric upgrades,
newer-core protection, authenticated drain and LAN-mode handling remain.
`daemon-start --replace` explicitly swaps an equal core once (including identical
content); consuming it before the first spawn also protects the successor when
the initial endpoint is absent. `dev.py rerun daemon` passes that flag and no
longer performs its own identical-content shutdown.

Python reuses hash-bound compatible local runtimes on source drift, emits the
explicit build+swap command, and preserves the old source fingerprint. Missing
or incompatible runtime repair and explicit bootstrap binary/URL installs remain.
Pi lead/fork startup uses a read-only Python helper import to share the launcher's
compatibility, runtime naming and fingerprint authority rather than duplicating
it in TypeScript. Retained runtime bytes are not owned by bootstrap cleanup;
the child contract-drift guard remains active after reuse. Worker/explore startup
stays build-free. The existing child-wiring fake launcher was adapted to expose
the imported helpers while retaining its fake executable behavior.

Compatibility remains **T0**. The optional flag is parsed and forwarded locally
(`agents-plugin-tool/cmd/ws-mcp/proxy.go:365-409`); dev.py invokes its own staged
candidate (`scripts/dev.py`, `run_daemon_start`). Health still writes version,
build and hash (`agents-plugin-tool/internal/wsdashboard/daemon.go:761-775`) and
Go reads those unchanged fields (`client.go:25-46`, relative to the same package).
Shutdown retains the existing token path (`client.go:53-69`). Metadata and source
stamp writers/readers retain their shapes
(`agents-plugin/bin/ws-mcp-launcher.py:198-240,300-349`); reopening this launcher
revalidates the retained bytes rather than migrating data. Old launchers can
still perform their old build-time replacement until updated, as P1e states.
No protocol, manifest, shared-store shape or version changed.

Verification (all isolated; complete commands/output in
`/tmp/exec5/daemonfix/verification.md`):

- Focused offline Go Ensure/core-policy/absent/dead/hung/LAN fixtures: PASS;
  `go-dashboard-final.log`. In-process servers use temporary directories and
  loopback. No whole-dashboard-package run.
- Go daemon-start flag wiring and LAN apply rule: PASS (2 tests);
  `go-cli-final.log`. Ensure is injected; no detached process starts.
- Existing Python proxy file, focused selection: PASS (16 tests);
  `python-proxy.log`. Existing reconnect fixtures emit unclosed-pipe
  ResourceWarnings; added tests introduce none.
- Existing Python capabilities file, focused selection: PASS (10 tests);
  `python-capabilities-fixed.log`. The initial fixture was outside its marker's
  source root; fixed the fixture rather than production behavior.
- `scripts/test_dev.py`: PASS (11 tests); `python-dev-final.log`. Builds,
  staging, seeding, health and drain subprocesses are mocked for rerun coverage.
- Named Pi `local-devenv.test.ts` and `version-check.test.ts`: PASS (51 tests);
  `pi-final.log`. Builds are injected. Coverage includes quiet unchanged source,
  drift reuse, unusable-runtime repair, actual bridge cleanup/child-guard wiring,
  role gates and byte-identical ws/wsflow/Pi launchers. The initial new worker
  assertion incorrectly included the existing tool-registration notification;
  narrowed the assertion to source-build notices.
- `cmp` for all three launchers and `git diff --check`: PASS.

Review: the implementer performed the lite correctness/test-integrity read and
manual self-audit because the caller prohibited subagents. Removed unreachable
assertion branches from the updated Go fixtures. Evidence:
`/tmp/exec5/daemonfix/self-review.md`. No unresolved correctness finding.
Independent review/audit, broader suites (including the adapted existing
child-wiring integration fixture) and shared activation were omitted under the
caller's restrictions. The two named Pi suites directly exercise the new bridge
reuse/cleanup/guard behavior. The worktree temporarily linked the primary
checkout's existing dependencies for these tests; no install ran and the link
was removed.

No shared daemon operation/cache activation, live Python/Pi Go build, process
kill, install, merge, push or version bump was performed. Existing absent/dead/
hung behavior uses the selected runtime; this phase does not establish accepted
or last-known-good status. Recovery/promotion/rollback, large-request failures
and mail durability remain outside this ticket as declared above.
